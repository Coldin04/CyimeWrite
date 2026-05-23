package config

import "testing"

func TestGetPublicAPIBaseURLPrecedence(t *testing.T) {
	t.Setenv("PUBLIC_API_BASE_URL", "https://api.example.test/")
	t.Setenv("API_BASE_URL", "https://internal.example.test")
	t.Setenv("PORT", "9000")

	if got := GetPublicAPIBaseURL(); got != "https://api.example.test" {
		t.Fatalf("GetPublicAPIBaseURL() = %q", got)
	}
}

func TestGetPublicAPIBaseURLFallback(t *testing.T) {
	t.Setenv("PUBLIC_API_BASE_URL", "")
	t.Setenv("API_BASE_URL", "")
	t.Setenv("PORT", ":9000")

	if got := GetPublicAPIBaseURL(); got != "http://localhost:9000" {
		t.Fatalf("GetPublicAPIBaseURL() = %q", got)
	}
}

func TestJoinPublicAPIURL(t *testing.T) {
	t.Setenv("PUBLIC_API_BASE_URL", "https://example.test/base/")

	got, err := JoinPublicAPIURL("/api/v1/", "open", "/files")
	if err != nil {
		t.Fatalf("JoinPublicAPIURL returned error: %v", err)
	}
	want := "https://example.test/base/api/v1/open/files"
	if got != want {
		t.Fatalf("JoinPublicAPIURL() = %q, want %q", got, want)
	}
}
