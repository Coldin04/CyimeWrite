package config

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
)

func trimURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

// GetPublicBaseURL returns the externally reachable frontend/base URL.
func GetPublicBaseURL() string {
	if value := trimURL(os.Getenv("PUBLIC_BASE_URL")); value != "" {
		return value
	}
	if value := trimURL(os.Getenv("FRONTEND_BASE_URL")); value != "" {
		return value
	}
	return "http://localhost:5173"
}

// GetPublicAPIBaseURL returns the externally reachable API origin.
func GetPublicAPIBaseURL() string {
	if value := trimURL(os.Getenv("PUBLIC_API_BASE_URL")); value != "" {
		return value
	}
	if value := trimURL(os.Getenv("API_BASE_URL")); value != "" {
		return value
	}

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}
	port = strings.TrimPrefix(port, ":")
	return fmt.Sprintf("http://localhost:%s", port)
}

func JoinPublicAPIURL(segments ...string) (string, error) {
	base, err := url.Parse(GetPublicAPIBaseURL())
	if err != nil {
		return "", err
	}

	parts := []string{base.Path}
	for _, segment := range segments {
		if trimmed := strings.Trim(segment, "/"); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	base.Path = path.Join(parts...)
	if len(segments) > 0 && strings.HasSuffix(segments[len(segments)-1], "/") {
		base.Path += "/"
	}

	return base.String(), nil
}
