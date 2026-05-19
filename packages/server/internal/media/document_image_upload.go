package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/acl"
	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/imagebeds"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"g.co1d.in/Coldin04/Cyime/server/internal/securevalue"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	documentImageModeManagedAsset = "managed_asset"
	documentImageModeExternalURL  = "external_url"

	documentImageTargetManagedR2       = "managed-r2"
	defaultDocumentImageMaxBytes int64 = 5 * 1024 * 1024
)

type DocumentImageErrorCode string

const (
	DocumentImageErrUnsupportedTarget  DocumentImageErrorCode = "unsupported_target"
	DocumentImageErrProviderNotFound   DocumentImageErrorCode = "provider_not_found"
	DocumentImageErrProviderConfig     DocumentImageErrorCode = "provider_config_invalid"
	DocumentImageErrProviderNotReady   DocumentImageErrorCode = "provider_not_configured"
	DocumentImageErrProviderUploadFail DocumentImageErrorCode = "provider_upload_failed"
	DocumentImageErrFileTooLarge       DocumentImageErrorCode = "file_too_large"
)

type DocumentImageError struct {
	Code    DocumentImageErrorCode
	Message string
}

func (e *DocumentImageError) Error() string {
	return e.Message
}

func newDocumentImageError(code DocumentImageErrorCode, message string) error {
	return &DocumentImageError{
		Code:    code,
		Message: message,
	}
}

// imageBedHTTPClient forwards user-configured image bed requests. Because
// the target URL comes from tenant config (and may be edited by any user
// with image bed access) the client MUST be hardened against SSRF:
//
//  1. DialContext uses safeDialContext, which resolves the hostname and
//     refuses to connect to loopback / private / link-local / cloud
//     metadata IPs. Dialing is then performed by literal IP to prevent
//     DNS-rebinding bypass.
//  2. CheckRedirect caps the redirect chain and re-applies the scheme
//     allowlist so a public entrypoint cannot bounce us into an internal
//     URL. The DialContext guard is enough on its own, but capping keeps
//     request budgets sane and makes error messages more actionable.
var imageBedHTTPClient = &http.Client{
	Timeout: 90 * time.Second,
	Transport: &http.Transport{
		// Indirect through the package-level imageBedDialContext variable so
		// tests can install a loopback-friendly dialer. Production still
		// runs safeDialContext — see ssrf_guard.go.
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return imageBedDialContext(ctx, network, addr)
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("too many redirects (%d)", len(via))
		}
		scheme := strings.ToLower(req.URL.Scheme)
		if scheme != "http" && scheme != "https" {
			return fmt.Errorf("redirect to unsupported scheme %q blocked", scheme)
		}
		return nil
	},
}

type UploadDocumentImageRequest struct {
	DocumentID uuid.UUID
	UserID     uuid.UUID
	FileHeader *multipart.FileHeader
	TargetID   string
}

type UploadDocumentImageResult struct {
	TargetID string     `json:"targetId"`
	Mode     string     `json:"mode"`
	URL      string     `json:"url"`
	AssetID  *uuid.UUID `json:"assetId,omitempty"`
}

type documentImageUploader interface {
	Upload(ctx context.Context, req UploadDocumentImageRequest) (*UploadDocumentImageResult, error)
}

type managedDocumentImageUploader struct{}

func (u *managedDocumentImageUploader) Upload(ctx context.Context, req UploadDocumentImageRequest) (*UploadDocumentImageResult, error) {
	result, err := UploadDocumentAsset(ctx, UploadAssetRequest{
		DocumentID: req.DocumentID,
		UserID:     req.UserID,
		FileHeader: req.FileHeader,
		Visibility: "private",
	})
	if err != nil {
		return nil, err
	}

	return &UploadDocumentImageResult{
		TargetID: documentImageTargetManagedR2,
		Mode:     documentImageModeManagedAsset,
		URL:      "",
		AssetID:  &result.Asset.ID,
	}, nil
}

type genericImageBedUploader struct {
	targetID string
	provider imagebeds.Provider
	config   *models.UserImageBedConfig
}

func (u *genericImageBedUploader) Upload(ctx context.Context, req UploadDocumentImageRequest) (*UploadDocumentImageResult, error) {
	if req.FileHeader == nil {
		return nil, ErrFileRequired
	}

	variables, err := buildProviderVariables(u.provider, u.config)
	if err != nil {
		return nil, err
	}
	if err := validateProviderVariables(u.provider, variables); err != nil {
		return nil, err
	}

	requestURL, err := buildRequestURL(u.provider.Upload.URLTemplate, variables, u.provider.Upload.Query)
	if err != nil {
		return nil, err
	}

	body, contentType, err := buildMultipartPayload(req.FileHeader, u.provider, variables)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, strings.ToUpper(u.provider.Upload.Method), requestURL, body)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", contentType)
	if err := applyRequestHeaders(httpReq, u.provider.Upload.Headers, variables); err != nil {
		return nil, err
	}

	resp, err := imageBedHTTPClient.Do(httpReq)
	if err != nil {
		return nil, newDocumentImageError(DocumentImageErrProviderUploadFail, err.Error())
	}
	defer resp.Body.Close()

	payloadBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, newDocumentImageError(DocumentImageErrProviderUploadFail, err.Error())
	}

	var payload any
	if len(payloadBytes) > 0 {
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			return nil, newDocumentImageError(DocumentImageErrProviderUploadFail, "Invalid provider response")
		}
	}

	success, successErr := evaluateUploadSuccess(u.provider, resp.StatusCode, payload)
	if successErr != nil {
		return nil, successErr
	}
	uploadedURL := normalizeProviderResultURL(extractResultURL(u.provider.Upload.ResultURLPaths, payload), requestURL)
	if !success && uploadedURL != "" {
		success = true
	}
	if !success {
		return nil, newDocumentImageError(DocumentImageErrProviderUploadFail, extractProviderErrorMessage(u.provider, payload))
	}

	if uploadedURL == "" {
		return nil, newDocumentImageError(DocumentImageErrProviderUploadFail, "Provider upload did not return a usable URL")
	}

	return &UploadDocumentImageResult{
		TargetID: u.targetID,
		Mode:     documentImageModeExternalURL,
		URL:      uploadedURL,
	}, nil
}

func buildProviderVariables(provider imagebeds.Provider, config *models.UserImageBedConfig) (map[string]string, error) {
	apiToken := strings.TrimSpace(stringPtrValue(config.APIToken))
	if apiToken != "" {
		decryptedToken, err := securevalue.DecryptString(apiToken)
		if err != nil {
			return nil, newDocumentImageError(DocumentImageErrProviderConfig, "failed to decrypt provider token")
		}
		apiToken = decryptedToken
	}

	values := map[string]string{
		"providerType": provider.ProviderType,
		"baseUrl":      strings.TrimSpace(stringPtrValue(config.BaseURL)),
		"apiToken":     apiToken,
	}

	if provider.Runtime.BaseURLEnv != "" {
		if values["baseUrl"] == "" {
			values["baseUrl"] = strings.TrimSpace(os.Getenv(provider.Runtime.BaseURLEnv))
		}
	}
	if values["baseUrl"] == "" {
		values["baseUrl"] = strings.TrimSpace(provider.Runtime.DefaultBaseURL)
	}

	if provider.Runtime.APITokenEnv != "" {
		if values["apiToken"] == "" {
			values["apiToken"] = strings.TrimSpace(os.Getenv(provider.Runtime.APITokenEnv))
		}
	}

	for key, value := range parseProviderConfigJSON(config.ConfigJSON) {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		values[key] = strings.TrimSpace(value)
	}
	values["baseUrl"] = normalizeBaseURL(values["baseUrl"], provider.Upload.URLTemplate)

	return values, nil
}

func validateProviderVariables(provider imagebeds.Provider, variables map[string]string) error {
	for _, field := range provider.Fields {
		if !field.Required {
			continue
		}
		value := strings.TrimSpace(variables[field.Key])
		if value == "" {
			return newDocumentImageError(DocumentImageErrProviderNotReady, fmt.Sprintf("%s %s is required", provider.DisplayName, field.Key))
		}

		if field.Type == "url" {
			parsed, err := url.Parse(value)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return newDocumentImageError(DocumentImageErrProviderConfig, fmt.Sprintf("Invalid URL for %s", field.Key))
			}
		}
		if field.Type == "number" {
			num, err := strconv.Atoi(value)
			if err != nil || num <= 0 {
				return newDocumentImageError(DocumentImageErrProviderConfig, fmt.Sprintf("Invalid number for %s", field.Key))
			}
		}
	}
	return nil
}

func buildRequestURL(urlTemplate string, variables map[string]string, queryParams []imagebeds.QueryParam) (string, error) {
	renderedURL := renderTemplate(urlTemplate, variables)
	parsedURL, err := url.Parse(renderedURL)
	if err != nil {
		return "", newDocumentImageError(DocumentImageErrProviderConfig, "Invalid upload url")
	}

	query := parsedURL.Query()
	for _, param := range queryParams {
		renderedValue := strings.TrimSpace(renderTemplate(param.ValueTemplate, variables))
		if renderedValue == "" {
			if param.Required {
				return "", newDocumentImageError(DocumentImageErrProviderNotReady, fmt.Sprintf("Missing query parameter: %s", param.Key))
			}
			continue
		}
		query.Set(param.Key, renderedValue)
	}
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

func buildMultipartPayload(fileHeader *multipart.FileHeader, provider imagebeds.Provider, variables map[string]string) (*bytes.Buffer, string, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	filePart, err := writer.CreateFormFile(provider.Upload.FileField, fileHeader.Filename)
	if err != nil {
		return nil, "", err
	}
	if _, err := io.Copy(filePart, file); err != nil {
		return nil, "", err
	}

	for _, field := range provider.Upload.FormFields {
		renderedValue := strings.TrimSpace(renderTemplate(field.ValueTemplate, variables))
		if renderedValue == "" {
			if field.Required {
				return nil, "", newDocumentImageError(DocumentImageErrProviderNotReady, fmt.Sprintf("Missing form field: %s", field.Key))
			}
			if field.OmitIfEmpty {
				continue
			}
		}
		if err := writer.WriteField(field.Key, renderedValue); err != nil {
			return nil, "", err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body, writer.FormDataContentType(), nil
}

func applyRequestHeaders(req *http.Request, headers []imagebeds.RequestHeader, variables map[string]string) error {
	for _, header := range headers {
		value := strings.TrimSpace(renderTemplate(header.ValueTemplate, variables))
		if value == "" {
			return newDocumentImageError(DocumentImageErrProviderNotReady, fmt.Sprintf("Missing header value for %s", header.Key))
		}
		req.Header.Set(header.Key, value)
	}
	return nil
}

func evaluateUploadSuccess(provider imagebeds.Provider, statusCode int, payload any) (bool, error) {
	if statusCode >= 400 {
		return false, nil
	}

	successPath := strings.TrimSpace(provider.Upload.SuccessJSONPath)
	if successPath == "" {
		return true, nil
	}

	raw := lookupJSONPath(payload, successPath)
	if raw == nil {
		return false, nil
	}
	expected := strings.ToLower(strings.TrimSpace(provider.Upload.SuccessEquals))
	actual := strings.ToLower(strings.TrimSpace(stringifyAny(raw)))
	return actual == expected, nil
}

func extractProviderErrorMessage(provider imagebeds.Provider, payload any) string {
	for _, path := range provider.Upload.ErrorMessagePaths {
		value := strings.TrimSpace(stringifyAny(lookupJSONPath(payload, path)))
		if value != "" {
			return value
		}
	}
	return "Provider upload failed"
}

func extractResultURL(paths []string, payload any) string {
	for _, path := range paths {
		value := strings.TrimSpace(stringifyAny(lookupJSONPath(payload, path)))
		if value != "" {
			return value
		}
	}
	return ""
}

func normalizeProviderResultURL(rawURL string, requestURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "//") {
		return "https:" + trimmed
	}

	base, err := url.Parse(requestURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return trimmed
	}

	if strings.HasPrefix(trimmed, "/") {
		return base.Scheme + "://" + base.Host + trimmed
	}
	return trimmed
}

func lookupJSONPath(payload any, path string) any {
	if strings.TrimSpace(path) == "" {
		return nil
	}

	current := payload
	for _, segment := range strings.Split(path, ".") {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		next, exists := obj[segment]
		if !exists {
			return nil
		}
		current = next
	}
	return current
}

func stringifyAny(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func renderTemplate(template string, values map[string]string) string {
	rendered := template
	for key, value := range values {
		rendered = strings.ReplaceAll(rendered, "{{"+key+"}}", value)
	}
	return rendered
}

func normalizeBaseURL(baseURL string, urlTemplate string) string {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.Contains(urlTemplate, "{{baseUrl}}/api/v2") && strings.HasSuffix(normalized, "/api/v2") {
		normalized = strings.TrimSuffix(normalized, "/api/v2")
	}
	if strings.Contains(urlTemplate, "{{baseUrl}}/api/v1") && strings.HasSuffix(normalized, "/api/v1") {
		normalized = strings.TrimSuffix(normalized, "/api/v1")
	}
	if strings.Contains(urlTemplate, "{{baseUrl}}/upload") && strings.HasSuffix(normalized, "/upload") {
		normalized = strings.TrimSuffix(normalized, "/upload")
	}
	return normalized
}

func parseProviderConfigJSON(raw *string) map[string]string {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return map[string]string{}
	}

	var payload struct {
		Fields map[string]any `json:"fields"`
	}
	if err := json.Unmarshal([]byte(*raw), &payload); err == nil && len(payload.Fields) > 0 {
		values := map[string]string{}
		for key, value := range payload.Fields {
			stringified := strings.TrimSpace(stringifyAny(value))
			if stringified == "" {
				continue
			}
			stringified = strings.TrimSuffix(stringified, ".0")
			values[key] = stringified
		}
		return values
	}
	// Legacy flat config payload is intentionally unsupported.
	return map[string]string{}
}

func getDocumentImageUploadTargetID(userID, documentID uuid.UUID) (string, error) {
	document, err := acl.CanEditDocument(database.DB, userID, documentID)
	if err != nil {
		return "", ErrDocumentNotAccessible
	}

	targetID := strings.TrimSpace(document.PreferredImageTargetID)

	var preference models.DocumentImageTargetPreference
	if err := database.DB.
		Where("document_id = ? AND user_id = ? AND deleted_at IS NULL", documentID, userID).
		First(&preference).Error; err == nil {
		targetID = strings.TrimSpace(preference.TargetID)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}

	switch targetID {
	case "", documentImageTargetManagedR2:
		return documentImageTargetManagedR2, nil
	default:
		configID, err := uuid.Parse(targetID)
		if err != nil {
			return documentImageTargetManagedR2, nil
		}
		if _, err := getUserImageBedConfig(userID, configID); err != nil {
			return documentImageTargetManagedR2, nil
		}
		return targetID, nil
	}
}

func normalizeDocumentImageTargetID(value string) string {
	trimmed := strings.TrimSpace(value)
	switch trimmed {
	case "", documentImageTargetManagedR2:
		return documentImageTargetManagedR2
	default:
		if _, err := uuid.Parse(trimmed); err == nil {
			return trimmed
		}
		return ""
	}
}

func newDocumentImageUploader(userID uuid.UUID, targetID string) (documentImageUploader, error) {
	switch targetID {
	case documentImageTargetManagedR2:
		return &managedDocumentImageUploader{}, nil
	default:
		configID, err := uuid.Parse(targetID)
		if err != nil {
			return nil, newDocumentImageError(DocumentImageErrUnsupportedTarget, "document image target is not supported")
		}

		config, err := getUserImageBedConfig(userID, configID)
		if err != nil {
			return nil, err
		}

		provider, ok := imagebeds.GetProviderByType(config.ProviderType)
		if !ok {
			return nil, newDocumentImageError(DocumentImageErrProviderNotFound, "provider is not registered")
		}

		return &genericImageBedUploader{
			targetID: config.ID.String(),
			provider: provider,
			config:   config,
		}, nil
	}
}

func getUserImageBedConfig(userID, configID uuid.UUID) (*models.UserImageBedConfig, error) {
	var config models.UserImageBedConfig
	if err := database.DB.
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", configID, userID).
		First(&config).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, newDocumentImageError(DocumentImageErrUnsupportedTarget, "image bed config not found")
		}
		return nil, err
	}
	if !config.IsEnabled {
		return nil, newDocumentImageError(DocumentImageErrProviderNotReady, "image bed config is disabled")
	}
	return &config, nil
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func UploadDocumentImage(ctx context.Context, req UploadDocumentImageRequest) (*UploadDocumentImageResult, error) {
	if req.FileHeader == nil {
		return nil, ErrFileRequired
	}
	maxBytes := documentImageMaxBytes()
	if req.FileHeader.Size > 0 && req.FileHeader.Size > maxBytes {
		return nil, newDocumentImageError(
			DocumentImageErrFileTooLarge,
			fmt.Sprintf("document image file too large: max %d bytes", maxBytes),
		)
	}

	targetID := normalizeDocumentImageTargetID(req.TargetID)
	if targetID == "" {
		return nil, newDocumentImageError(DocumentImageErrUnsupportedTarget, "document image target is not supported")
	}
	if strings.TrimSpace(req.TargetID) == "" {
		var err error
		targetID, err = getDocumentImageUploadTargetID(req.UserID, req.DocumentID)
		if err != nil {
			return nil, err
		}
	}

	uploader, err := newDocumentImageUploader(req.UserID, targetID)
	if err != nil {
		return nil, err
	}

	return uploader.Upload(ctx, req)
}

func documentImageMaxBytes() int64 {
	raw := strings.TrimSpace(os.Getenv("MEDIA_DOCUMENT_IMAGE_MAX_BYTES"))
	if raw == "" {
		return defaultDocumentImageMaxBytes
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed <= 0 {
		return defaultDocumentImageMaxBytes
	}
	return parsed
}
