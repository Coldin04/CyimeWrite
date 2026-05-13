package database

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/config"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"g.co1d.in/Coldin04/Cyime/server/internal/securevalue"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

const (
	databaseDirPerm  os.FileMode = 0o700
	databaseFilePerm os.FileMode = 0o600
)

// Connect initializes the database connection and runs auto-migrations.
func Connect() {
	var err error

	// Use a logger to see generated SQL
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             200 * 1000,  // Slow SQL threshold
			LogLevel:                  logger.Info, // Log level
			IgnoreRecordNotFoundError: true,        // Ignore ErrRecordNotFound error for logger
			Colorful:                  true,        // Disable color
		},
	)

	// For simplicity, we'll place the SQLite file in the user's home directory.
	// A better approach for production would be a configurable path.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get user home directory: %v", err)
	}
	dbPath := filepath.Join(homeDir, ".cyimewrite")
	if err := ensurePrivateDatabaseDir(dbPath); err != nil {
		log.Fatalf("Failed to create private database directory: %v", err)
	}
	// SQLite DSN with safe defaults:
	//   _journal_mode=WAL       — readers don't block writers and vice versa.
	//   _busy_timeout=5000      — wait up to 5s on locked db before SQLITE_BUSY.
	//   _foreign_keys=1         — enforce ON DELETE CASCADE declared in models.
	//   _synchronous=NORMAL     — durability/perf trade-off appropriate for WAL.
	//   _txlock=immediate       — acquire RESERVED lock on BEGIN to avoid
	//                             SQLITE_BUSY on transaction promotion.
	dbFile := filepath.Join(dbPath, "cyimewrite.db")
	if err := ensurePrivateDatabaseFile(dbFile); err != nil {
		log.Fatalf("Failed to create private database file: %v", err)
	}
	dsn := dbFile + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=1&_synchronous=NORMAL&_txlock=immediate"

	DB, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// SQLite serializes writers; opening multiple write connections only causes
	// SQLITE_BUSY contention. Pin the pool to a single connection so GORM does
	// not silently fan out under load.
	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatalf("Failed to access underlying *sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	// Verify foreign keys are actually enabled. mattn/go-sqlite3 will silently
	// ignore the DSN flag if compiled without the FK feature, and the rest of
	// the schema relies on ON DELETE CASCADE for cleanup, so refuse to boot if
	// they are off.
	var fkEnabled int
	if err := DB.Raw("PRAGMA foreign_keys").Scan(&fkEnabled).Error; err != nil {
		log.Fatalf("Failed to read foreign_keys pragma: %v", err)
	}
	if fkEnabled != 1 {
		log.Fatalf("SQLite foreign keys are not enabled (got %d); refusing to start", fkEnabled)
	}

	// Verify WAL is active so we don't silently fall back to rollback journal.
	var journalMode string
	if err := DB.Raw("PRAGMA journal_mode").Scan(&journalMode).Error; err != nil {
		log.Fatalf("Failed to read journal_mode pragma: %v", err)
	}
	if journalMode != "wal" && journalMode != "WAL" {
		log.Printf("Warning: SQLite journal_mode=%q (expected wal); concurrent reads may block writers", journalMode)
	}

	log.Println("Database connection established.")

	// Optional development reset (disabled by default).
	// Set RESET_WORKSPACE_TABLES_ON_BOOT=true to drop workspace/content/media tables.
	if config.IsTrue(os.Getenv("RESET_WORKSPACE_TABLES_ON_BOOT")) {
		resetTables := []string{
			"blob_gc_jobs",
			"blob_objects",
			"asset_gc_jobs",
			"assets",
			"document_asset_refs",
			"document_bodies",
			"documents",
			"folders",
			// Legacy table name from previous schema.
			"document_contents",
		}
		for _, table := range resetTables {
			if DB.Migrator().HasTable(table) {
				if err := DB.Migrator().DropTable(table); err != nil {
					log.Fatalf("Failed to drop table %s: %v", table, err)
				}
			}
		}
	}

	// The blob deduplication key used to be global on (sha256, size). Drop that
	// legacy index before AutoMigrate creates the owner-scoped replacement so
	// different users can store identical private files without sharing physical
	// blob metadata.
	if DB.Migrator().HasTable(&models.BlobObject{}) && DB.Migrator().HasIndex(&models.BlobObject{}, "idx_blob_hash_size") {
		if err := DB.Migrator().DropIndex(&models.BlobObject{}, "idx_blob_hash_size"); err != nil {
			log.Fatalf("Failed to drop legacy blob hash index: %v", err)
		}
	}

	// Auto-migrate the identity tables first so legacy refresh tokens can be
	// backfilled with session rows before UserRefreshToken's non-null session_id
	// column is applied. SQLite cannot add a NOT NULL column without a default to
	// a non-empty table, so doing this in one AutoMigrate call bricks upgrades.
	err = DB.AutoMigrate(
		&models.User{},
		&models.UserImageBedConfig{},
		&models.AuthProvider{},
		&models.UserIdentityProvider{},
		&models.UserSession{},
	)
	if err != nil {
		log.Fatalf("Failed to auto-migrate identity tables: %v", err)
	}

	if err := backfillLegacyRefreshTokenSessions(DB); err != nil {
		log.Fatalf("Failed to backfill legacy refresh token sessions: %v", err)
	}

	err = DB.AutoMigrate(
		&models.UserRefreshToken{},
		&models.Folder{},
		&models.Document{},
		&models.DocumentBody{},
		&models.DocumentPermission{},
		&models.DocumentImageTargetPreference{},
		&models.DocumentInvite{},
		&models.Notification{},
		&models.BlobObject{},
		&models.Asset{},
		&models.DocumentAssetRef{},
		&models.AssetGCJob{},
		&models.BlobGCJob{},
	)
	if err != nil {
		log.Fatalf("Failed to auto-migrate database: %v", err)
	}

	if err := encryptPlaintextAuthProviderSecrets(); err != nil {
		log.Fatalf("Failed to encrypt auth provider secrets: %v", err)
	}

	log.Println("Database migrated.")
}

func ensurePrivateDatabaseDir(path string) error {
	if err := os.MkdirAll(path, databaseDirPerm); err != nil {
		return err
	}
	return os.Chmod(path, databaseDirPerm)
}

func ensurePrivateDatabaseFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, databaseFilePerm)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Chmod(path, databaseFilePerm)
}

func encryptPlaintextAuthProviderSecrets() error {
	var providers []models.AuthProvider
	if err := DB.Find(&providers).Error; err != nil {
		return err
	}

	for _, provider := range providers {
		secret := strings.TrimSpace(provider.ClientSecretEncrypted)
		if secret == "" || strings.HasPrefix(secret, securevalue.EncryptedValuePrefix) {
			continue
		}

		encrypted, err := securevalue.EncryptString(provider.ClientSecretEncrypted)
		if err != nil {
			return err
		}
		if err := DB.Model(&models.AuthProvider{}).Where("id = ?", provider.ID).Update("client_secret_encrypted", encrypted).Error; err != nil {
			return err
		}
	}

	return nil
}

type legacyRefreshTokenSessionBackfill struct {
	ID        string
	UserID    string
	CreatedAt time.Time
}

// backfillLegacyRefreshTokenSessions upgrades databases that predate
// UserSession. The old user_refresh_tokens table had no session_id column; on
// SQLite, asking AutoMigrate to add the new non-null column to a populated table
// fails before the application can start. Add the column as nullable first,
// create one session per existing refresh token, and then let AutoMigrate finish
// creating indexes and constraints for the current model.
func backfillLegacyRefreshTokenSessions(db *gorm.DB) error {
	if db.Dialector.Name() != "sqlite" || !db.Migrator().HasTable(&models.UserRefreshToken{}) {
		return nil
	}

	hasSessionID := db.Migrator().HasColumn(&models.UserRefreshToken{}, "SessionID")
	if !hasSessionID {
		var tokenCount int64
		if err := db.Table("user_refresh_tokens").Count(&tokenCount).Error; err != nil {
			return err
		}
		if tokenCount == 0 {
			return nil
		}

		if err := db.Exec("ALTER TABLE user_refresh_tokens ADD COLUMN session_id uuid").Error; err != nil {
			return err
		}
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var tokens []legacyRefreshTokenSessionBackfill
		if err := tx.Table("user_refresh_tokens").
			Select("id, user_id, created_at").
			Where("session_id IS NULL OR session_id = ?", "").
			Find(&tokens).Error; err != nil {
			return err
		}

		for _, token := range tokens {
			var userCount int64
			if err := tx.Model(&models.User{}).Where("id = ?", token.UserID).Count(&userCount).Error; err != nil {
				return err
			}
			if userCount == 0 {
				log.Printf("Deleting legacy refresh token %s for missing user %s during session backfill", token.ID, token.UserID)
				if err := tx.Exec("DELETE FROM user_refresh_tokens WHERE id = ?", token.ID).Error; err != nil {
					return err
				}
				continue
			}

			userID, err := uuid.Parse(token.UserID)
			if err != nil {
				log.Printf("Deleting legacy refresh token %s with invalid user id %q during session backfill", token.ID, token.UserID)
				if err := tx.Exec("DELETE FROM user_refresh_tokens WHERE id = ?", token.ID).Error; err != nil {
					return err
				}
				continue
			}

			lastSeenAt := token.CreatedAt
			if lastSeenAt.IsZero() {
				lastSeenAt = time.Now()
			}
			session := models.UserSession{
				UserID:      userID,
				UserAgent:   "",
				DeviceLabel: "Legacy session",
				LastSeenAt:  lastSeenAt,
			}
			if err := tx.Create(&session).Error; err != nil {
				return err
			}
			if err := tx.Table("user_refresh_tokens").Where("id = ?", token.ID).Update("session_id", session.ID).Error; err != nil {
				return err
			}
		}

		var missing int64
		if err := tx.Table("user_refresh_tokens").Where("session_id IS NULL OR session_id = ?", "").Count(&missing).Error; err != nil {
			return err
		}
		if missing != 0 {
			return errors.New("legacy refresh token session backfill left tokens without session_id")
		}
		return nil
	})
}
