package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"g.co1d.in/Coldin04/Cyime/server/internal/ai"
	"g.co1d.in/Coldin04/Cyime/server/internal/apitoken"
	"g.co1d.in/Coldin04/Cyime/server/internal/workspace"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const (
	protocolVersion = "2025-06-18"
	serverName      = "cyime-workspace"
	serverVersion   = "0.1.0"

	jsonRPCParseError     = -32700
	jsonRPCInvalidRequest = -32600
	jsonRPCMethodNotFound = -32601
	jsonRPCInvalidParams  = -32602
	jsonRPCInternalError  = -32603
	jsonRPCForbidden      = -32003
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
}

type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      serverInfo     `json:"serverInfo"`
	Instructions    string         `json:"instructions"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolCallResult struct {
	Content           []contentItem `json:"content"`
	IsError           bool          `json:"isError,omitempty"`
	StructuredContent any           `json:"structuredContent,omitempty"`
}

type toolDefinition struct {
	Name           string
	Description    string
	RequiredScopes []string
	InputSchema    map[string]any
	Call           func(userID uuid.UUID, arguments json.RawMessage) (any, error)
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type listFilesArgs struct {
	ParentID      *string `json:"parentId"`
	ParentIDSnake *string `json:"parent_id"`
	Limit         int     `json:"limit"`
	Offset        int     `json:"offset"`
	SortBy        string  `json:"sortBy"`
	SortBySnake   string  `json:"sort_by"`
	Order         string  `json:"order"`
	Type          string  `json:"type"`
}

type createFolderArgs struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	ParentID    *string `json:"parentId"`
}

type createDocumentArgs struct {
	Title                  string  `json:"title"`
	Format                 string  `json:"format"`
	Content                string  `json:"content"`
	FolderID               *string `json:"folderId"`
	PreferredImageTargetID string  `json:"preferredImageTargetId"`
}

type documentIDArgs struct {
	ID         string `json:"id"`
	DocumentID string `json:"documentId"`
	Format     string `json:"format"`
}

type updateDocumentArgs struct {
	ID          string `json:"id"`
	DocumentID  string `json:"documentId"`
	Format      string `json:"format"`
	Content     string `json:"content"`
	BaseVersion *int64 `json:"baseVersion"`
}

type patchDocumentArgs struct {
	ID          string              `json:"id"`
	DocumentID  string              `json:"documentId"`
	Format      string              `json:"format"`
	BaseVersion *int64              `json:"baseVersion"`
	Operations  []ai.PatchOperation `json:"operations"`
}

type fileArgs struct {
	ID string `json:"id"`
}

type renameFileArgs struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
}

type moveFileArgs struct {
	ID                  string  `json:"id"`
	Type                string  `json:"type"`
	DestinationFolderID *string `json:"destinationFolderId"`
}

type copyFileArgs struct {
	ID                  string  `json:"id"`
	Type                string  `json:"type"`
	DestinationFolderID *string `json:"destinationFolderId"`
	Name                string  `json:"name"`
}

var tools = []toolDefinition{
	{
		Name:           "cyime_list_files",
		Description:    "List Cyime workspace folders and documents.",
		RequiredScopes: []string{apitoken.ScopeWorkspaceRead},
		InputSchema: objectSchema(map[string]any{
			"parentId": nullableUUIDSchema("Folder UUID, empty, or null for the workspace root."),
			"limit":    integerSchema("Maximum number of items."),
			"offset":   integerSchema("Pagination offset."),
			"sortBy":   enumSchema("Sort field.", []string{"name", "title", "created_at", "updated_at"}),
			"order":    enumSchema("Sort order.", []string{"asc", "desc"}),
			"type":     enumSchema("Filter.", []string{"all", "folder", "document", "folders", "documents"}),
		}, nil),
		Call: callListFiles,
	},
	{
		Name:           "cyime_create_folder",
		Description:    "Create a Cyime workspace folder.",
		RequiredScopes: []string{apitoken.ScopeWorkspaceWrite},
		InputSchema: objectSchema(map[string]any{
			"name":        stringSchema("Folder name."),
			"description": nullableStringSchema("Optional folder description."),
			"parentId":    nullableUUIDSchema("Parent folder UUID. Use null for the workspace root."),
		}, []string{"name"}),
		Call: callCreateFolder,
	},
	{
		Name:           "cyime_create_markdown_document",
		Description:    "Create a Markdown document in Cyime.",
		RequiredScopes: []string{apitoken.ScopeWorkspaceWrite, apitoken.ScopeDocumentWrite},
		InputSchema: objectSchema(map[string]any{
			"title":                  stringSchema("Document title."),
			"format":                 enumSchema("Content format. Use markdown.", []string{"markdown"}),
			"content":                stringSchema("Markdown content."),
			"folderId":               nullableUUIDSchema("Parent folder UUID. Use null for the workspace root."),
			"preferredImageTargetId": stringSchema("Optional image target ID."),
		}, []string{"title", "format", "content"}),
		Call: callCreateMarkdownDocument,
	},
	{
		Name:           "cyime_read_markdown_document",
		Description:    "Read a Cyime document as Markdown.",
		RequiredScopes: []string{apitoken.ScopeDocumentRead},
		InputSchema: objectSchema(map[string]any{
			"id":     uuidSchema("Document UUID."),
			"format": enumSchema("Content format. Use markdown.", []string{"markdown"}),
		}, []string{"id"}),
		Call: callReadMarkdownDocument,
	},
	{
		Name:           "cyime_update_markdown_document",
		Description:    "Replace a Cyime document with Markdown content.",
		RequiredScopes: []string{apitoken.ScopeDocumentWrite},
		InputSchema: objectSchema(map[string]any{
			"id":          uuidSchema("Document UUID."),
			"format":      enumSchema("Content format. Use markdown.", []string{"markdown"}),
			"content":     stringSchema("Full Markdown content."),
			"baseVersion": nullableIntegerSchema("Expected current document content version."),
		}, []string{"id", "format", "content"}),
		Call: callUpdateMarkdownDocument,
	},
	{
		Name:           "cyime_patch_markdown_document",
		Description:    "Apply incremental Markdown patch operations to a Cyime document.",
		RequiredScopes: []string{apitoken.ScopeDocumentRead, apitoken.ScopeDocumentWrite},
		InputSchema: objectSchema(map[string]any{
			"id":          uuidSchema("Document UUID."),
			"format":      enumSchema("Content format. Use markdown.", []string{"markdown"}),
			"baseVersion": nullableIntegerSchema("Expected current document content version."),
			"operations": objectArraySchema("Patch operations.", map[string]any{
				"type":    enumSchema("Patch operation.", []string{"append", "prepend", "replace", "insert_after", "insert_before"}),
				"target":  stringSchema("Optional target, for example section."),
				"heading": stringSchema("Optional Markdown heading."),
				"match":   stringSchema("Optional exact text match."),
				"content": stringSchema("Markdown fragment."),
			}, []string{"type", "content"}),
		}, []string{"id", "format", "operations"}),
		Call: callPatchMarkdownDocument,
	},
	{
		Name:           "cyime_rename_file",
		Description:    "Rename a Cyime folder or document.",
		RequiredScopes: []string{apitoken.ScopeWorkspaceWrite},
		InputSchema: objectSchema(map[string]any{
			"id":   uuidSchema("File or folder UUID."),
			"type": enumSchema("File type.", []string{"folder", "document"}),
			"name": stringSchema("New folder name or document title."),
		}, []string{"id", "type", "name"}),
		Call: callRenameFile,
	},
	{
		Name:           "cyime_move_file",
		Description:    "Move a Cyime folder or document.",
		RequiredScopes: []string{apitoken.ScopeFileMove},
		InputSchema: objectSchema(map[string]any{
			"id":                  uuidSchema("File or folder UUID."),
			"type":                enumSchema("File type.", []string{"folder", "document"}),
			"destinationFolderId": nullableUUIDSchema("Destination folder UUID. Use null for the workspace root."),
		}, []string{"id", "type"}),
		Call: callMoveFile,
	},
	{
		Name:           "cyime_copy_file",
		Description:    "Copy a Cyime folder or document.",
		RequiredScopes: []string{apitoken.ScopeFileCopy, apitoken.ScopeWorkspaceWrite},
		InputSchema: objectSchema(map[string]any{
			"id":                  uuidSchema("File or folder UUID."),
			"type":                enumSchema("File type.", []string{"folder", "document"}),
			"destinationFolderId": nullableUUIDSchema("Destination folder UUID. Use null for the workspace root."),
			"name":                stringSchema("Optional copy name/title. Omit or empty to auto-generate."),
		}, []string{"id", "type"}),
		Call: callCopyFile,
	},
}

func Handle(c *fiber.Ctx) error {
	var req rpcRequest
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errorResponse(nil, jsonRPCParseError, "Parse error", err.Error()))
	}

	if req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" {
		return c.JSON(errorResponse(req.ID, jsonRPCInvalidRequest, "Invalid Request", nil))
	}

	if len(req.ID) == 0 {
		return c.SendStatus(fiber.StatusAccepted)
	}

	switch req.Method {
	case "initialize":
		return handleInitialize(c, req)
	case "ping":
		return c.JSON(successResponse(req.ID, map[string]any{}))
	case "tools/list":
		return c.JSON(successResponse(req.ID, map[string]any{"tools": listTools()}))
	case "tools/call":
		return handleToolsCall(c, req)
	default:
		return c.JSON(errorResponse(req.ID, jsonRPCMethodNotFound, "Method not found", req.Method))
	}
}

func handleInitialize(c *fiber.Ctx, req rpcRequest) error {
	var params initializeParams
	if err := decodeParams(req.Params, &params); err != nil {
		return c.JSON(errorResponse(req.ID, jsonRPCInvalidParams, "Invalid params", err.Error()))
	}

	version := protocolVersion
	if strings.TrimSpace(params.ProtocolVersion) != "" {
		version = params.ProtocolVersion
	}

	return c.JSON(successResponse(req.ID, initializeResult{
		ProtocolVersion: version,
		Capabilities: map[string]any{
			"tools": map[string]any{
				"listChanged": false,
			},
		},
		ServerInfo:   serverInfo{Name: serverName, Version: serverVersion},
		Instructions: "Use Cyime tools when the user wants to read, create, organize, or update Cyime workspace documents and folders. Keep document content in Markdown.",
	}))
}

func handleToolsCall(c *fiber.Ctx, req rpcRequest) error {
	var params toolCallParams
	if err := decodeParams(req.Params, &params); err != nil {
		return c.JSON(errorResponse(req.ID, jsonRPCInvalidParams, "Invalid params", err.Error()))
	}
	params.Name = strings.TrimSpace(params.Name)
	if params.Name == "" {
		return c.JSON(errorResponse(req.ID, jsonRPCInvalidParams, "Invalid params", "tool name is required"))
	}

	tool, ok := findTool(params.Name)
	if !ok {
		return c.JSON(errorResponse(req.ID, jsonRPCInvalidParams, "Unknown tool", params.Name))
	}

	userID, scopes, err := userAndScopes(c)
	if err != nil {
		return c.JSON(errorResponse(req.ID, jsonRPCInternalError, "Invalid authenticated context", err.Error()))
	}
	if !apitoken.HasScopes(scopes, tool.RequiredScopes...) {
		return c.JSON(errorResponse(req.ID, jsonRPCForbidden, "Insufficient API token scope", map[string]any{
			"requiredScopes": tool.RequiredScopes,
		}))
	}

	result, err := tool.Call(userID, params.Arguments)
	if err != nil {
		return c.JSON(successResponse(req.ID, toolErrorResult(err)))
	}
	return c.JSON(successResponse(req.ID, toolSuccessResult(result)))
}

func callListFiles(userID uuid.UUID, raw json.RawMessage) (any, error) {
	var args listFilesArgs
	if err := decodeParams(raw, &args); err != nil {
		return nil, err
	}
	parentID, err := parseOptionalUUID(firstString(args.ParentID, args.ParentIDSnake), "parentId")
	if err != nil {
		return nil, err
	}
	limit := args.Limit
	if limit == 0 {
		limit = 50
	}
	sortBy := strings.TrimSpace(args.SortBy)
	if sortBy == "" {
		sortBy = strings.TrimSpace(args.SortBySnake)
	}
	return workspace.GetFiles(userID, parentID, limit, args.Offset, sortBy, args.Order, normalizeListType(args.Type))
}

func callCreateFolder(userID uuid.UUID, raw json.RawMessage) (any, error) {
	var args createFolderArgs
	if err := decodeParams(raw, &args); err != nil {
		return nil, err
	}
	parentID, err := parseOptionalUUID(args.ParentID, "parentId")
	if err != nil {
		return nil, err
	}
	folder, err := workspace.CreateFolder(userID, args.Name, args.Description, parentID)
	if err != nil {
		return nil, err
	}
	return workspace.CreateFolderResponse{
		ID:          folder.ID,
		Type:        "folder",
		Name:        folder.Name,
		Description: folder.Description,
		ParentID:    folder.ParentID,
		CreatedAt:   folder.CreatedAt,
		UpdatedAt:   folder.UpdatedAt,
		Creator: workspace.CreatorInfo{
			ID: userID,
		},
	}, nil
}

func callCreateMarkdownDocument(userID uuid.UUID, raw json.RawMessage) (any, error) {
	var args createDocumentArgs
	if err := decodeParams(raw, &args); err != nil {
		return nil, err
	}
	if err := validateMarkdownFormat(args.Format); err != nil {
		return nil, err
	}
	folderID, err := parseOptionalUUID(args.FolderID, "folderId")
	if err != nil {
		return nil, err
	}
	return ai.CreateMarkdownDocument(userID, ai.CreateMarkdownDocumentInput{
		Title:                  args.Title,
		Content:                args.Content,
		FolderID:               folderID,
		PreferredImageTargetID: args.PreferredImageTargetID,
	})
}

func callReadMarkdownDocument(userID uuid.UUID, raw json.RawMessage) (any, error) {
	var args documentIDArgs
	if err := decodeParams(raw, &args); err != nil {
		return nil, err
	}
	if err := validateMarkdownFormat(args.Format); err != nil {
		return nil, err
	}
	documentID, err := parseRequiredUUID(firstNonEmpty(args.ID, args.DocumentID), "id")
	if err != nil {
		return nil, err
	}
	return ai.GetMarkdownContent(userID, documentID)
}

func callUpdateMarkdownDocument(userID uuid.UUID, raw json.RawMessage) (any, error) {
	var args updateDocumentArgs
	if err := decodeParams(raw, &args); err != nil {
		return nil, err
	}
	if err := validateMarkdownFormat(args.Format); err != nil {
		return nil, err
	}
	documentID, err := parseRequiredUUID(firstNonEmpty(args.ID, args.DocumentID), "id")
	if err != nil {
		return nil, err
	}
	return ai.UpdateMarkdownContent(userID, documentID, args.Content, args.BaseVersion)
}

func callPatchMarkdownDocument(userID uuid.UUID, raw json.RawMessage) (any, error) {
	var args patchDocumentArgs
	if err := decodeParams(raw, &args); err != nil {
		return nil, err
	}
	if err := validateMarkdownFormat(args.Format); err != nil {
		return nil, err
	}
	if len(args.Operations) == 0 {
		return nil, errors.New("at least one patch operation is required")
	}
	documentID, err := parseRequiredUUID(firstNonEmpty(args.ID, args.DocumentID), "id")
	if err != nil {
		return nil, err
	}
	return ai.PatchMarkdownContent(userID, documentID, args.Operations, args.BaseVersion)
}

func callRenameFile(userID uuid.UUID, raw json.RawMessage) (any, error) {
	var args renameFileArgs
	if err := decodeParams(raw, &args); err != nil {
		return nil, err
	}
	fileID, fileType, err := parseFileArgs(args.ID, args.Type)
	if err != nil {
		return nil, err
	}
	switch fileType {
	case "document":
		err = workspace.UpdateDocumentTitle(userID, fileID, args.Name)
	case "folder":
		err = workspace.UpdateFolderName(userID, fileID, args.Name)
	}
	if err != nil {
		return nil, err
	}
	return fileOperationResult(userID, fileID, fileType)
}

func callMoveFile(userID uuid.UUID, raw json.RawMessage) (any, error) {
	var args moveFileArgs
	if err := decodeParams(raw, &args); err != nil {
		return nil, err
	}
	fileID, fileType, err := parseFileArgs(args.ID, args.Type)
	if err != nil {
		return nil, err
	}
	destinationFolderID, err := parseOptionalUUID(args.DestinationFolderID, "destinationFolderId")
	if err != nil {
		return nil, err
	}
	switch fileType {
	case "document":
		_, err = workspace.MoveDocument(userID, fileID, destinationFolderID)
	case "folder":
		_, err = workspace.MoveFolder(userID, fileID, destinationFolderID)
	}
	if err != nil {
		return nil, err
	}
	return fileOperationResult(userID, fileID, fileType)
}

func callCopyFile(userID uuid.UUID, raw json.RawMessage) (any, error) {
	var args copyFileArgs
	if err := decodeParams(raw, &args); err != nil {
		return nil, err
	}
	fileID, fileType, err := parseFileArgs(args.ID, args.Type)
	if err != nil {
		return nil, err
	}
	destinationFolderID, err := parseOptionalUUID(args.DestinationFolderID, "destinationFolderId")
	if err != nil {
		return nil, err
	}
	item, err := workspace.CopyFile(userID, fileID, fileType, destinationFolderID, args.Name)
	if err != nil {
		return nil, err
	}
	return map[string]any{"success": true, "item": item}, nil
}

func fileOperationResult(userID uuid.UUID, fileID uuid.UUID, fileType string) (any, error) {
	item, err := workspace.GetFile(userID, fileID, fileType)
	if err != nil {
		return nil, err
	}
	return map[string]any{"success": true, "item": item}, nil
}

func listTools() []mcpTool {
	items := make([]mcpTool, 0, len(tools))
	for _, tool := range tools {
		items = append(items, mcpTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	return items
}

func findTool(name string) (toolDefinition, bool) {
	for _, tool := range tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return toolDefinition{}, false
}

func userAndScopes(c *fiber.Ctx) (uuid.UUID, []string, error) {
	userIDStr, ok := c.Locals("userId").(string)
	if !ok {
		return uuid.Nil, nil, errors.New("missing user id")
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return uuid.Nil, nil, err
	}
	scopes, ok := c.Locals(apitoken.LocalsScopes).([]string)
	if !ok {
		return uuid.Nil, nil, errors.New("missing token scopes")
	}
	return userID, scopes, nil
}

func decodeParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	return json.Unmarshal(raw, target)
}

func successResponse(id json.RawMessage, result any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: normalizedID(id), Result: result}
}

func errorResponse(id json.RawMessage, code int, message string, data any) rpcResponse {
	return rpcResponse{
		JSONRPC: "2.0",
		ID:      normalizedID(id),
		Error:   &rpcError{Code: code, Message: message, Data: data},
	}
}

func normalizedID(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return json.RawMessage("null")
	}
	return id
}

func toolSuccessResult(data any) toolCallResult {
	text := "{}"
	if data != nil {
		if payload, err := json.MarshalIndent(data, "", "  "); err == nil {
			text = string(payload)
		}
	}
	return toolCallResult{
		Content:           []contentItem{{Type: "text", Text: text}},
		StructuredContent: data,
	}
}

func toolErrorResult(err error) toolCallResult {
	return toolCallResult{
		Content: []contentItem{{
			Type: "text",
			Text: err.Error(),
		}},
		IsError: true,
	}
}

func firstString(values ...*string) *string {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseFileArgs(rawID string, rawType string) (uuid.UUID, string, error) {
	fileID, err := parseRequiredUUID(rawID, "id")
	if err != nil {
		return uuid.Nil, "", err
	}
	fileType, err := normalizeFileType(rawType)
	if err != nil {
		return uuid.Nil, "", err
	}
	return fileID, fileType, nil
}

func parseRequiredUUID(raw string, label string) (uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, fmt.Errorf("%s is required", label)
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s must be a valid UUID", label)
	}
	return parsed, nil
}

func parseOptionalUUID(raw *string, label string) (*uuid.UUID, error) {
	if raw == nil {
		return nil, nil
	}
	value := strings.TrimSpace(*raw)
	if value == "" || value == "null" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("%s must be a valid UUID or null", label)
	}
	return &parsed, nil
}

func normalizeFileType(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "document":
		return "document", nil
	case "folder":
		return "folder", nil
	default:
		return "", errors.New("type must be document or folder")
	}
}

func normalizeListType(raw string) string {
	switch strings.TrimSpace(raw) {
	case "folder":
		return "folders"
	case "document":
		return "documents"
	default:
		return raw
	}
}

func validateMarkdownFormat(format string) error {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" || format == "markdown" {
		return nil
	}
	return errors.New("format must be markdown")
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func objectArraySchema(description string, properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       objectSchema(properties, required),
	}
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func nullableStringSchema(description string) map[string]any {
	return map[string]any{"type": []string{"string", "null"}, "description": description}
}

func uuidSchema(description string) map[string]any {
	return map[string]any{"type": "string", "format": "uuid", "description": description}
}

func nullableUUIDSchema(description string) map[string]any {
	return map[string]any{"type": []string{"string", "null"}, "format": "uuid", "description": description}
}

func integerSchema(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func nullableIntegerSchema(description string) map[string]any {
	return map[string]any{"type": []string{"integer", "null"}, "description": description}
}

func enumSchema(description string, values []string) map[string]any {
	return map[string]any{"type": "string", "description": description, "enum": values}
}
