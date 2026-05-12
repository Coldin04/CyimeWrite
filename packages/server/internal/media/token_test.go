package media

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIssueAndVerifyAssetReadToken(t *testing.T) {
	t.Setenv("MEDIA_TOKEN_SECRET", "test-media-secret")
	t.Setenv("MEDIA_SIGN_TTL_SECONDS", "60")

	svc, err := NewTokenService()
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	assetID := uuid.New()
	userID := uuid.New()

	token, _, err := svc.IssueAssetReadToken(assetID, userID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	claims, err := svc.VerifyAssetReadToken(token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}

	if claims.AssetID != assetID {
		t.Fatalf("asset id mismatch: got %s", claims.AssetID)
	}
	if claims.UserID != userID {
		t.Fatalf("user id mismatch: got %s", claims.UserID)
	}
}

func TestAvatarReadTokenUsesShortDefaultTTL(t *testing.T) {
	t.Setenv("MEDIA_TOKEN_SECRET", "test-media-secret")
	t.Setenv("MEDIA_AVATAR_SIGN_TTL_SECONDS", "")

	svc, err := NewTokenService()
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	if svc.avatarSignTTL != defaultSignTTLSeconds*time.Second {
		t.Fatalf("expected avatar TTL to match short default, got %s", svc.avatarSignTTL)
	}
}

func TestNewTokenServiceRequiresSecret(t *testing.T) {
	// Ensure fallback is also absent.
	t.Setenv("MEDIA_TOKEN_SECRET", "")
	t.Setenv("JWT_SECRET_KEY", "")

	if _, err := NewTokenService(); err == nil {
		t.Fatal("expected missing secret error")
	}
}
