package ai

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteMarkdownConverterUsesInternalService(t *testing.T) {
	t.Setenv("MARKDOWN_CONVERTER_TOKEN", "test-secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"contentJson":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"converted"}]}]}}`))
	}))
	defer server.Close()
	t.Setenv("MARKDOWN_CONVERTER_URL", server.URL)

	raw, err := markdownToContentJSON("# Title")
	if err != nil {
		t.Fatalf("markdownToContentJSON returned error: %v", err)
	}
	if !strings.Contains(string(raw), "converted") {
		t.Fatalf("expected remote converter content, got %s", string(raw))
	}
}

func TestRemoteMarkdownConverterFailureDoesNotFallbackByDefault(t *testing.T) {
	t.Setenv("MARKDOWN_CONVERTER_TOKEN", "test-secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"down"}`))
	}))
	defer server.Close()
	t.Setenv("MARKDOWN_CONVERTER_URL", server.URL)
	t.Setenv("MARKDOWN_CONVERTER_FALLBACK", "")

	_, err := markdownToContentJSON("# Title")
	if !errors.Is(err, ErrMarkdownConverterUnavailable) {
		t.Fatalf("expected ErrMarkdownConverterUnavailable, got %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "document was not changed") {
		t.Fatalf("expected user-facing no-write message, got %q", err.Error())
	}
}

func TestRemoteMarkdownConverterExplicitFallbackUsesLegacyConverter(t *testing.T) {
	t.Setenv("MARKDOWN_CONVERTER_TOKEN", "test-secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	t.Setenv("MARKDOWN_CONVERTER_URL", server.URL)
	t.Setenv("MARKDOWN_CONVERTER_FALLBACK", "true")

	raw, err := markdownToContentJSON("# Title")
	if err != nil {
		t.Fatalf("markdownToContentJSON returned error: %v", err)
	}
	if !strings.Contains(string(raw), `"type":"heading"`) {
		t.Fatalf("expected legacy converter content, got %s", string(raw))
	}
}

func TestNormalizeMarkdownConverterURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "origin",
			raw:  "http://127.0.0.1:5173",
			want: "http://127.0.0.1:5173/markdown/convert",
		},
		{
			name: "origin slash",
			raw:  "http://127.0.0.1:5173/",
			want: "http://127.0.0.1:5173/markdown/convert",
		},
		{
			name: "full endpoint",
			raw:  "http://127.0.0.1:5173/markdown/convert",
			want: "http://127.0.0.1:5173/markdown/convert",
		},
		{
			name: "custom path",
			raw:  "http://127.0.0.1:5173/internal/convert",
			want: "http://127.0.0.1:5173/internal/convert",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeMarkdownConverterURL(tt.raw); got != tt.want {
				t.Fatalf("normalizeMarkdownConverterURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
