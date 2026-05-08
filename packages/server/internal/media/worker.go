package media

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	defaultAssetGCInterval            = time.Minute
	defaultAssetReconcileInterval     = 24 * time.Hour
	defaultThumbnailReconcileInterval = time.Hour
	defaultAssetGCBatch               = 20
	defaultAssetGCMaxRetry            = 5
	defaultAssetGCRetryGap            = 5 * time.Minute
	defaultAssetReconcile             = 200
	defaultBlobDeleteDelay            = 24 * time.Hour
	defaultAssetDeleteDelay           = 24 * time.Hour
)

func StartAssetGCWorker(ctx context.Context) {
	if !assetGCEnabledFromEnv() {
		log.Println("[media.gc] disabled")
		return
	}

	interval := assetGCIntervalFromEnv()
	reconcileInterval := assetReconcileIntervalFromEnv()
	thumbnailInterval := thumbnailReconcileIntervalFromEnv()
	batchSize := assetGCBatchSizeFromEnv()
	reconcileBatch := assetReconcileBatchSizeFromEnv()
	reconcileEnabled := assetReconcileEnabledFromEnv()
	if interval <= 0 || batchSize <= 0 {
		log.Printf("[media.gc] skipped invalid config interval=%s batch=%d", interval, batchSize)
		return
	}

	go func() {
		gcTicker := time.NewTicker(interval)
		defer gcTicker.Stop()

		var reconcileTicker *time.Ticker
		var reconcileCh <-chan time.Time
		if reconcileEnabled && reconcileInterval > 0 {
			reconcileTicker = time.NewTicker(reconcileInterval)
			reconcileCh = reconcileTicker.C
			defer reconcileTicker.Stop()
		}

		var thumbnailTicker *time.Ticker
		var thumbnailCh <-chan time.Time
		if reconcileEnabled && thumbnailInterval > 0 {
			thumbnailTicker = time.NewTicker(thumbnailInterval)
			thumbnailCh = thumbnailTicker.C
			defer thumbnailTicker.Stop()
		}

		log.Printf("[media.gc] started gc_interval=%s batch=%d reconcile=%v reconcile_interval=%s thumbnail_interval=%s reconcile_batch=%d", interval, batchSize, reconcileEnabled, reconcileInterval, thumbnailInterval, reconcileBatch)
		for {
			select {
			case <-ctx.Done():
				log.Println("[media.gc] stopped")
				return
			case <-gcTicker.C:
				if _, err := RunDueAssetGCJobs(ctx, time.Now(), batchSize); err != nil {
					log.Printf("[media.gc] run failed: %v", err)
				}
				if _, err := RunDueBlobGCJobs(ctx, time.Now(), batchSize); err != nil {
					log.Printf("[media.gc] blob run failed: %v", err)
				}
			case <-reconcileCh:
				now := time.Now()
				reconciled, err := RunAssetReferenceReconcilePass(now, reconcileBatch)
				if err != nil {
					log.Printf("[media.gc] reconcile failed: %v", err)
				} else if reconciled > 0 {
					log.Printf("[media.gc] reconciled assets=%d", reconciled)
				}
			case <-thumbnailCh:
				backfilled, err := RunBlobThumbnailReconcilePass(ctx, reconcileBatch)
				if err != nil {
					log.Printf("[media.gc] thumbnail reconcile failed: %v", err)
				} else if backfilled > 0 {
					log.Printf("[media.gc] reconciled thumbnails=%d", backfilled)
				}
			}
		}
	}()
}

func RunDueAssetGCJobs(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = defaultAssetGCBatch
	}

	var jobs []models.AssetGCJob
	if err := database.DB.
		Where("job_type = ? AND status = ? AND run_after <= ?", "delete", "pending", now).
		Order("run_after asc").
		Limit(limit).
		Find(&jobs).Error; err != nil {
		return 0, err
	}

	processed := 0
	for _, job := range jobs {
		processed++
		if err := runAssetDeleteJob(ctx, job.ID, now); err != nil {
			log.Printf("[media.gc] job=%s failed: %v", job.ID, err)
		}
	}

	return processed, nil
}

func runAssetDeleteJob(ctx context.Context, jobID uuid.UUID, now time.Time) error {
	var job models.AssetGCJob
	var asset models.Asset
	var blobID uuid.UUID

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND job_type = ? AND status = ?", jobID, "delete", "pending").First(&job).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.AssetGCJob{}).
			Where("id = ?", job.ID).
			Updates(map[string]any{
				"status":        "running",
				"attempt_count": gorm.Expr("attempt_count + 1"),
				"updated_at":    now,
				"last_error":    nil,
			}).Error; err != nil {
			return err
		}

		if err := tx.Where("id = ?", job.AssetID).First(&asset).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return tx.Model(&models.AssetGCJob{}).
					Where("id = ?", job.ID).
					Updates(map[string]any{
						"status":     "done",
						"updated_at": now,
					}).Error
			}
			return err
		}

		var refCount int64
		if err := tx.Model(&models.DocumentAssetRef{}).
			Where("asset_id = ? AND ref_type = ?", job.AssetID, "editor_content").
			Count(&refCount).Error; err != nil {
			return err
		}
		if refCount > 0 {
			if err := tx.Model(&models.Asset{}).
				Where("id = ?", asset.ID).
				Updates(map[string]any{
					"status":          "ready",
					"reference_count": int(refCount),
					"updated_at":      now,
				}).Error; err != nil {
				return err
			}
			return tx.Model(&models.AssetGCJob{}).
				Where("id = ?", job.ID).
				Updates(map[string]any{
					"status":     "cancelled",
					"updated_at": now,
				}).Error
		}

		blobID = asset.BlobID
		if err := tx.Model(&models.Asset{}).
			Where("id = ? AND deleted_at IS NULL", asset.ID).
			Updates(map[string]any{
				"status":          "deleted",
				"reference_count": 0,
				"updated_at":      now,
				"deleted_at":      now,
			}).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.AssetGCJob{}).
			Where("id = ?", jobID).
			Updates(map[string]any{
				"status":     "done",
				"updated_at": now,
			}).Error; err != nil {
			return err
		}

		return ensurePendingBlobDeleteJob(tx, blobID, now)
	})
	if err != nil {
		return markAssetGCJobFailed(jobID, now, err)
	}
	_ = ctx
	_ = asset
	_ = blobID
	return nil
}

func markAssetGCJobFailed(jobID uuid.UUID, now time.Time, cause error) error {
	message := cause.Error()
	maxAttempts := assetGCMaxAttemptsFromEnv()
	retryGap := assetGCRetryGapFromEnv()

	var job models.AssetGCJob
	if err := database.DB.First(&job, "id = ?", jobID).Error; err != nil {
		return cause
	}

	status := "failed"
	runAfter := job.RunAfter
	if job.AttemptCount < maxAttempts {
		attemptCount := job.AttemptCount
		if attemptCount < 1 {
			attemptCount = 1
		}
		status = "pending"
		runAfter = now.Add(retryGap * time.Duration(1<<(attemptCount-1)))
	}

	updateErr := database.DB.Model(&models.AssetGCJob{}).
		Where("id = ?", jobID).
		Updates(map[string]any{
			"status":     status,
			"run_after":  runAfter,
			"last_error": message,
			"updated_at": now,
		}).Error
	if updateErr != nil {
		return errors.Join(cause, updateErr)
	}
	return cause
}

func RunAssetReferenceReconcilePass(now time.Time, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = defaultAssetReconcile
	}

	var assets []models.Asset
	if err := database.DB.Where("deleted_at IS NULL").Order("updated_at asc").Limit(batchSize).Find(&assets).Error; err != nil {
		return 0, err
	}

	reconciled := 0
	for _, asset := range assets {
		if err := reconcileOneAsset(now, asset.ID); err != nil {
			return reconciled, err
		}
		reconciled++
	}
	return reconciled, nil
}

func RunBlobThumbnailReconcilePass(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = defaultAssetReconcile
	}
	if err := initStorageProvider(); err != nil {
		return 0, err
	}
	maxBytes := thumbnailSourceMaxBytesFromEnv()

	subQuery := database.DB.Model(&models.Asset{}).
		Select("DISTINCT blob_id").
		Where("deleted_at IS NULL")

	var blobs []models.BlobObject
	if err := database.DB.
		Where("id IN (?)", subQuery).
		Where("deleted_at IS NULL").
		Where("mime_type LIKE ?", "image/%").
		Where("thumbnail_status = ? OR thumbnail_status = '' OR thumbnail_status IS NULL OR (thumbnail_status = ? AND (thumbnail_object_key = '' OR thumbnail_object_key IS NULL))", blobThumbnailStatusPending, blobThumbnailStatusReady).
		Order("updated_at asc").
		Limit(batchSize).
		Find(&blobs).Error; err != nil {
		return 0, err
	}

	backfilled := 0
	for _, blob := range blobs {
		if blob.Size > maxBytes {
			_ = markBlobThumbnailState(blob.ID, blobThumbnailStatusSkipped, map[string]any{
				"thumbnail_object_key": "",
				"thumbnail_mime_type":  "",
				"thumbnail_size":       0,
				"thumbnail_width":      nil,
				"thumbnail_height":     nil,
			})
			continue
		}

		obj, err := storageProvider.GetObject(ctx, blob.ObjectKey)
		if err != nil {
			log.Printf("[media.gc] thumbnail source fetch failed blob=%s err=%v", blob.ID, err)
			continue
		}

		sourceBytes, readErr := readAllLimited(obj.Body, maxBytes)
		_ = obj.Body.Close()
		if readErr != nil {
			log.Printf("[media.gc] thumbnail source read failed blob=%s err=%v", blob.ID, readErr)
			continue
		}

		if err := generateAndStoreBlobThumbnail(ctx, blob, sourceBytes); err != nil {
			log.Printf("[media.gc] thumbnail generate failed blob=%s err=%v", blob.ID, err)
			continue
		}
		backfilled++
	}

	return backfilled, nil
}

func readAllLimited(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, errors.New("invalid read limit")
	}
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("object too large to load into memory")
	}
	return data, nil
}

func RunDueBlobGCJobs(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = defaultAssetGCBatch
	}

	var jobs []models.BlobGCJob
	if err := database.DB.
		Where("job_type = ? AND status = ? AND run_after <= ?", "delete", "pending", now).
		Order("run_after asc").
		Limit(limit).
		Find(&jobs).Error; err != nil {
		return 0, err
	}

	processed := 0
	for _, job := range jobs {
		processed++
		if err := runBlobDeleteJob(ctx, job.ID, now); err != nil {
			log.Printf("[media.gc] blob job=%s failed: %v", job.ID, err)
		}
	}
	return processed, nil
}

func runBlobDeleteJob(ctx context.Context, jobID uuid.UUID, now time.Time) error {
	var job models.BlobGCJob
	var blob models.BlobObject
	var shouldDelete bool

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND job_type = ? AND status = ?", jobID, "delete", "pending").First(&job).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.BlobGCJob{}).
			Where("id = ?", job.ID).
			Updates(map[string]any{
				"status":        "running",
				"attempt_count": gorm.Expr("attempt_count + 1"),
				"updated_at":    now,
				"last_error":    nil,
			}).Error; err != nil {
			return err
		}

		if err := tx.Where("id = ?", job.BlobID).First(&blob).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return tx.Model(&models.BlobGCJob{}).
					Where("id = ?", job.ID).
					Updates(map[string]any{
						"status":     "done",
						"updated_at": now,
					}).Error
			}
			return err
		}

		var activeAssetCount int64
		if err := tx.Model(&models.Asset{}).
			Where("blob_id = ? AND deleted_at IS NULL", job.BlobID).
			Count(&activeAssetCount).Error; err != nil {
			return err
		}
		if activeAssetCount > 0 {
			return tx.Model(&models.BlobGCJob{}).
				Where("id = ?", job.ID).
				Updates(map[string]any{
					"status":     "cancelled",
					"updated_at": now,
				}).Error
		}

		shouldDelete = true
		return nil
	})
	if err != nil {
		return markBlobGCJobFailed(jobID, now, err)
	}
	if blob.ID == uuid.Nil || !shouldDelete {
		return nil
	}

	if err := initStorageProvider(); err != nil {
		return markBlobGCJobFailed(jobID, now, err)
	}
	if blob.ThumbnailStatus == blobThumbnailStatusReady && strings.TrimSpace(blob.ThumbnailObjectKey) != "" {
		if err := storageProvider.DeleteObject(ctx, blob.ThumbnailObjectKey); err != nil {
			return markBlobGCJobFailed(jobID, now, err)
		}
	}
	if err := storageProvider.DeleteObject(ctx, blob.ObjectKey); err != nil {
		return markBlobGCJobFailed(jobID, now, err)
	}

	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.BlobObject{}).
			Where("id = ? AND deleted_at IS NULL", blob.ID).
			Updates(map[string]any{
				"status":     "deleted",
				"updated_at": now,
				"deleted_at": now,
			}).Error; err != nil {
			return err
		}

		return tx.Model(&models.BlobGCJob{}).
			Where("id = ?", jobID).
			Updates(map[string]any{
				"status":     "done",
				"updated_at": now,
			}).Error
	})
}

func markBlobGCJobFailed(jobID uuid.UUID, now time.Time, cause error) error {
	message := cause.Error()
	maxAttempts := assetGCMaxAttemptsFromEnv()
	retryGap := assetGCRetryGapFromEnv()

	var job models.BlobGCJob
	if err := database.DB.First(&job, "id = ?", jobID).Error; err != nil {
		return cause
	}

	status := "failed"
	runAfter := job.RunAfter
	if job.AttemptCount < maxAttempts {
		attemptCount := job.AttemptCount
		if attemptCount < 1 {
			attemptCount = 1
		}
		status = "pending"
		runAfter = now.Add(retryGap * time.Duration(1<<(attemptCount-1)))
	}

	updateErr := database.DB.Model(&models.BlobGCJob{}).
		Where("id = ?", jobID).
		Updates(map[string]any{
			"status":     status,
			"run_after":  runAfter,
			"last_error": message,
			"updated_at": now,
		}).Error
	if updateErr != nil {
		return errors.Join(cause, updateErr)
	}
	return cause
}

func reconcileOneAsset(now time.Time, assetID uuid.UUID) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		var asset models.Asset
		if err := tx.Where("id = ?", assetID).First(&asset).Error; err != nil {
			return err
		}

		var refCount int64
		if err := tx.Model(&models.DocumentAssetRef{}).
			Where("asset_id = ? AND ref_type = ?", assetID, "editor_content").
			Count(&refCount).Error; err != nil {
			return err
		}

		nextStatus := "ready"
		if refCount == 0 {
			nextStatus = "pending_delete"
		}
		if asset.Status == "deleted" {
			nextStatus = "deleted"
		}

		if err := tx.Model(&models.Asset{}).
			Where("id = ?", assetID).
			Updates(map[string]any{
				"reference_count": int(refCount),
				"status":          nextStatus,
				"updated_at":      now,
			}).Error; err != nil {
			return err
		}

		if nextStatus == "deleted" {
			return tx.Model(&models.AssetGCJob{}).
				Where("asset_id = ? AND status = ?", assetID, "pending").
				Updates(map[string]any{
					"status":     "cancelled",
					"updated_at": now,
				}).Error
		}

		if refCount > 0 {
			return tx.Model(&models.AssetGCJob{}).
				Where("asset_id = ? AND status = ?", assetID, "pending").
				Updates(map[string]any{
					"status":     "cancelled",
					"updated_at": now,
				}).Error
		}

		var pending models.AssetGCJob
		err := tx.Where("asset_id = ? AND job_type = ? AND status = ?", assetID, "delete", "pending").First(&pending).Error
		if err == nil {
			delay := assetDeleteDelayFromEnv()
			targetRunAfter := now.Add(delay)
			// Only pull the schedule earlier; never push an already-earlier job later.
			if pending.RunAfter.After(targetRunAfter) {
				return tx.Model(&models.AssetGCJob{}).
					Where("id = ?", pending.ID).
					Updates(map[string]any{
						"run_after":  targetRunAfter,
						"updated_at": now,
					}).Error
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		delay := assetDeleteDelayFromEnv()
		return tx.Create(&models.AssetGCJob{
			ID:       uuid.New(),
			AssetID:  assetID,
			JobType:  "delete",
			Status:   "pending",
			RunAfter: now.Add(delay),
		}).Error
	})
}

func assetGCEnabledFromEnv() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEDIA_ASSET_GC_ENABLED")))
	return raw == "" || raw == "1" || raw == "true" || raw == "yes"
}

func assetGCIntervalFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("MEDIA_ASSET_GC_INTERVAL"))
	if raw == "" {
		return defaultAssetGCInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return defaultAssetGCInterval
	}
	return d
}

func assetReconcileIntervalFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("MEDIA_ASSET_RECONCILE_INTERVAL"))
	if raw == "" {
		return defaultAssetReconcileInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultAssetReconcileInterval
	}
	return d
}

func thumbnailReconcileIntervalFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("MEDIA_THUMBNAIL_RECONCILE_INTERVAL"))
	if raw == "" {
		return defaultThumbnailReconcileInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultThumbnailReconcileInterval
	}
	return d
}

func assetGCBatchSizeFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("MEDIA_ASSET_GC_BATCH_SIZE"))
	if raw == "" {
		return defaultAssetGCBatch
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultAssetGCBatch
	}
	return n
}

func assetGCMaxAttemptsFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("MEDIA_ASSET_GC_MAX_ATTEMPTS"))
	if raw == "" {
		return defaultAssetGCMaxRetry
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultAssetGCMaxRetry
	}
	return n
}

func assetGCRetryGapFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("MEDIA_ASSET_GC_RETRY_GAP"))
	if raw == "" {
		return defaultAssetGCRetryGap
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultAssetGCRetryGap
	}
	return d
}

func assetReconcileEnabledFromEnv() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEDIA_ASSET_RECONCILE_ENABLED")))
	return raw == "" || raw == "1" || raw == "true" || raw == "yes"
}

func assetReconcileBatchSizeFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("MEDIA_ASSET_RECONCILE_BATCH_SIZE"))
	if raw == "" {
		return defaultAssetReconcile
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultAssetReconcile
	}
	return n
}

func assetDeleteDelayFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("MEDIA_ASSET_DELETE_DELAY"))
	if raw == "" {
		return defaultAssetDeleteDelay
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		return defaultAssetDeleteDelay
	}
	return d
}

func blobDeleteDelayFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("MEDIA_BLOB_DELETE_DELAY"))
	if raw == "" {
		return defaultBlobDeleteDelay
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultBlobDeleteDelay
	}
	return d
}
