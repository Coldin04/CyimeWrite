package apitoken

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	ScopeWorkspaceRead  = "workspace:read"
	ScopeWorkspaceWrite = "workspace:write"
	ScopeDocumentRead   = "document:read"
	ScopeDocumentWrite  = "document:write"
	ScopeFileMove       = "file:move"
	ScopeFileCopy       = "file:copy"
)

var allowedScopes = []string{
	ScopeWorkspaceRead,
	ScopeWorkspaceWrite,
	ScopeDocumentRead,
	ScopeDocumentWrite,
	ScopeFileMove,
	ScopeFileCopy,
}

func NormalizeScopes(scopes []string) ([]string, error) {
	seen := make(map[string]struct{}, len(scopes))
	normalized := make([]string, 0, len(scopes))

	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if !slices.Contains(allowedScopes, scope) {
			return nil, fmt.Errorf("unsupported scope: %s", scope)
		}
		if _, exists := seen[scope]; exists {
			continue
		}
		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}

	if len(normalized) == 0 {
		return nil, fmt.Errorf("at least one scope is required")
	}
	return normalized, nil
}

func EncodeScopes(scopes []string) (string, error) {
	normalized, err := NormalizeScopes(scopes)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func DecodeScopes(raw string) ([]string, error) {
	var scopes []string
	if err := json.Unmarshal([]byte(raw), &scopes); err != nil {
		return nil, err
	}
	return NormalizeScopes(scopes)
}

func HasScopes(granted []string, required ...string) bool {
	for _, scope := range required {
		if !slices.Contains(granted, scope) {
			return false
		}
	}
	return true
}
