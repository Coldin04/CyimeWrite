package ai

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

type docNode struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Text    string         `json:"text,omitempty"`
	Marks   []markNode     `json:"marks,omitempty"`
	Content []docNode      `json:"content,omitempty"`
}

type markNode struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

var orderedListPattern = regexp.MustCompile(`^\s*\d+\.\s+(.+)$`)

func legacyMarkdownToContentJSON(markdown string) ([]byte, error) {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	blocks := make([]docNode, 0, len(lines))

	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "```") {
			language := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			codeLines := make([]string, 0)
			i++
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				codeLines = append(codeLines, lines[i])
				i++
			}
			if i < len(lines) {
				i++
			}
			attrs := map[string]any{}
			if language != "" {
				attrs["language"] = language
			}
			blocks = append(blocks, docNode{
				Type:    "codeBlock",
				Attrs:   attrs,
				Content: textContent(strings.Join(codeLines, "\n")),
			})
			continue
		}

		if level, text, ok := parseHeading(trimmed); ok {
			blocks = append(blocks, docNode{
				Type:    "heading",
				Attrs:   map[string]any{"level": level},
				Content: textContent(text),
			})
			i++
			continue
		}

		if isBulletLine(trimmed) {
			items := make([]docNode, 0)
			for i < len(lines) && isBulletLine(strings.TrimSpace(lines[i])) {
				items = append(items, listItem(strings.TrimSpace(lines[i])[2:]))
				i++
			}
			blocks = append(blocks, docNode{Type: "bulletList", Content: items})
			continue
		}

		if matches := orderedListPattern.FindStringSubmatch(trimmed); len(matches) == 2 {
			items := make([]docNode, 0)
			for i < len(lines) {
				matches := orderedListPattern.FindStringSubmatch(strings.TrimSpace(lines[i]))
				if len(matches) != 2 {
					break
				}
				items = append(items, listItem(matches[1]))
				i++
			}
			blocks = append(blocks, docNode{Type: "orderedList", Content: items})
			continue
		}

		paragraphLines := []string{trimmed}
		i++
		for i < len(lines) {
			next := strings.TrimSpace(lines[i])
			if next == "" || strings.HasPrefix(next, "```") || isBulletLine(next) || orderedListPattern.MatchString(next) {
				break
			}
			if _, _, ok := parseHeading(next); ok {
				break
			}
			paragraphLines = append(paragraphLines, next)
			i++
		}
		blocks = append(blocks, docNode{
			Type:    "paragraph",
			Content: textContent(strings.Join(paragraphLines, " ")),
		})
	}

	if len(blocks) == 0 {
		blocks = append(blocks, docNode{Type: "paragraph"})
	}

	return json.Marshal(docNode{Type: "doc", Content: blocks})
}

func legacyContentJSONToMarkdown(raw []byte) (string, error) {
	var doc docNode
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", err
	}

	blocks := make([]string, 0, len(doc.Content))
	for _, child := range doc.Content {
		rendered := renderBlock(child)
		if strings.TrimSpace(rendered) != "" {
			blocks = append(blocks, rendered)
		}
	}
	return strings.TrimSpace(strings.Join(blocks, "\n\n")), nil
}

func parseHeading(line string) (int, string, bool) {
	count := 0
	for count < len(line) && line[count] == '#' {
		count++
	}
	if count == 0 || count > 6 || count >= len(line) || line[count] != ' ' {
		return 0, "", false
	}
	return count, strings.TrimSpace(line[count+1:]), true
}

func isBulletLine(line string) bool {
	return strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ")
}

func listItem(text string) docNode {
	return docNode{
		Type: "listItem",
		Content: []docNode{{
			Type:    "paragraph",
			Content: textContent(strings.TrimSpace(text)),
		}},
	}
}

func textContent(text string) []docNode {
	if text == "" {
		return nil
	}
	return []docNode{{Type: "text", Text: text}}
}

func renderBlock(node docNode) string {
	switch node.Type {
	case "heading":
		level := 1
		if rawLevel, ok := node.Attrs["level"]; ok {
			switch value := rawLevel.(type) {
			case float64:
				level = int(value)
			case int:
				level = value
			}
		}
		if level < 1 || level > 6 {
			level = 1
		}
		return strings.Repeat("#", level) + " " + renderInline(node.Content)
	case "paragraph":
		return renderInline(node.Content)
	case "codeBlock":
		language, _ := node.Attrs["language"].(string)
		return "```" + language + "\n" + plainText(node.Content) + "\n```"
	case "bulletList":
		lines := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			lines = append(lines, "- "+strings.TrimSpace(renderListItem(item)))
		}
		return strings.Join(lines, "\n")
	case "orderedList":
		lines := make([]string, 0, len(node.Content))
		for index, item := range node.Content {
			lines = append(lines, strconv.Itoa(index+1)+". "+strings.TrimSpace(renderListItem(item)))
		}
		return strings.Join(lines, "\n")
	case "blockquote":
		text := strings.TrimSpace(renderChildren(node.Content))
		lines := strings.Split(text, "\n")
		for i := range lines {
			lines[i] = "> " + lines[i]
		}
		return strings.Join(lines, "\n")
	default:
		return renderChildren(node.Content)
	}
}

func renderChildren(nodes []docNode) string {
	parts := make([]string, 0, len(nodes))
	for _, child := range nodes {
		parts = append(parts, renderBlock(child))
	}
	return strings.Join(parts, "\n\n")
}

func renderListItem(node docNode) string {
	if node.Type != "listItem" {
		return renderBlock(node)
	}
	parts := make([]string, 0, len(node.Content))
	for _, child := range node.Content {
		parts = append(parts, renderBlock(child))
	}
	return strings.Join(parts, " ")
}

func renderInline(nodes []docNode) string {
	var builder strings.Builder
	for _, node := range nodes {
		if node.Type == "text" {
			builder.WriteString(renderMarkedText(node))
			continue
		}
		builder.WriteString(renderBlock(node))
	}
	return builder.String()
}

func renderMarkedText(node docNode) string {
	text := node.Text
	for _, mark := range node.Marks {
		switch mark.Type {
		case "bold":
			text = "**" + text + "**"
		case "italic":
			text = "*" + text + "*"
		case "code":
			text = "`" + text + "`"
		case "link":
			if href, ok := mark.Attrs["href"].(string); ok && strings.TrimSpace(href) != "" {
				text = "[" + text + "](" + href + ")"
			}
		}
	}
	return text
}

func plainText(nodes []docNode) string {
	var builder strings.Builder
	for _, node := range nodes {
		if node.Text != "" {
			builder.WriteString(node.Text)
		}
		builder.WriteString(plainText(node.Content))
	}
	return builder.String()
}
