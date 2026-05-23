# Cyime Workspace Skill Spec v0.1

This spec defines how clients should use the Cyime Workspace skill.

## Purpose

The skill lets a client search, read, create, organize, and update a user's Cyime workspace when the user wants help with documents, notes, drafts, folders, or saved writing.

Supported capabilities:

- List folders and documents.
- Search documents, folders, and media references.
- Create folders.
- Create documents.
- Read document content.
- Update document content.
- Rename folders or documents.
- Move folders or documents.
- Copy folders or documents.
- Move folders or documents to trash.

## Public Skill URLs

Cyime exposes cache-friendly public skill metadata from the frontend app:

- Skill Markdown: `/skill.md`
- Manifest: `/manifest.json`
- OpenAPI: `/openapi.json`

These responses are pseudo-static. They only change when the frontend origin, public API base URL, or skill spec version changes, and they include HTTP cache headers.

## API Summary

All protected API calls use:

```http
Authorization: Bearer <cyime_api_token>
```

Clients that support browser OAuth should use the Cyime skill OAuth flow to obtain the bearer token:

- Authorization URL: `/api/v1/auth/skill/oauth/authorize`
- Token URL: `/api/v1/auth/skill/oauth/token`
- Flow: OAuth 2.0 authorization code with PKCE
- Response type: `code`
- Grant type: `authorization_code`

The user completes Cyime login in the browser. Cyime then shows a frontend-rendered consent page with the requesting client, redirect URI, requested scopes, and token lifetime. The server creates an authorization code only after the user approves. The token endpoint returns a Cyime API token as `access_token` with `token_type: Bearer`. The client stores that token in its secret store and sends it as:

```http
Authorization: Bearer <access_token>
```

Server-side deployment configuration for browser OAuth:

- `PUBLIC_BASE_URL`: externally reachable frontend origin. Anonymous authorization requests redirect to `${PUBLIC_BASE_URL}/login`.
- `API_BASE_URL` or `PUBLIC_API_BASE_URL`: externally reachable backend origin. Used to generate auth URLs, provider callbacks, and OAuth return targets.
- `CYIME_SKILL_OAUTH_REDIRECT_URIS`: allowlist for production HTTPS `redirect_uri` values. Separate multiple values with commas. Loopback redirect URIs (`localhost` / `127.0.0.1`) and custom schemes are allowed by default for local apps.

For clients without browser OAuth, expose a manually created token as a secret skill variable named `CYIME_API_TOKEN`.
The imported skill should instruct the client to set:

- Key: `CYIME_API_TOKEN`
- Type: secret or password text
- Required: true only for manual-token clients
- Value: a Cyime API token created in Cyime user settings

The token must never be embedded in `skill.md`, `/manifest.json`, `/openapi.json`, prompts, chat messages, generated documents, or logs. When making API requests, the client should read the secret and set:

```http
Authorization: Bearer $CYIME_API_TOKEN
```

Recommended scopes for the full workspace skill are:

- `workspace:read`
- `workspace:write`
- `document:read`
- `document:write`
- `file:move`
- `file:copy`

Optional destructive scope:

- `file:delete` — enable only when AI clients should be allowed to move files to trash.

Preferred MCP endpoint:

- `POST /api/v1/mcp`

## MCP Client Config Shapes

Clients may wrap the same MCP endpoint with different config schemas:

- MCP server map: a named `mcpServers` object for clients that manage multiple servers in one config.
- Streamable HTTP: a single transport object with `transport: "streamable_http"` for clients that configure one server directly.

Both shapes use the same endpoint and bearer token:

```json
{
  "mcpServers": {
    "cyime-workspace": {
      "type": "http",
      "url": "https://api.example.test/api/v1/mcp",
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
  "url": "https://api.example.test/api/v1/mcp",
  "headers": {
    "Authorization": "Bearer <CYIME_API_TOKEN>"
  },
  "timeout": 5,
  "sse_read_timeout": 300
}
```

MCP tools:

- `cyime_search_files`: search documents, folders, and media references by keyword.
- `cyime_list_files`: list direct child folders and documents under the root or a known folder.
- `cyime_create_folder`: create a workspace folder.
- `cyime_create_markdown_document`: create a document from Markdown.
- `cyime_read_markdown_document`: read document content as Markdown.
- `cyime_update_markdown_document`: replace a whole document with Markdown.
- `cyime_patch_markdown_document`: apply focused Markdown patch operations.
- `cyime_rename_file`: rename a folder or document.
- `cyime_move_file`: move a folder or document.
- `cyime_copy_file`: copy a folder or document.
- `cyime_delete_file`: move a folder or document to trash after explicit user confirmation.

REST fallback endpoints:

- `GET /api/v1/open/files`
- `GET /api/v1/open/search?q=keyword&limit=10`
- `POST /api/v1/open/folders`
- `POST /api/v1/open/documents`
- `PATCH /api/v1/open/files/{id}`
- `PUT /api/v1/open/files/{id}/move`
- `POST /api/v1/open/files/{id}/copy`
- `DELETE /api/v1/open/files/{id}?type=document`
- `GET /api/v1/open/documents/{id}/content?format=markdown`
- `PUT /api/v1/open/documents/{id}/content`
- `PATCH /api/v1/open/documents/{id}/content`

## When To Use

Use this skill proactively when the user asks to work with Cyime documents or workspace content.

Good triggers:

- The user mentions Cyime, workspace, documents, notes, drafts, folders, or saved writing.
- The user asks to create, organize, summarize, rewrite, continue, or update a document.
- The user asks to save generated content into Cyime.
- The user appears to need persistent writing output. In that case, suggest writing it to Cyime.

Do not call the skill when:

- The user explicitly says not to call Cyime or not to use external tools.
- The task is only a general writing discussion and does not need existing or saved Cyime content.
- The operation would modify existing content but the target document is unclear.

## MCP Contract

Clients should prefer MCP when available. Cyime exposes an HTTP JSON-RPC MCP endpoint at `POST /api/v1/mcp` with:

- `initialize`
- `ping`
- `tools/list`
- `tools/call`

Business errors from `tools/call` are returned as a normal JSON-RPC result with `result.isError: true`. Clients must inspect `result.isError` before assuming a tool call succeeded. Protocol errors such as invalid JSON-RPC requests or insufficient token scope are returned as JSON-RPC errors. `tools/list` includes MCP tool annotations such as `readOnlyHint` and `destructiveHint` for clients that use them.

For target discovery, use `cyime_search_files` when the user provides a partial title, content clue, folder name, or media reference. Use `cyime_list_files` when browsing a known folder hierarchy.

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

Example tool call:

```json
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

## Markdown Contract

Clients should read and write document content as Markdown. The API may store content internally in another format, but skill-facing endpoints should accept Markdown input and return Markdown output.

Recommended read response:

```json
{
  "format": "markdown",
  "content": "# Title\n\nDocument body..."
}
```

Recommended full update request:

```json
{
  "format": "markdown",
  "content": "# Title\n\nUpdated body..."
}
```

## Incremental Writes

Prefer incremental writes when editing existing documents.

Recommended patch request:

```json
{
  "format": "markdown",
  "operations": [
    {
      "type": "replace",
      "target": "section",
      "heading": "Weekly Summary",
      "content": "New section content..."
    }
  ]
}
```

Allowed operation types:

- `append`
- `prepend`
- `replace`
- `insert_after`
- `insert_before`

## Safety Rules

- Never expose access tokens in chat output.
- Ask for confirmation before large rewrites, bulk moves, or destructive actions.
- Only delete files when the user clearly requests deletion and confirms it. Delete moves files to trash; the MCP skill does not expose permanent deletion.
- Before modifying an existing document, read the current content unless the user provided the latest content directly.
- If multiple matching documents are found, ask the user to choose or use the most likely match only when the context is clear.
- If a write fails with a Markdown conversion error or converter unavailable error, tell the user the document was not changed and suggest retrying later or simplifying unsupported Markdown syntax.
