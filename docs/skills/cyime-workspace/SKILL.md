---
name: cyime-workspace
author: Cyime
description: Use this skill when the user wants to search, read, create, organize, or update Cyime workspace documents, notes, drafts, folders, or persistent writing.
version: 0.1.0
allowed-tools: WebFetch, Bash
---

# Cyime Workspace

Use this skill to operate the user's Cyime workspace through MCP-first Markdown tools. Prefer MCP tools when the client supports MCP. Fall back to the REST Open API only when MCP is unavailable.

## Connection

- MCP endpoint: `http://127.0.0.1:8080/api/v1/mcp`
- REST Open API root: `http://127.0.0.1:8080/api/v1/open`
- REST OpenAPI: `http://localhost:5173/openapi.json`
- Browser OAuth authorize URL: `http://127.0.0.1:8080/api/v1/auth/skill/oauth/authorize`
- Browser OAuth token URL: `http://127.0.0.1:8080/api/v1/auth/skill/oauth/token`
- Authentication: set `Authorization: Bearer <cyime_api_token>` on every protected request.
- Never reveal, repeat, log, or summarize the raw API token in chat output.

## MCP Client Config Shapes

Different MCP clients wrap the same Cyime endpoint in different config shapes:

- MCP server map: use this when the client expects a named `mcpServers` map. This shape is useful for clients that manage multiple MCP servers in one config object.
- Streamable HTTP: use this when the client expects one server transport object with `transport: "streamable_http"`. This shape describes the HTTP MCP transport directly.

Both shapes use the same endpoint and bearer token:

```json
{
  "mcpServers": {
    "cyime-workspace": {
      "type": "http",
      "url": "http://127.0.0.1:8080/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer <CYIME_API_TOKEN>"
      }
    }
  }
}
```

```json
{
  "transport": "streamable_http",
  "url": "http://127.0.0.1:8080/api/v1/mcp",
  "headers": {
    "Authorization": "Bearer <CYIME_API_TOKEN>"
  },
  "timeout": 5,
  "sse_read_timeout": 300
}
```

## Browser OAuth Token Flow

When the client supports browser OAuth, use the OAuth 2.0 authorization-code flow with PKCE:

- Authorization URL: `http://127.0.0.1:8080/api/v1/auth/skill/oauth/authorize`
- Token URL: `http://127.0.0.1:8080/api/v1/auth/skill/oauth/token`
- Response type: `code`
- Token grant type: `authorization_code`
- Recommended scopes: `workspace:read`, `workspace:write`, `document:read`, `document:write`, `file:move`, `file:copy`
- Optional destructive scope: `file:delete`. Request it only when the user wants AI clients to move files to trash.

The user completes Cyime login in the browser, then Cyime shows a frontend consent page with the requesting client, redirect URI, requested scopes, and token lifetime. An authorization code is created only after the user approves. The token endpoint returns a Cyime API token as `access_token` with `token_type: Bearer`. Store it in the client's secret store and send it only as `Authorization: Bearer <access_token>`.

For deployed Cyime instances, server operators must configure:

- `PUBLIC_BASE_URL`: externally reachable frontend origin. Anonymous authorization requests redirect to `${PUBLIC_BASE_URL}/login`.
- `API_BASE_URL` or `PUBLIC_API_BASE_URL`: externally reachable backend origin. Used to generate auth URLs, provider callbacks, and OAuth return targets.
- `CYIME_SKILL_OAUTH_REDIRECT_URIS`: allowlist for production HTTPS `redirect_uri` values. Separate multiple values with commas. Loopback redirect URIs (`localhost` / `127.0.0.1`) and custom schemes are allowed by default.

## LobeHub Skill Token Configuration

When browser OAuth is unavailable, configure a secret skill variable:

- Key: `CYIME_API_TOKEN`
- Type: secret or password text
- Required: false when browser OAuth is available; true for manual-token clients
- Value: a Cyime API token created in Cyime user settings
- Recommended scopes: `workspace:read`, `workspace:write`, `document:read`, `document:write`, `file:move`, `file:copy`
- Optional destructive scope: `file:delete`. Enable it only when the user wants AI clients to move files to trash.

Use this secret only to send HTTP requests with `Authorization: Bearer $CYIME_API_TOKEN`. Do not place the token in `skill.md`, manifest URLs, prompts, chat messages, generated documents, or logs.

If the importing client does not support secret skill variables, ask the user to configure `CYIME_API_TOKEN` in that client's environment or secret manager before calling Cyime.

For deployed Cyime instances, import the frontend `skill.md` URL and use the MCP and REST URLs declared by that skill file.

## When To Use

Use Cyime proactively when the user asks to work with Cyime documents, notes, drafts, folders, saved writing, or durable knowledge.

Good triggers:

- The user mentions Cyime, workspace, document, note, draft, folder, or saved writing.
- The user asks to create, organize, summarize, rewrite, continue, or update a document.
- The generated answer would be useful as persistent content. In that case, suggest writing it to Cyime.

Do not call Cyime when:

- The user explicitly says not to use Cyime or not to use external tools.
- The task is only a general discussion and does not need saved workspace content.
- A write operation target is ambiguous. Ask a short clarification first.

## Core Workflow

1. Prefer MCP tool calls through `/api/v1/mcp`.
2. Locate known folders with `cyime_list_files`. Use `cyime_search_files` first when the target name, folder, or document content is only partially known.
3. Before editing an existing document, read it with `cyime_read_markdown_document`.
4. Convert user instructions into Markdown before writing.
5. Prefer incremental writes with `cyime_patch_markdown_document` when only part of a document changes.
6. Ask for confirmation before bulk copy/move operations or large rewrites.
7. Ask for explicit confirmation before using `cyime_delete_file`.

## MCP Tools

- `cyime_search_files`: search documents, folders, and media references by keyword. Use it when the target is not already known.
- `cyime_list_files`: list direct child folders and documents under the root or a known folder.
- `cyime_create_folder`: create a folder under the root or a parent folder.
- `cyime_create_markdown_document`: create a document from Markdown.
- `cyime_read_markdown_document`: read document content as Markdown.
- `cyime_update_markdown_document`: replace a whole document with Markdown. Use carefully.
- `cyime_patch_markdown_document`: apply focused Markdown edits. Prefer this for section-level changes.
- `cyime_rename_file`: rename a folder or document.
- `cyime_move_file`: move a folder or document to another folder or the root.
- `cyime_copy_file`: copy a folder or document.
- `cyime_delete_file`: move a folder or document to trash after explicit user confirmation.

Business errors from `tools/call` are returned as a normal JSON-RPC result with `result.isError: true`. Check `result.isError` before assuming the operation succeeded. `tools/list` includes MCP tool annotations such as `readOnlyHint` and `destructiveHint` for clients that use them.

MCP uses HTTP JSON-RPC. Send requests with `POST /api/v1/mcp` and `Content-Type: application/json`.

Tool scopes:

- `cyime_list_files`: `workspace:read`
- `cyime_search_files`: `workspace:read`
- `cyime_create_folder`: `workspace:write`
- `cyime_create_markdown_document`: `workspace:write`, `document:write`
- `cyime_read_markdown_document`: `document:read`
- `cyime_update_markdown_document`: `document:write`
- `cyime_patch_markdown_document`: `document:read`, `document:write`
- `cyime_rename_file`: `workspace:write`
- `cyime_move_file`: `file:move`
- `cyime_copy_file`: `file:copy`, `workspace:write`
- `cyime_delete_file`: `file:delete`

Example:

```http
POST /api/v1/mcp
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "cyime_list_files",
    "arguments": {
      "parentId": null,
      "limit": 50,
      "type": "all"
    }
  }
}
```

## REST Fallback

Use these endpoints only when MCP is unavailable:

```http
GET /api/v1/open/files?parent_id=null&limit=50&type=all
GET /api/v1/open/search?q=keyword&limit=10
POST /api/v1/open/folders
POST /api/v1/open/documents
PATCH /api/v1/open/files/{id}
PUT /api/v1/open/files/{id}/move
POST /api/v1/open/files/{id}/copy
DELETE /api/v1/open/files/{id}?type=document
GET /api/v1/open/documents/{documentId}/content?format=markdown
PUT /api/v1/open/documents/{documentId}/content
PATCH /api/v1/open/documents/{documentId}/content
```

REST requests and responses are documented at `/openapi.json`. They use the same token and scopes as MCP.

## Safety Rules

- Only delete files when the user clearly asks for deletion and confirms it. Delete moves files to trash; this skill does not expose permanent deletion.
- Do not overwrite a document without reading current content unless the user provided the latest content directly.
- If multiple matching documents are found, ask the user to choose unless the context clearly identifies one.
- If a write fails with a Markdown conversion error or converter unavailable error, tell the user the document was not changed and suggest retrying later or simplifying unsupported Markdown syntax.
- Keep Cyime-facing content in Markdown.
