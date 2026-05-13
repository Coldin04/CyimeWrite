package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"g.co1d.in/Coldin04/Cyime/server/internal/securevalue"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const testEncryptionKey = "f3a4d6e7c1b2a8d9e0f1a2b3c4d5e6f70a1b2c3d"

// TestConnect_AppliesSafeSQLitePragmas runs the production Connect path against
// a temp HOME and verifies that the DSN actually delivered the safety options
// we requested. mattn/go-sqlite3 silently drops unsupported pragma flags, so
// the assertions below are the only thing that proves the fix is real.
func TestConnect_AppliesSafeSQLitePragmas(t *testing.T) {
	if runtime.GOOS == "windows" {
		// os.UserHomeDir on Windows reads USERPROFILE; this test only fakes HOME.
		t.Skip("HOME redirection skipped on windows")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("APP_ENCRYPTION_KEY", testEncryptionKey)

	// Connect calls log.Fatalf on failure, which would terminate the test
	// process. This test therefore only exercises the success path and then
	// proves the resulting DB handle actually has the requested pragmas.
	Connect()
	t.Cleanup(func() {
		if DB != nil {
			if sqlDB, err := DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
	})

	if DB == nil {
		t.Fatal("DB was not initialised")
	}

	var fk int
	if err := DB.Raw("PRAGMA foreign_keys").Scan(&fk).Error; err != nil {
		t.Fatalf("read foreign_keys pragma: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}

	var journal string
	if err := DB.Raw("PRAGMA journal_mode").Scan(&journal).Error; err != nil {
		t.Fatalf("read journal_mode pragma: %v", err)
	}
	if journal != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journal)
	}

	var busy int
	if err := DB.Raw("PRAGMA busy_timeout").Scan(&busy).Error; err != nil {
		t.Fatalf("read busy_timeout pragma: %v", err)
	}
	if busy < 5000 {
		t.Fatalf("busy_timeout = %d, want >= 5000", busy)
	}
}

func TestConnect_EnforcesPrivateDatabasePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission checks skipped on windows")
	}
	home := t.TempDir()
	dbDir := filepath.Join(home, ".cyimewrite")
	if err := os.MkdirAll(dbDir, 0o777); err != nil {
		t.Fatalf("create database dir: %v", err)
	}
	dbFile := filepath.Join(dbDir, "cyimewrite.db")
	if err := os.WriteFile(dbFile, nil, 0o666); err != nil {
		t.Fatalf("create database file: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("APP_ENCRYPTION_KEY", testEncryptionKey)
	Connect()
	t.Cleanup(func() {
		if DB != nil {
			if sqlDB, err := DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
	})

	dirInfo, err := os.Stat(dbDir)
	if err != nil {
		t.Fatalf("stat database dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("database dir mode = %o, want 700", got)
	}

	fileInfo, err := os.Stat(dbFile)
	if err != nil {
		t.Fatalf("stat database file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("database file mode = %o, want 600", got)
	}
}

func TestConnect_EncryptsExistingPlaintextAuthProviderSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME redirection skipped on windows")
	}
	home := t.TempDir()
	dbDir := filepath.Join(home, ".cyimewrite")
	if err := os.MkdirAll(dbDir, 0o700); err != nil {
		t.Fatalf("create database dir: %v", err)
	}
	dbFile := filepath.Join(dbDir, "cyimewrite.db")
	seedDB, err := gorm.Open(sqlite.Open(dbFile), &gorm.Config{})
	if err != nil {
		t.Fatalf("open seed sqlite: %v", err)
	}
	if err := seedDB.AutoMigrate(&models.AuthProvider{}); err != nil {
		t.Fatalf("seed migrate: %v", err)
	}
	provider := models.AuthProvider{
		ID:                    uuid.New(),
		Name:                  "github",
		ProtocolType:          "oauth2",
		ClientID:              "client-id",
		ClientSecretEncrypted: "plaintext-secret",
		Scopes:                "read:user",
		IsActive:              true,
	}
	if err := seedDB.Create(&provider).Error; err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	seedSQLDB, err := seedDB.DB()
	if err != nil {
		t.Fatalf("seed sql db: %v", err)
	}
	if err := seedSQLDB.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("APP_ENCRYPTION_KEY", testEncryptionKey)
	Connect()
	t.Cleanup(func() {
		if DB != nil {
			if sqlDB, err := DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
	})

	var stored models.AuthProvider
	if err := DB.First(&stored, "id = ?", provider.ID).Error; err != nil {
		t.Fatalf("load provider: %v", err)
	}
	if !strings.HasPrefix(stored.ClientSecretEncrypted, securevalue.EncryptedValuePrefix) {
		t.Fatalf("secret was not encrypted: %q", stored.ClientSecretEncrypted)
	}
	decrypted, err := securevalue.DecryptString(stored.ClientSecretEncrypted)
	if err != nil {
		t.Fatalf("decrypt stored secret: %v", err)
	}
	if decrypted != "plaintext-secret" {
		t.Fatalf("decrypted secret = %q, want plaintext-secret", decrypted)
	}
}

// TestConnect_CascadeDeletesUserSessions verifies that the foreign-key fix
// actually causes the OnDelete:CASCADE declared on UserSession to fire. This
// is the regression test for the silent-cascade-loss part of bug P0-#2.
func TestConnect_CascadeDeletesUserSessions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME redirection skipped on windows")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("APP_ENCRYPTION_KEY", testEncryptionKey)

	Connect()
	t.Cleanup(func() {
		if DB != nil {
			if sqlDB, err := DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
	})

	email := "cascade@example.com"
	user := models.User{
		ID:    uuid.New(),
		Email: &email,
	}
	if err := DB.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	session := models.UserSession{
		ID:         uuid.New(),
		UserID:     user.ID,
		LastSeenAt: time.Now(),
	}
	if err := DB.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}

	if err := DB.Unscoped().Delete(&user).Error; err != nil {
		t.Fatalf("delete user: %v", err)
	}

	var remaining int64
	if err := DB.Unscoped().Model(&models.UserSession{}).Where("id = ?", session.ID).Count(&remaining).Error; err != nil {
		t.Fatalf("count remaining sessions: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected user session to cascade-delete, %d rows still present", remaining)
	}
}

// TestConnect_BackfillsLegacyRefreshTokenSessions verifies that upgrades from
// the pre-session schema do not fail when existing refresh tokens are present.
func TestConnect_BackfillsLegacyRefreshTokenSessions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME redirection skipped on windows")
	}

	home := t.TempDir()
	dbDir := filepath.Join(home, ".cyimewrite")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("create db dir: %v", err)
	}

	legacyDB, err := sql.Open("sqlite3", filepath.Join(dbDir, "cyimewrite.db"))
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}

	userID := uuid.New().String()
	tokenID := uuid.New().String()
	createdAt := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	legacySQL := []string{
		`CREATE TABLE users (id uuid PRIMARY KEY, email text UNIQUE, email_verified numeric NOT NULL DEFAULT false, created_at datetime, updated_at datetime)`,
		`CREATE TABLE user_refresh_tokens (id uuid PRIMARY KEY, user_id uuid NOT NULL, token_hash varchar(255) NOT NULL, expires_at datetime NOT NULL, created_at datetime)`,
		`CREATE UNIQUE INDEX idx_user_refresh_tokens_token_hash ON user_refresh_tokens(token_hash)`,
		`INSERT INTO users (id, email, email_verified, created_at, updated_at) VALUES (?, 'legacy@example.com', false, ?, ?)`,
		`INSERT INTO user_refresh_tokens (id, user_id, token_hash, expires_at, created_at) VALUES (?, ?, 'legacy-token-hash', ?, ?)`,
	}
	for i, stmt := range legacySQL {
		var err error
		switch i {
		case 3:
			_, err = legacyDB.Exec(stmt, userID, createdAt, createdAt)
		case 4:
			_, err = legacyDB.Exec(stmt, tokenID, userID, time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), createdAt)
		default:
			_, err = legacyDB.Exec(stmt)
		}
		if err != nil {
			_ = legacyDB.Close()
			t.Fatalf("run legacy sql %d: %v", i, err)
		}
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("APP_ENCRYPTION_KEY", testEncryptionKey)
	Connect()
	t.Cleanup(func() {
		if DB != nil {
			if sqlDB, err := DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
	})

	var token models.UserRefreshToken
	if err := DB.First(&token, "id = ?", tokenID).Error; err != nil {
		t.Fatalf("load backfilled refresh token: %v", err)
	}
	if token.SessionID == uuid.Nil {
		t.Fatal("refresh token session_id was not backfilled")
	}

	var sessions int64
	if err := DB.Model(&models.UserSession{}).Where("id = ? AND user_id = ?", token.SessionID, userID).Count(&sessions).Error; err != nil {
		t.Fatalf("count backfilled sessions: %v", err)
	}
	if sessions != 1 {
		t.Fatalf("backfilled sessions = %d, want 1", sessions)
	}
}
