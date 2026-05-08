package media

import (
	"context"
	"errors"
	"testing"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/google/uuid"
)

type mockStorageProvider struct {
	deleteCalls []string
	getCalls    []string
	deleteErr   error
}

func (m *mockStorageProvider) ProviderName() string {
	return "mock"
}

func (m *mockStorageProvider) PutObject(_ context.Context, _ PutObjectInput) (*PutObjectResult, error) {
	return nil, errors.New("not implemented")
}

func (m *mockStorageProvider) GetObject(_ context.Context, objectKey string) (*GetObjectResult, error) {
	m.getCalls = append(m.getCalls, objectKey)
	return nil, errors.New("not implemented")
}

func (m *mockStorageProvider) DeleteObject(_ context.Context, objectKey string) error {
	m.deleteCalls = append(m.deleteCalls, objectKey)
	return m.deleteErr
}

func TestRunBlobThumbnailReconcilePassSkipsTerminalStatuses(t *testing.T) {
	db := setupMediaTestDB(t)
	userID := uuid.New()
	docID := seedOwnedDocument(t, db, userID)

	mock := &mockStorageProvider{}
	storageProvider = mock
	t.Cleanup(func() { storageProvider = nil })

	for _, status := range []string{blobThumbnailStatusFailed, blobThumbnailStatusSkipped} {
		blob := seedBlob(t, db, "owner/"+status+".png", "image/png", 1, "hash-"+status)
		if err := db.Model(&models.BlobObject{}).
			Where("id = ?", blob.ID).
			Update("thumbnail_status", status).Error; err != nil {
			t.Fatalf("set thumbnail status: %v", err)
		}
		asset := models.Asset{
			ID:             uuid.New(),
			OwnerUserID:    userID,
			DocumentID:     &docID,
			BlobID:         blob.ID,
			Kind:           "image",
			Filename:       status + ".png",
			URL:            blob.URL,
			Visibility:     "private",
			Status:         "ready",
			ReferenceCount: 0,
			CreatedBy:      userID,
		}
		if err := db.Create(&asset).Error; err != nil {
			t.Fatalf("create asset: %v", err)
		}
	}

	backfilled, err := RunBlobThumbnailReconcilePass(context.Background(), 10)
	if err != nil {
		t.Fatalf("reconcile thumbnails: %v", err)
	}
	if backfilled != 0 {
		t.Fatalf("expected no terminal thumbnails backfilled, got %d", backfilled)
	}
	if len(mock.getCalls) != 0 {
		t.Fatalf("expected terminal thumbnails not to fetch storage, got %+v", mock.getCalls)
	}
}

func TestRunDueAssetGCJobs_DeletesUnusedAssets(t *testing.T) {
	db := setupMediaTestDB(t)
	userID := uuid.New()
	docID := seedOwnedDocument(t, db, userID)
	now := time.Now()

	mock := &mockStorageProvider{}
	storageProvider = mock
	t.Cleanup(func() { storageProvider = nil })

	blob := seedBlob(t, db, "owner/unused.png", "image/png", 3, "hash-unused")
	asset := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    userID,
		DocumentID:     &docID,
		BlobID:         blob.ID,
		Kind:           "image",
		Filename:       "unused.png",
		URL:            blob.URL,
		Visibility:     "private",
		Status:         "pending_delete",
		ReferenceCount: 0,
		CreatedBy:      userID,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}
	job := models.AssetGCJob{
		ID:       uuid.New(),
		AssetID:  asset.ID,
		JobType:  "delete",
		Status:   "pending",
		RunAfter: now.Add(-time.Minute),
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}

	processed, err := RunDueAssetGCJobs(context.Background(), now, 10)
	if err != nil {
		t.Fatalf("run gc jobs: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected processed=1, got %d", processed)
	}
	if len(mock.deleteCalls) != 0 {
		t.Fatalf("asset gc should not delete storage directly, got %+v", mock.deleteCalls)
	}

	var gotAsset models.Asset
	if err := db.Unscoped().First(&gotAsset, "id = ?", asset.ID).Error; err != nil {
		t.Fatalf("load asset: %v", err)
	}
	if gotAsset.Status != "deleted" || !gotAsset.DeletedAt.Valid {
		t.Fatalf("expected deleted asset, got status=%s deleted=%v", gotAsset.Status, gotAsset.DeletedAt.Valid)
	}

	var gotJob models.AssetGCJob
	if err := db.First(&gotJob, "id = ?", job.ID).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}
	if gotJob.Status != "done" || gotJob.AttemptCount != 1 {
		t.Fatalf("expected done job with attempt_count=1, got status=%s attempts=%d", gotJob.Status, gotJob.AttemptCount)
	}

	var blobJob models.BlobGCJob
	if err := db.First(&blobJob, "blob_id = ?", blob.ID).Error; err != nil {
		t.Fatalf("load blob job: %v", err)
	}
	if blobJob.Status != "pending" {
		t.Fatalf("expected pending blob job, got %s", blobJob.Status)
	}
}

func TestRunDueAssetGCJobs_CancelsWhenAssetIsReferencedAgain(t *testing.T) {
	db := setupMediaTestDB(t)
	userID := uuid.New()
	docID := seedOwnedDocument(t, db, userID)
	now := time.Now()

	mock := &mockStorageProvider{}
	storageProvider = mock
	t.Cleanup(func() { storageProvider = nil })

	blob := seedBlob(t, db, "owner/used.png", "image/png", 3, "hash-used-worker")
	asset := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    userID,
		DocumentID:     &docID,
		BlobID:         blob.ID,
		Kind:           "image",
		Filename:       "used.png",
		URL:            blob.URL,
		Visibility:     "private",
		Status:         "pending_delete",
		ReferenceCount: 0,
		CreatedBy:      userID,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}
	if err := db.Create(&models.DocumentAssetRef{
		ID:          uuid.New(),
		DocumentID:  docID,
		AssetID:     asset.ID,
		OwnerUserID: userID,
		RefType:     "editor_content",
	}).Error; err != nil {
		t.Fatalf("create ref: %v", err)
	}
	job := models.AssetGCJob{
		ID:       uuid.New(),
		AssetID:  asset.ID,
		JobType:  "delete",
		Status:   "pending",
		RunAfter: now.Add(-time.Minute),
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}

	processed, err := RunDueAssetGCJobs(context.Background(), now, 10)
	if err != nil {
		t.Fatalf("run gc jobs: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected processed=1, got %d", processed)
	}
	if len(mock.deleteCalls) != 0 {
		t.Fatalf("expected no delete calls, got %+v", mock.deleteCalls)
	}

	var gotAsset models.Asset
	if err := db.First(&gotAsset, "id = ?", asset.ID).Error; err != nil {
		t.Fatalf("load asset: %v", err)
	}
	if gotAsset.Status != "ready" || gotAsset.ReferenceCount != 1 {
		t.Fatalf("expected ready asset with ref=1, got status=%s ref=%d", gotAsset.Status, gotAsset.ReferenceCount)
	}

	var gotJob models.AssetGCJob
	if err := db.First(&gotJob, "id = ?", job.ID).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}
	if gotJob.Status != "cancelled" || gotJob.AttemptCount != 1 {
		t.Fatalf("expected cancelled job with attempt_count=1, got status=%s attempts=%d", gotJob.Status, gotJob.AttemptCount)
	}
}

func TestRunAssetReferenceReconcilePass_ReschedulesExistingPendingDeleteJob(t *testing.T) {
	db := setupMediaTestDB(t)
	userID := uuid.New()
	docID := seedOwnedDocument(t, db, userID)
	now := time.Now()

	t.Setenv("MEDIA_ASSET_DELETE_DELAY", "0s")

	blob := seedBlob(t, db, "owner/reschedule.png", "image/png", 3, "hash-reschedule")
	asset := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    userID,
		DocumentID:     &docID,
		BlobID:         blob.ID,
		Kind:           "image",
		Filename:       "reschedule.png",
		URL:            blob.URL,
		Visibility:     "private",
		Status:         "pending_delete",
		ReferenceCount: 0,
		CreatedBy:      userID,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}
	job := models.AssetGCJob{
		ID:       uuid.New(),
		AssetID:  asset.ID,
		JobType:  "delete",
		Status:   "pending",
		RunAfter: now.Add(24 * time.Hour),
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create pending job: %v", err)
	}

	reconciled, err := RunAssetReferenceReconcilePass(now, 10)
	if err != nil {
		t.Fatalf("reconcile pass: %v", err)
	}
	if reconciled != 1 {
		t.Fatalf("expected reconciled=1, got %d", reconciled)
	}

	var gotJob models.AssetGCJob
	if err := db.First(&gotJob, "id = ?", job.ID).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}
	if gotJob.RunAfter.After(now.Add(2 * time.Second)) {
		t.Fatalf("expected run_after rescheduled near now, got %s", gotJob.RunAfter)
	}
}

func TestRunDueAssetGCJobs_MarksFailedOnDeleteError(t *testing.T) {
	db := setupMediaTestDB(t)
	userID := uuid.New()
	docID := seedOwnedDocument(t, db, userID)
	now := time.Now()

	mock := &mockStorageProvider{deleteErr: errors.New("boom")}
	storageProvider = mock
	t.Cleanup(func() { storageProvider = nil })

	blob := seedBlob(t, db, "owner/broken.png", "image/png", 3, "hash-broken")
	asset := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    userID,
		DocumentID:     &docID,
		BlobID:         blob.ID,
		Kind:           "image",
		Filename:       "broken.png",
		URL:            blob.URL,
		Visibility:     "private",
		Status:         "pending_delete",
		ReferenceCount: 0,
		CreatedBy:      userID,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}
	job := models.AssetGCJob{
		ID:       uuid.New(),
		AssetID:  asset.ID,
		JobType:  "delete",
		Status:   "pending",
		RunAfter: now.Add(-time.Minute),
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}

	processed, err := RunDueAssetGCJobs(context.Background(), now, 10)
	if err != nil {
		t.Fatalf("run gc jobs: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected processed=1, got %d", processed)
	}

	var gotAsset models.Asset
	if err := db.Unscoped().First(&gotAsset, "id = ?", asset.ID).Error; err != nil {
		t.Fatalf("load asset: %v", err)
	}
	if gotAsset.Status != "deleted" || !gotAsset.DeletedAt.Valid {
		t.Fatalf("expected asset to be deleted before blob gc, got status=%s deleted=%v", gotAsset.Status, gotAsset.DeletedAt.Valid)
	}

	var gotJob models.AssetGCJob
	if err := db.First(&gotJob, "id = ?", job.ID).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}
	if gotJob.Status != "done" || gotJob.AttemptCount != 1 {
		t.Fatalf("expected done asset job with attempt_count=1, got status=%s attempts=%d", gotJob.Status, gotJob.AttemptCount)
	}

	var gotBlobJob models.BlobGCJob
	if err := db.First(&gotBlobJob, "blob_id = ?", blob.ID).Error; err != nil {
		t.Fatalf("load blob job: %v", err)
	}
	if gotBlobJob.Status != "pending" || gotBlobJob.AttemptCount != 0 {
		t.Fatalf("expected pending blob job before physical delete, got status=%s attempts=%d", gotBlobJob.Status, gotBlobJob.AttemptCount)
	}
}

func TestRunDueAssetGCJobs_MarksFailedWhenMaxAttemptsReached(t *testing.T) {
	db := setupMediaTestDB(t)
	userID := uuid.New()
	docID := seedOwnedDocument(t, db, userID)
	now := time.Now()
	t.Setenv("MEDIA_ASSET_GC_MAX_ATTEMPTS", "1")

	mock := &mockStorageProvider{deleteErr: errors.New("boom-final")}
	storageProvider = mock
	t.Cleanup(func() { storageProvider = nil })

	blob := seedBlob(t, db, "owner/broken-final.png", "image/png", 3, "hash-broken-final")
	asset := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    userID,
		DocumentID:     &docID,
		BlobID:         blob.ID,
		Kind:           "image",
		Filename:       "broken-final.png",
		URL:            blob.URL,
		Visibility:     "private",
		Status:         "pending_delete",
		ReferenceCount: 0,
		CreatedBy:      userID,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}
	job := models.AssetGCJob{
		ID:       uuid.New(),
		AssetID:  asset.ID,
		JobType:  "delete",
		Status:   "pending",
		RunAfter: now.Add(-time.Minute),
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}

	processed, err := RunDueAssetGCJobs(context.Background(), now, 10)
	if err != nil {
		t.Fatalf("run gc jobs: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected processed=1, got %d", processed)
	}

	var gotJob models.AssetGCJob
	if err := db.First(&gotJob, "id = ?", job.ID).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}
	if gotJob.Status != "done" || gotJob.AttemptCount != 1 {
		t.Fatalf("expected asset gc done with attempt_count=1, got status=%s attempts=%d", gotJob.Status, gotJob.AttemptCount)
	}

	var gotBlobJob models.BlobGCJob
	if err := db.First(&gotBlobJob, "blob_id = ?", blob.ID).Error; err != nil {
		t.Fatalf("load blob job: %v", err)
	}
	if gotBlobJob.Status != "pending" {
		t.Fatalf("expected pending blob job after asset gc, got %s", gotBlobJob.Status)
	}
}

func TestRunAssetReferenceReconcilePass_RepairsDriftedAssetState(t *testing.T) {
	db := setupMediaTestDB(t)
	userID := uuid.New()
	docID := seedOwnedDocument(t, db, userID)
	now := time.Now()

	blob := seedBlob(t, db, "owner/drifted.png", "image/png", 3, "hash-drifted")
	asset := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    userID,
		DocumentID:     &docID,
		BlobID:         blob.ID,
		Kind:           "image",
		Filename:       "drifted.png",
		URL:            blob.URL,
		Visibility:     "private",
		Status:         "ready",
		ReferenceCount: 99,
		CreatedBy:      userID,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}

	reconciled, err := RunAssetReferenceReconcilePass(now, 10)
	if err != nil {
		t.Fatalf("reconcile pass: %v", err)
	}
	if reconciled != 1 {
		t.Fatalf("expected reconciled=1, got %d", reconciled)
	}

	var gotAsset models.Asset
	if err := db.First(&gotAsset, "id = ?", asset.ID).Error; err != nil {
		t.Fatalf("load asset: %v", err)
	}
	if gotAsset.ReferenceCount != 0 || gotAsset.Status != "pending_delete" {
		t.Fatalf("expected asset repaired to pending_delete with ref=0, got status=%s ref=%d", gotAsset.Status, gotAsset.ReferenceCount)
	}

	var pendingJobs []models.AssetGCJob
	if err := db.Where("asset_id = ? AND status = ?", asset.ID, "pending").Find(&pendingJobs).Error; err != nil {
		t.Fatalf("load pending jobs: %v", err)
	}
	if len(pendingJobs) != 1 || pendingJobs[0].JobType != "delete" {
		t.Fatalf("expected one pending delete job, got %+v", pendingJobs)
	}
}
