package apitoken

import "testing"

func TestNormalizeScopesDeduplicatesAndRejectsUnknown(t *testing.T) {
	scopes, err := NormalizeScopes([]string{
		" " + ScopeWorkspaceRead + " ",
		ScopeWorkspaceRead,
		ScopeDocumentWrite,
	})
	if err != nil {
		t.Fatalf("NormalizeScopes returned error: %v", err)
	}
	if len(scopes) != 2 {
		t.Fatalf("len(scopes) = %d, want 2", len(scopes))
	}
	if scopes[0] != ScopeWorkspaceRead || scopes[1] != ScopeDocumentWrite {
		t.Fatalf("scopes = %#v", scopes)
	}

	if _, err := NormalizeScopes([]string{"admin:all"}); err == nil {
		t.Fatal("expected unknown scope to fail")
	}
}

func TestEncodeDecodeScopes(t *testing.T) {
	encoded, err := EncodeScopes([]string{ScopeWorkspaceRead, ScopeFileCopy, ScopeFileDelete})
	if err != nil {
		t.Fatalf("EncodeScopes returned error: %v", err)
	}

	decoded, err := DecodeScopes(encoded)
	if err != nil {
		t.Fatalf("DecodeScopes returned error: %v", err)
	}
	if !HasScopes(decoded, ScopeWorkspaceRead, ScopeFileCopy, ScopeFileDelete) {
		t.Fatalf("decoded scopes %#v do not include expected scopes", decoded)
	}
	if HasScopes(decoded, ScopeDocumentWrite) {
		t.Fatalf("decoded scopes %#v unexpectedly include %s", decoded, ScopeDocumentWrite)
	}
}
