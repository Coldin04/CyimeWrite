package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMarkdownToContentJSONAndBack(t *testing.T) {
	t.Setenv("MARKDOWN_CONVERTER_URL", "")
	t.Setenv("MARKDOWN_CONVERTER_FALLBACK", "true")

	raw, err := markdownToContentJSON("# Title\n\nHello world\n\n- one\n- two\n\n```go\nfmt.Println(1)\n```")
	if err != nil {
		t.Fatalf("markdownToContentJSON returned error: %v", err)
	}

	var doc docNode
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if doc.Type != "doc" || len(doc.Content) != 4 {
		t.Fatalf("unexpected doc: %#v", doc)
	}

	markdown, err := contentJSONToMarkdown(raw)
	if err != nil {
		t.Fatalf("contentJSONToMarkdown returned error: %v", err)
	}
	for _, expected := range []string{"# Title", "Hello world", "- one", "```go"} {
		if !strings.Contains(markdown, expected) {
			t.Fatalf("expected markdown to contain %q, got:\n%s", expected, markdown)
		}
	}
}

func TestLegacyMarkdownPreservesPrivateImageMarkers(t *testing.T) {
	t.Setenv("MARKDOWN_CONVERTER_URL", "")
	t.Setenv("MARKDOWN_CONVERTER_FALLBACK", "true")

	assetID := "11111111-2222-3333-4444-555555555555"
	raw, err := markdownToContentJSON("![cover](cyime-asset:" + assetID + ` "Cover")`)
	if err != nil {
		t.Fatalf("markdownToContentJSON returned error: %v", err)
	}

	var doc docNode
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Type != "image" {
		t.Fatalf("expected one image node, got %#v", doc.Content)
	}
	if got, _ := doc.Content[0].Attrs["assetId"].(string); got != assetID {
		t.Fatalf("assetId = %q, want %q", got, assetID)
	}
	if _, ok := doc.Content[0].Attrs["src"]; ok {
		t.Fatalf("private image marker should not be stored as src: %#v", doc.Content[0].Attrs)
	}

	markdown, err := contentJSONToMarkdown(raw)
	if err != nil {
		t.Fatalf("contentJSONToMarkdown returned error: %v", err)
	}
	if !strings.Contains(markdown, "![cover](cyime-asset:"+assetID+` "Cover")`) {
		t.Fatalf("private image marker was not preserved:\n%s", markdown)
	}
}

func TestApplyMarkdownPatchReplaceSection(t *testing.T) {
	input := "# A\n\nold\n\n## Child\n\nchild\n\n# B\n\nkeep"
	output, err := applyMarkdownPatch(input, []PatchOperation{{
		Type:    "replace",
		Target:  "section",
		Heading: "A",
		Content: "new",
	}})
	if err != nil {
		t.Fatalf("applyMarkdownPatch returned error: %v", err)
	}
	if !strings.Contains(output, "# A\n\nnew\n\n# B\n\nkeep") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	if strings.Contains(output, "old") || strings.Contains(output, "Child") {
		t.Fatalf("section content was not replaced:\n%s", output)
	}
}
