import { apiBaseUrl } from '$lib/config/api';

export const skillSpecVersion = '2026-05-23.2';

export type SkillDocument = {
	apiBaseUrl: string;
	apiRootUrl: string;
	openApiRootUrl: string;
	mcpUrl: string;
	frontendOrigin: string;
	openapiUrl: string;
	manifestUrl: string;
	etag: string;
};

export function buildSkillDocument(frontendOrigin: string): SkillDocument {
	const normalizedOrigin = frontendOrigin.replace(/\/+$/, '');
	const apiRootUrl = `${apiBaseUrl}/api/v1`;
	const openApiRootUrl = `${apiRootUrl}/open`;
	const mcpUrl = `${apiRootUrl}/mcp`;
	const openapiUrl = `${normalizedOrigin}/openapi.json`;
	const manifestUrl = `${normalizedOrigin}/manifest.json`;
	const etagHash = hashForETag(`${skillSpecVersion}|${apiBaseUrl}|${normalizedOrigin}`);

	return {
		apiBaseUrl,
		apiRootUrl,
		openApiRootUrl,
		mcpUrl,
		frontendOrigin: normalizedOrigin,
		openapiUrl,
		manifestUrl,
		etag: `W/"cyime-skill-${etagHash}"`
	};
}

export function cacheHeaders(etag: string, contentType: string): HeadersInit {
	return {
		'cache-control': 'public, max-age=3600, stale-while-revalidate=86400',
		'content-type': contentType,
		etag
	};
}

export function matchesETag(request: Request, etag: string): boolean {
	const header = request.headers.get('if-none-match');
	if (!header) return false;
	return header
		.split(',')
		.map((value) => value.trim())
		.some((value) => value === etag || value === '*');
}

function hashForETag(input: string): string {
	let hash = 0x811c9dc5;
	for (let index = 0; index < input.length; index += 1) {
		hash ^= input.charCodeAt(index);
		hash = Math.imul(hash, 0x01000193);
	}
	return (hash >>> 0).toString(16).padStart(8, '0');
}

export function renderSkillMarkdown(document: SkillDocument): string {
	return `---
name: cyime-workspace
author: Cyime
description: This skill should be used when the user wants to read, create, organize, or update Cyime workspace documents, notes, drafts, folders, or persistent writing through Cyime integrations.
version: 0.1.0
allowed-tools: WebFetch, Bash
---

# Cyime Workspace

Use this skill to operate the user's Cyime workspace. Prefer the MCP endpoint when the client supports MCP tools. Fall back to the REST Open API only when MCP is unavailable.

## Connection

- MCP endpoint: ${document.mcpUrl}
- REST Open API root: ${document.openApiRootUrl}
- REST OpenAPI: ${document.openapiUrl}
- Authentication: set \`Authorization: Bearer <cyime_api_token>\` on every protected request.
- Never reveal, repeat, log, or summarize the raw API token in chat output.

## LobeHub Skill Token Configuration

When importing this skill into LobeHub Skills, configure a secret skill variable:

- Key: \`CYIME_API_TOKEN\`
- Type: secret or password text
- Required: true
- Value: a Cyime API token created in Cyime user settings
- Recommended scopes: \`workspace:read\`, \`workspace:write\`, \`document:read\`, \`document:write\`, \`file:move\`, \`file:copy\`

Use this secret only to send HTTP requests with \`Authorization: Bearer $CYIME_API_TOKEN\`. Do not place the token in \`skill.md\`, manifest URLs, prompts, chat messages, generated documents, or logs.

If the importing client does not support secret skill variables, ask the user to configure \`CYIME_API_TOKEN\` in that client's environment or secret manager before calling Cyime.

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

1. Prefer MCP tool calls through \`${document.mcpUrl}\`.
2. Locate the target with \`cyime_list_files\`. Use \`parentId\` to browse folders.
3. Before editing an existing document, read it with \`cyime_read_markdown_document\`.
4. Convert user instructions into Markdown before writing.
5. Prefer incremental writes with \`cyime_patch_markdown_document\` when only part of a document changes.
6. Use \`baseVersion\` on write requests. If the server returns a conflict, reread the document and retry carefully.
7. Ask for confirmation before bulk copy/move operations or large rewrites.

## MCP Tools

- \`cyime_list_files\`
- \`cyime_create_folder\`
- \`cyime_create_markdown_document\`
- \`cyime_read_markdown_document\`
- \`cyime_update_markdown_document\`
- \`cyime_patch_markdown_document\`
- \`cyime_rename_file\`
- \`cyime_move_file\`
- \`cyime_copy_file\`

MCP uses JSON-RPC. Example:

\`\`\`http
POST ${document.mcpUrl}
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
\`\`\`

## REST Fallback

Use these endpoints only when MCP is unavailable:

\`\`\`http
GET /api/v1/open/files?parent_id=null&limit=50&type=all
POST /api/v1/open/folders
POST /api/v1/open/documents
PATCH /api/v1/open/files/{id}
PUT /api/v1/open/files/{id}/move
POST /api/v1/open/files/{id}/copy
GET /api/v1/open/documents/{documentId}/content?format=markdown
PUT /api/v1/open/documents/{documentId}/content
PATCH /api/v1/open/documents/{documentId}/content
\`\`\`

REST requests and responses are documented at ${document.openapiUrl}. They use the same token and scopes as MCP.

## Safety Rules

- Do not delete content; this skill does not expose delete operations.
- Do not overwrite a document without reading current content unless the user provided the latest content directly.
- If multiple matching documents are found, ask the user to choose unless the context clearly identifies one.
- If a write fails with a Markdown conversion error or converter unavailable error, tell the user the document was not changed and suggest retrying later or simplifying unsupported Markdown syntax.
- Keep Cyime-facing content in Markdown.`;
}

export function buildSkillManifest(document: SkillDocument) {
	return {
		schemaVersion: 'cyime.skill.v1',
		version: skillSpecVersion,
		name: 'Cyime Workspace',
		description:
			'Read, create, organize, and update Cyime workspace documents and folders through MCP-first Markdown integrations.',
		mcpUrl: document.mcpUrl,
		apiBaseUrl: document.openApiRootUrl,
		restApiRootUrl: document.openApiRootUrl,
		openapiUrl: document.openapiUrl,
		auth: {
			type: 'http',
			scheme: 'bearer',
			header: 'Authorization',
			description: 'Use a Cyime API token as: Authorization: Bearer <token>. For LobeHub Skills, configure it as the secret CYIME_API_TOKEN.'
		},
		tokenConfig: {
			environmentVariable: 'CYIME_API_TOKEN',
			type: 'secret',
			required: true,
			header: 'Authorization',
			valueTemplate: 'Bearer ${CYIME_API_TOKEN}',
			description:
				'Configure CYIME_API_TOKEN as a LobeHub skill secret or client environment secret. Never publish the token in skill.md or manifest.json.'
		},
		instructions: [
			'Use Cyime proactively when the user works with Cyime documents, notes, folders, drafts, or persistent writing output.',
			'Prefer the MCP endpoint and its tools when the client supports MCP.',
			'Use the REST Open API only as a fallback when MCP is unavailable.',
			'Do not call Cyime when the user explicitly says not to use Cyime or external tools.',
			'Read and write document content as Markdown.',
			'Prefer incremental markdown patch operations. Use baseVersion and reread on version conflicts.',
			'Suggest writing useful long-form or persistent output to Cyime when appropriate.'
		],
		capabilities: [
			'cyime_list_files',
			'cyime_create_folder',
			'cyime_create_markdown_document',
			'cyime_read_markdown_document',
			'cyime_update_markdown_document',
			'cyime_patch_markdown_document',
			'cyime_rename_file',
			'cyime_move_file',
			'cyime_copy_file'
		]
	};
}

export function buildOpenAPISpec(document: SkillDocument) {
	return {
		openapi: '3.1.0',
		info: {
			title: 'Cyime Workspace Open API',
			version: skillSpecVersion,
			description: 'REST fallback for Markdown-first Cyime workspace integrations. Prefer MCP when available.'
		},
		servers: [{ url: document.apiBaseUrl }],
		security: [{ BearerAuth: [] }],
		paths: {
			'/api/v1/open/files': {
				get: withParameters(
					operation('listFiles', 'List workspace folders and documents.', ['workspace:read'], null, fileListSchema()),
					fileListParameters()
				)
			},
			'/api/v1/open/folders': {
				post: operation(
					'createFolder',
					'Create a folder.',
					['workspace:write'],
					createFolderSchema(),
					fileOperationSchema()
				)
			},
			'/api/v1/open/documents': {
				post: operation(
					'createMarkdownDocument',
					'Create a Markdown document.',
					['workspace:write', 'document:write'],
					createMarkdownDocumentSchema(),
					createMarkdownDocumentResponseSchema()
				)
			},
			'/api/v1/open/files/{id}': {
				patch: withParameters(
					operation(
						'renameFile',
						'Rename a folder or document.',
						['workspace:write'],
						renameFileSchema(),
						fileOperationSchema()
					),
					[pathIDParameter()]
				)
			},
			'/api/v1/open/files/{id}/move': {
				put: withParameters(
					operation('moveFile', 'Move a folder or document.', ['file:move'], moveFileSchema(), fileOperationSchema()),
					[pathIDParameter()]
				)
			},
			'/api/v1/open/files/{id}/copy': {
				post: withParameters(
					operation(
						'copyFile',
						'Copy a folder or document.',
						['file:copy', 'workspace:write'],
						copyFileSchema(),
						fileOperationSchema()
					),
					[pathIDParameter()]
				)
			},
			'/api/v1/open/documents/{id}/content': {
				get: withParameters(
					operation(
						'getMarkdownContent',
						'Read document content as Markdown.',
						['document:read'],
						null,
						markdownContentSchema()
					),
					[pathIDParameter(), queryParameter('format', 'markdown', 'Content format. Use markdown.')]
				),
				put: withParameters(
					operation(
						'updateMarkdownContent',
						'Replace document content with Markdown.',
						['document:write'],
						updateMarkdownSchema(),
						markdownUpdateResultSchema()
					),
					[pathIDParameter()]
				),
				patch: withParameters(
					operation(
						'patchMarkdownContent',
						'Apply incremental Markdown patch operations.',
						['document:read', 'document:write'],
						patchMarkdownSchema(),
						markdownUpdateResultSchema()
					),
					[pathIDParameter()]
				)
			}
		},
		components: {
			securitySchemes: {
				BearerAuth: {
					type: 'http',
					scheme: 'bearer',
					bearerFormat: 'Cyime API Token',
					description:
						'Use the LobeHub skill secret or client environment secret CYIME_API_TOKEN as: Authorization: Bearer <token>.'
				}
			}
		}
	};
}

function operation(
	operationId: string,
	summary: string,
	scopes: string[],
	requestSchema: Record<string, unknown> | null,
	responseSchema: Record<string, unknown>
) {
	const value: Record<string, unknown> = {
		operationId,
		summary,
		'x-cyime-scopes': scopes,
		responses: {
			'200': jsonResponse(responseSchema),
			'201': jsonResponse(responseSchema),
			'400': errorResponse(),
			'401': errorResponse(),
			'403': errorResponse(),
			'404': errorResponse(),
			'409': errorResponse()
		}
	};
	if (requestSchema) {
		value.requestBody = {
			required: true,
			content: {
				'application/json': { schema: requestSchema }
			}
		};
	}
	return value;
}

function withParameters(operation: Record<string, unknown>, parameters: Record<string, unknown>[]) {
	operation.parameters = parameters;
	return operation;
}

function pathIDParameter() {
	return {
		name: 'id',
		in: 'path',
		required: true,
		description: 'File or document UUID.',
		schema: { type: 'string', format: 'uuid' }
	};
}

function queryParameter(name: string, example: string, description: string) {
	return {
		name,
		in: 'query',
		required: false,
		description,
		schema: { type: 'string' },
		example
	};
}

function fileListParameters() {
	return [
		queryParameter('parent_id', 'null', 'Folder UUID, empty, or null for root.'),
		queryParameter('limit', '50', 'Maximum number of items.'),
		queryParameter('offset', '0', 'Pagination offset.'),
		queryParameter('sort_by', 'updated_at', 'Sort field.'),
		queryParameter('order', 'desc', 'Sort order: asc or desc.'),
		queryParameter('type', 'all', 'Filter: all, folder, or document.')
	];
}

function jsonResponse(schema: Record<string, unknown>) {
	return {
		description: 'OK',
		content: {
			'application/json': { schema }
		}
	};
}

function errorResponse() {
	return {
		description: 'Error response',
		content: {
			'application/json': {
				schema: objectSchema({
					error: { type: 'string' },
					message: { type: 'string' }
				})
			}
		}
	};
}

function objectSchema(properties: Record<string, unknown>, required: string[] = []) {
	return {
		type: 'object',
		properties,
		required
	};
}

function stringSchema(description: string) {
	return { type: 'string', description };
}

function nullableUUIDSchema(description: string) {
	return { type: ['string', 'null'], format: 'uuid', description };
}

function fileItemSchema() {
	return objectSchema(
		{
			id: stringSchema('Item UUID.'),
			type: { type: 'string', enum: ['folder', 'document'] },
			name: stringSchema('Folder name or document title.'),
			parentId: nullableUUIDSchema('Parent folder ID for folders.'),
			folderId: nullableUUIDSchema('Parent folder ID for documents.'),
			title: stringSchema('Document title when type is document.'),
			excerpt: stringSchema('Document excerpt when type is document.'),
			createdAt: stringSchema('ISO timestamp.'),
			updatedAt: stringSchema('ISO timestamp.')
		},
		['id', 'type', 'name', 'createdAt', 'updatedAt']
	);
}

function fileListSchema() {
	return objectSchema(
		{
			items: { type: 'array', items: fileItemSchema() },
			hasMore: { type: 'boolean' },
			total: { type: 'integer' }
		},
		['items', 'hasMore', 'total']
	);
}

function fileOperationSchema() {
	return objectSchema(
		{
			success: { type: 'boolean' },
			item: fileItemSchema()
		},
		['success', 'item']
	);
}

function createFolderSchema() {
	return objectSchema(
		{
			name: stringSchema('Folder name.'),
			description: { type: ['string', 'null'] },
			parentId: nullableUUIDSchema('Parent folder ID. Use null for workspace root.')
		},
		['name']
	);
}

function createMarkdownDocumentSchema() {
	return objectSchema(
		{
			title: stringSchema('Document title.'),
			format: { type: 'string', enum: ['markdown'] },
			content: stringSchema('Markdown content.'),
			folderId: nullableUUIDSchema('Parent folder ID. Use null for workspace root.'),
			preferredImageTargetId: stringSchema('Optional image target ID.')
		},
		['title', 'format', 'content']
	);
}

function createMarkdownDocumentResponseSchema() {
	return objectSchema(
		{
			id: stringSchema('Document UUID.'),
			type: { type: 'string', enum: ['document'] },
			title: stringSchema('Document title.'),
			folderId: nullableUUIDSchema('Parent folder ID.'),
			version: { type: 'integer' },
			createdAt: stringSchema('ISO timestamp.'),
			updatedAt: stringSchema('ISO timestamp.')
		},
		['id', 'type', 'title', 'version', 'createdAt', 'updatedAt']
	);
}

function renameFileSchema() {
	return objectSchema(
		{
			type: { type: 'string', enum: ['folder', 'document'] },
			name: stringSchema('New folder name or document title.')
		},
		['type', 'name']
	);
}

function moveFileSchema() {
	return objectSchema(
		{
			type: { type: 'string', enum: ['folder', 'document'] },
			destinationFolderId: nullableUUIDSchema('Destination parent folder ID. Use null for workspace root.')
		},
		['type']
	);
}

function copyFileSchema() {
	return objectSchema(
		{
			type: { type: 'string', enum: ['folder', 'document'] },
			destinationFolderId: nullableUUIDSchema('Destination parent folder ID. Use null for workspace root.'),
			name: stringSchema('Optional copy name/title. Omit or empty to auto-generate.')
		},
		['type']
	);
}

function markdownContentSchema() {
	return objectSchema(
		{
			format: { type: 'string', enum: ['markdown'] },
			content: stringSchema('Markdown content.'),
			version: { type: 'integer' },
			updatedAt: stringSchema('ISO timestamp.')
		},
		['format', 'content', 'version', 'updatedAt']
	);
}

function updateMarkdownSchema() {
	return objectSchema(
		{
			format: { type: 'string', enum: ['markdown'] },
			content: stringSchema('Full Markdown content.'),
			baseVersion: { type: ['integer', 'null'] }
		},
		['format', 'content']
	);
}

function patchMarkdownSchema() {
	return objectSchema(
		{
			format: { type: 'string', enum: ['markdown'] },
			baseVersion: { type: ['integer', 'null'] },
			operations: {
				type: 'array',
				items: objectSchema(
					{
						type: {
							type: 'string',
							enum: ['append', 'prepend', 'replace', 'insert_after', 'insert_before']
						},
						target: stringSchema('Optional target, for example section.'),
						heading: stringSchema('Optional Markdown heading.'),
						match: stringSchema('Optional exact text match.'),
						content: stringSchema('Markdown fragment.')
					},
					['type', 'content']
				)
			}
		},
		['format', 'operations']
	);
}

function markdownUpdateResultSchema() {
	return objectSchema(
		{
			success: { type: 'boolean' },
			version: { type: 'integer' },
			updatedAt: stringSchema('ISO timestamp.')
		},
		['success', 'version', 'updatedAt']
	);
}
