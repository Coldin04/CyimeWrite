package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/config"
)

const (
	markdownConverterDefaultTimeout = 5 * time.Second
	markdownConverterMaxResponse    = 4 * 1024 * 1024
)

var (
	ErrMarkdownConverterUnavailable = errors.New("markdown converter unavailable")
	ErrMarkdownConversionFailed     = errors.New("markdown conversion failed")
)

type MarkdownConverter interface {
	MarkdownToContentJSON(ctx context.Context, markdown string) ([]byte, error)
	ContentJSONToMarkdown(ctx context.Context, raw []byte) (string, error)
}

type MarkdownConversionError struct {
	Kind      error
	Operation string
	Detail    string
	Cause     error
}

func (e *MarkdownConversionError) Error() string {
	return e.Detail
}

func (e *MarkdownConversionError) Unwrap() error {
	return e.Cause
}

func (e *MarkdownConversionError) Is(target error) bool {
	return target == e.Kind
}

type legacyMarkdownConverter struct{}

func (legacyMarkdownConverter) MarkdownToContentJSON(_ context.Context, markdown string) ([]byte, error) {
	return legacyMarkdownToContentJSON(markdown)
}

func (legacyMarkdownConverter) ContentJSONToMarkdown(_ context.Context, raw []byte) (string, error) {
	return legacyContentJSONToMarkdown(raw)
}

type remoteMarkdownConverter struct {
	endpoint string
	token    string
	timeout  time.Duration
	fallback bool
	legacy   legacyMarkdownConverter
	client   *http.Client
}

type markdownConvertRequest struct {
	Direction   string          `json:"direction"`
	Markdown    string          `json:"markdown,omitempty"`
	ContentJSON json.RawMessage `json:"contentJson,omitempty"`
}

type markdownConvertResponse struct {
	ContentJSON json.RawMessage `json:"contentJson,omitempty"`
	Markdown    string          `json:"markdown,omitempty"`
	Code        string          `json:"code,omitempty"`
	Message     string          `json:"message,omitempty"`
}

func markdownToContentJSON(markdown string) ([]byte, error) {
	return configuredMarkdownConverter().MarkdownToContentJSON(context.Background(), markdown)
}

func contentJSONToMarkdown(raw []byte) (string, error) {
	return configuredMarkdownConverter().ContentJSONToMarkdown(context.Background(), raw)
}

func configuredMarkdownConverter() MarkdownConverter {
	endpoint := strings.TrimSpace(os.Getenv("MARKDOWN_CONVERTER_URL"))
	if endpoint == "" {
		return legacyMarkdownConverter{}
	}
	endpoint = normalizeMarkdownConverterURL(endpoint)

	timeout := markdownConverterDefaultTimeout
	if raw := strings.TrimSpace(os.Getenv("MARKDOWN_CONVERTER_TIMEOUT")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			timeout = parsed
		}
	}

	return remoteMarkdownConverter{
		endpoint: endpoint,
		token:    strings.TrimSpace(os.Getenv("MARKDOWN_CONVERTER_TOKEN")),
		timeout:  timeout,
		fallback: config.IsTrue(os.Getenv("MARKDOWN_CONVERTER_FALLBACK")),
		legacy:   legacyMarkdownConverter{},
		client:   &http.Client{Timeout: timeout},
	}
}

func normalizeMarkdownConverterURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	if strings.TrimSpace(parsed.Path) == "" || parsed.Path == "/" {
		parsed.Path = "/markdown/convert"
	}
	return parsed.String()
}

func (c remoteMarkdownConverter) MarkdownToContentJSON(ctx context.Context, markdown string) ([]byte, error) {
	var result markdownConvertResponse
	err := c.post(ctx, markdownConvertRequest{
		Direction: "markdown-to-json",
		Markdown:  markdown,
	}, &result)
	if err != nil {
		if c.fallback {
			return c.legacy.MarkdownToContentJSON(ctx, markdown)
		}
		return nil, err
	}
	if len(result.ContentJSON) == 0 || !json.Valid(result.ContentJSON) {
		err := markdownConversionFailed("markdown-to-json", errors.New("converter returned invalid contentJson"))
		if c.fallback {
			return c.legacy.MarkdownToContentJSON(ctx, markdown)
		}
		return nil, err
	}
	return result.ContentJSON, nil
}

func (c remoteMarkdownConverter) ContentJSONToMarkdown(ctx context.Context, raw []byte) (string, error) {
	if len(raw) == 0 || !json.Valid(raw) {
		return "", markdownConversionFailed("json-to-markdown", errors.New("invalid content json"))
	}

	var result markdownConvertResponse
	err := c.post(ctx, markdownConvertRequest{
		Direction:   "json-to-markdown",
		ContentJSON: json.RawMessage(raw),
	}, &result)
	if err != nil {
		if c.fallback {
			return c.legacy.ContentJSONToMarkdown(ctx, raw)
		}
		return "", err
	}
	return result.Markdown, nil
}

func (c remoteMarkdownConverter) post(ctx context.Context, payload markdownConvertRequest, result *markdownConvertResponse) error {
	if c.token == "" {
		return markdownConverterUnavailable(payload.Direction, errors.New("MARKDOWN_CONVERTER_TOKEN is not configured"))
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return markdownConversionFailed(payload.Direction, err)
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return markdownConverterUnavailable(payload.Direction, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return markdownConverterUnavailable(payload.Direction, err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, markdownConverterMaxResponse))
	if err != nil {
		return markdownConverterUnavailable(payload.Direction, err)
	}

	if len(responseBody) > 0 {
		_ = json.Unmarshal(responseBody, result)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	message := strings.TrimSpace(result.Message)
	if message == "" {
		message = fmt.Sprintf("markdown converter returned HTTP %d", resp.StatusCode)
	}

	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusRequestEntityTooLarge || resp.StatusCode == http.StatusUnprocessableEntity {
		return markdownConversionFailed(payload.Direction, errors.New(message))
	}
	return markdownConverterUnavailable(payload.Direction, errors.New(message))
}

func markdownConverterUnavailable(operation string, cause error) error {
	return &MarkdownConversionError{
		Kind:      ErrMarkdownConverterUnavailable,
		Operation: operation,
		Detail:    markdownConverterUserMessage(operation, true),
		Cause:     cause,
	}
}

func markdownConversionFailed(operation string, cause error) error {
	return &MarkdownConversionError{
		Kind:      ErrMarkdownConversionFailed,
		Operation: operation,
		Detail:    markdownConverterUserMessage(operation, false),
		Cause:     cause,
	}
}

func markdownConverterUserMessage(operation string, unavailable bool) string {
	if operation == "json-to-markdown" {
		if unavailable {
			return "Markdown export service is unavailable. Please try again later or ask the Cyime administrator to check the converter configuration."
		}
		return "Markdown export failed. Please try again later or ask the Cyime administrator to check the stored document content."
	}
	if unavailable {
		return "Markdown conversion service is unavailable. The document was not changed. Please try again later or ask the Cyime administrator to check the converter configuration."
	}
	return "Markdown conversion failed. The document was not changed. Please simplify unsupported Markdown syntax and retry."
}
