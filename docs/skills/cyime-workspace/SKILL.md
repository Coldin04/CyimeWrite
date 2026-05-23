---
name: cyime-workspace
author: Cyime
description: This skill should be used when the user wants to read, create, organize, or update Cyime workspace documents, notes, drafts, folders, or persistent writing through Cyime integrations.
version: 0.1.0
allowed-tools: WebFetch, Bash
---

# Cyime Workspace

Use this skill to operate the user's Cyime workspace. Prefer MCP tools when the client supports MCP. Fall back to the REST Open API only when MCP is unavailable.

## Connection

- MCP endpoint: `http://127.0.0.1:8080/api/v1/mcp`
- REST Open API root: `http://127.0.0.1:8080/api/v1/open`
- REST OpenAPI: `http://localhost:5173/openapi.json`
- Authentication: set `Authorization: Bearer <cyime_api_token>` on every protected request.
- Never reveal, repeat, log, or summarize the raw API token in chat output.

## LobeHub Skill Token Configuration

When importing this skill into LobeHub Skills, configure a secret skill variable:

- Key: `CYIME_API_TOKEN`
- Type: secret or password text
- Required: true
- Value: a Cyime API token created in Cyime user settings
- Recommended scopes: `workspace:read`, `workspace:write`, `document:read`, `document:write`, `file:move`, `file:copy`

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
2. Locate the target with `cyime_list_files`. Use `parentId` to browse folders.
3. Before editing an existing document, read it with `cyime_read_markdown_document`.
4. Convert user instructions into Markdown before writing.
5. Prefer incremental writes with `cyime_patch_markdown_document` when only part of a document changes.
6. Use `baseVersion` on write requests. If the server returns a conflict, reread the document and retry carefully.
7. Ask for confirmation before bulk copy/move operations or large rewrites.

## MCP Tools

- `cyime_list_files`
- `cyime_create_folder`
- `cyime_create_markdown_document`
- `cyime_read_markdown_document`
- `cyime_update_markdown_document`
- `cyime_patch_markdown_document`
- `cyime_rename_file`
- `cyime_move_file`
- `cyime_copy_file`

MCP uses JSON-RPC. Example:

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
POST /api/v1/open/folders
POST /api/v1/open/documents
PATCH /api/v1/open/files/{id}
PUT /api/v1/open/files/{id}/move
POST /api/v1/open/files/{id}/copy
GET /api/v1/open/documents/{documentId}/content?format=markdown
PUT /api/v1/open/documents/{documentId}/content
PATCH /api/v1/open/documents/{documentId}/content
```

REST requests and responses are documented at `/openapi.json`. They use the same token and scopes as MCP.

## Safety Rules

- Do not delete content; this skill does not expose delete operations.
- Do not overwrite a document without reading current content unless the user provided the latest content directly.
- If multiple matching documents are found, ask the user to choose unless the context clearly identifies one.
- If a write fails with a Markdown conversion error or converter unavailable error, tell the user the document was not changed and suggest retrying later or simplifying unsupported Markdown syntax.
- Keep Cyime-facing content in Markdown.
