package securevalue

import (
	"strings"
	"testing"
)

const strongEncryptionKey = "f3a4d6e7c1b2a8d9e0f1a2b3c4d5e6f70a1b2c3d"

func TestEncryptDecryptString_RoundTrip(t *testing.T) {
	t.Setenv("APP_ENCRYPTION_KEY", strongEncryptionKey)

	encrypted, err := EncryptString("secret-value")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if encrypted == "secret-value" {
		t.Fatalf("expected encrypted value to differ from plaintext")
	}

	decrypted, err := DecryptString(encrypted)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decrypted != "secret-value" {
		t.Fatalf("unexpected decrypted value: %q", decrypted)
	}
}

func TestDecryptString_RejectsInvalidFormat(t *testing.T) {
	t.Setenv("APP_ENCRYPTION_KEY", strongEncryptionKey)

	if _, err := DecryptString("plain-text"); err == nil {
		t.Fatalf("expected invalid format error")
	}
}

func TestValidateEncryptionKey_RejectsKnownDefaultAppKey(t *testing.T) {
	t.Setenv("APP_ENCRYPTION_KEY", "replace-with-a-strong-secret")
	t.Setenv("JWT_SECRET_KEY", strongEncryptionKey)

	err := ValidateEncryptionKey()
	if err == nil {
		t.Fatal("expected known default APP_ENCRYPTION_KEY to be rejected")
	}
	if !strings.Contains(err.Error(), "insecure default") {
		t.Fatalf("expected insecure default error, got %v", err)
	}
}

func TestValidateEncryptionKey_RejectsShortAppKey(t *testing.T) {
	t.Setenv("APP_ENCRYPTION_KEY", "short-secret")
	t.Setenv("JWT_SECRET_KEY", strongEncryptionKey)

	err := ValidateEncryptionKey()
	if err == nil {
		t.Fatal("expected short APP_ENCRYPTION_KEY to be rejected")
	}
	if !strings.Contains(err.Error(), "at least") {
		t.Fatalf("expected length error, got %v", err)
	}
}

func TestValidateEncryptionKey_FallsBackToStrongJWTSecret(t *testing.T) {
	t.Setenv("APP_ENCRYPTION_KEY", "")
	t.Setenv("JWT_SECRET_KEY", strongEncryptionKey)

	if err := ValidateEncryptionKey(); err != nil {
		t.Fatalf("unexpected fallback validation error: %v", err)
	}
}
