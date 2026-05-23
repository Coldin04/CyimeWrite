package ai

import (
	"fmt"
	"regexp"
	"strings"
)

type PatchOperation struct {
	Type    string `json:"type"`
	Target  string `json:"target"`
	Heading string `json:"heading"`
	Match   string `json:"match"`
	Content string `json:"content"`
}

var markdownHeadingPattern = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+?)\s*$`)

func applyMarkdownPatch(markdown string, operations []PatchOperation) (string, error) {
	next := strings.ReplaceAll(markdown, "\r\n", "\n")
	for _, operation := range operations {
		updated, err := applyMarkdownOperation(next, operation)
		if err != nil {
			return "", err
		}
		next = updated
	}
	return strings.TrimSpace(next), nil
}

func applyMarkdownOperation(markdown string, operation PatchOperation) (string, error) {
	content := strings.Trim(operation.Content, "\n")
	switch operation.Type {
	case "append":
		if strings.TrimSpace(operation.Heading) != "" {
			return insertInSection(markdown, operation.Heading, "\n\n"+content, false)
		}
		return strings.TrimSpace(markdown) + "\n\n" + content, nil
	case "prepend":
		if strings.TrimSpace(operation.Heading) != "" {
			return insertInSection(markdown, operation.Heading, content+"\n\n", true)
		}
		return content + "\n\n" + strings.TrimSpace(markdown), nil
	case "replace":
		if operation.Target == "section" || strings.TrimSpace(operation.Heading) != "" {
			return replaceSection(markdown, operation.Heading, content)
		}
		if strings.TrimSpace(operation.Match) == "" {
			return "", fmt.Errorf("replace operation requires heading or match")
		}
		if !strings.Contains(markdown, operation.Match) {
			return "", fmt.Errorf("replace match not found")
		}
		return strings.Replace(markdown, operation.Match, content, 1), nil
	case "insert_after":
		if strings.TrimSpace(operation.Heading) != "" {
			return insertAfterSection(markdown, operation.Heading, content)
		}
		if strings.TrimSpace(operation.Match) == "" {
			return "", fmt.Errorf("insert_after operation requires heading or match")
		}
		index := strings.Index(markdown, operation.Match)
		if index < 0 {
			return "", fmt.Errorf("insert_after match not found")
		}
		insertAt := index + len(operation.Match)
		return markdown[:insertAt] + "\n\n" + content + markdown[insertAt:], nil
	case "insert_before":
		if strings.TrimSpace(operation.Heading) != "" {
			section, ok := findSection(markdown, operation.Heading)
			if !ok {
				return "", fmt.Errorf("heading not found")
			}
			return markdown[:section.start] + content + "\n\n" + markdown[section.start:], nil
		}
		if strings.TrimSpace(operation.Match) == "" {
			return "", fmt.Errorf("insert_before operation requires heading or match")
		}
		index := strings.Index(markdown, operation.Match)
		if index < 0 {
			return "", fmt.Errorf("insert_before match not found")
		}
		return markdown[:index] + content + "\n\n" + markdown[index:], nil
	default:
		return "", fmt.Errorf("unsupported patch operation: %s", operation.Type)
	}
}

type markdownSection struct {
	start        int
	contentStart int
	end          int
}

func findSection(markdown string, heading string) (markdownSection, bool) {
	heading = strings.TrimSpace(heading)
	matches := markdownHeadingPattern.FindAllStringSubmatchIndex(markdown, -1)
	for index, match := range matches {
		title := strings.TrimSpace(markdown[match[4]:match[5]])
		if title != heading {
			continue
		}
		level := match[3] - match[2]
		end := len(markdown)
		for _, next := range matches[index+1:] {
			nextLevel := next[3] - next[2]
			if nextLevel <= level {
				end = next[0]
				break
			}
		}
		contentStart := match[1]
		for contentStart < len(markdown) && markdown[contentStart] == '\n' {
			contentStart++
		}
		return markdownSection{start: match[0], contentStart: contentStart, end: end}, true
	}
	return markdownSection{}, false
}

func replaceSection(markdown string, heading string, content string) (string, error) {
	section, ok := findSection(markdown, heading)
	if !ok {
		return "", fmt.Errorf("heading not found")
	}
	return markdown[:section.contentStart] + strings.Trim(content, "\n") + "\n\n" + markdown[section.end:], nil
}

func insertInSection(markdown string, heading string, content string, atStart bool) (string, error) {
	section, ok := findSection(markdown, heading)
	if !ok {
		return "", fmt.Errorf("heading not found")
	}
	if atStart {
		return markdown[:section.contentStart] + content + markdown[section.contentStart:], nil
	}
	return markdown[:section.end] + content + markdown[section.end:], nil
}

func insertAfterSection(markdown string, heading string, content string) (string, error) {
	section, ok := findSection(markdown, heading)
	if !ok {
		return "", fmt.Errorf("heading not found")
	}
	return markdown[:section.end] + "\n\n" + strings.Trim(content, "\n") + markdown[section.end:], nil
}
