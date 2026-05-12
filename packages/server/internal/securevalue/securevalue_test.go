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

func TestValidateEncryptionKey_RejectsPublicSampleKeys(t *testing.T) {
	publicKeys := []string{
		"f619a2942a188928414afd7e97fc6072c1c21905a723301749f13150bdd57612",
		"5619a079a803a895e1ced94f5a759dd12dd3df7c06a9355c11c12ed9805b6da9",
		"45d80dcef35bf1009602b9baa57c091daa5a307f3f275b7f510f6df18c2475bb",
	}

	for _, key := range publicKeys {
		t.Run(key, func(t *testing.T) {
			t.Setenv("APP_ENCRYPTION_KEY", key)
			t.Setenv("JWT_SECRET_KEY", strongEncryptionKey)

			err := ValidateEncryptionKey()
			if err == nil {
				t.Fatal("expected public sample APP_ENCRYPTION_KEY to be rejected")
			}
			if !strings.Contains(err.Error(), "insecure default") {
				t.Fatalf("expected insecure default error, got %v", err)
			}
		})
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
