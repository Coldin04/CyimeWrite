package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMarkdownToContentJSONAndBack(t *testing.T) {
	t.Setenv("MARKDOWN_CONVERTER_URL", "")

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
