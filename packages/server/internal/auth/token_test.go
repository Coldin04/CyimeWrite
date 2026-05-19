package auth

import (
	"testing"
	"time"
)

const tokenServiceTestSecret = "token-service-test-secret-aaaaaaaaaaaaaaaa"

func TestNewTokenService_DefaultsAccessTokenLifetimeToFifteenMinutes(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", tokenServiceTestSecret)
	t.Setenv("ACCESS_TOKEN_LIFETIME_MINUTES", "")
	t.Setenv("ACCESS_TOKEN_LIFETIME_SECONDS", "")

	svc, err := NewTokenService()
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	if svc.accessTokenLifetime != 15*time.Minute {
		t.Fatalf("expected 15m access token lifetime, got %s", svc.accessTokenLifetime)
	}
}

func TestNewTokenService_UsesMinuteAccessTokenLifetime(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", tokenServiceTestSecret)
	t.Setenv("ACCESS_TOKEN_LIFETIME_MINUTES", "2")
	t.Setenv("ACCESS_TOKEN_LIFETIME_SECONDS", "")

	svc, err := NewTokenService()
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	if svc.accessTokenLifetime != 2*time.Minute {
		t.Fatalf("expected 2m access token lifetime, got %s", svc.accessTokenLifetime)
	}
}

func TestNewTokenService_SecondsAccessTokenLifetimeOverridesMinutes(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", tokenServiceTestSecret)
	t.Setenv("ACCESS_TOKEN_LIFETIME_MINUTES", "15")
	t.Setenv("ACCESS_TOKEN_LIFETIME_SECONDS", "20")

	svc, err := NewTokenService()
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	if svc.accessTokenLifetime != 20*time.Second {
		t.Fatalf("expected 20s access token lifetime, got %s", svc.accessTokenLifetime)
	}
}
