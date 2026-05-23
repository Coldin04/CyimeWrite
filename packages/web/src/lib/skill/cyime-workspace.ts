import { apiBaseUrl } from '$lib/config/api';

export const skillSpecVersion = '2026-05-23.9';

export type SkillDocument = {
	apiBaseUrl: string;
	apiRootUrl: string;
	openApiRootUrl: string;
	mcpUrl: string;
	oauthAuthorizeUrl: string;
	oauthTokenUrl: string;
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
	const oauthAuthorizeUrl = `${apiRootUrl}/auth/skill/oauth/authorize`;
	const oauthTokenUrl = `${apiRootUrl}/auth/skill/oauth/token`;
	const openapiUrl = `${normalizedOrigin}/openapi.json`;
	const manifestUrl = `${normalizedOrigin}/manifest.json`;
	const etagHash = hashForETag(`${skillSpecVersion}|${apiBaseUrl}|${normalizedOrigin}`);

	return {
		apiBaseUrl,
		apiRootUrl,
		openApiRootUrl,
		mcpUrl,
		oauthAuthorizeUrl,
		oauthTokenUrl,
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
description: Use this skill when the user wants to search, read, create, organize, or update Cyime workspace documents, notes, drafts, folders, or persistent writing.
version: 0.1.0
allowed-tools: WebFetch, Bash
---

# Cyime Workspace

Use this skill to operate the user's Cyime workspace through MCP-first Markdown tools. Prefer the MCP endpoint when the client supports MCP tools. Fall back to the REST Open API only when MCP is unavailable.

## Connection

- MCP endpoint: ${document.mcpUrl}
- REST Open API root: ${document.openApiRootUrl}
- REST OpenAPI: ${document.openapiUrl}
- Browser OAuth authorize URL: ${document.oauthAuthorizeUrl}
- Browser OAuth token URL: ${document.oauthTokenUrl}
- Authentication: set \`Authorization: Bearer <cyime_api_token>\` on every protected request.
- Never reveal, repeat, log, or summarize the raw API token in chat output.

## MCP Client Config Shapes

Different MCP clients wrap the same Cyime endpoint in different config shapes:

- MCP server map: use this when the client expects a named \`mcpServers\` map. This shape is useful for clients that manage multiple MCP servers in one config object.
- Streamable HTTP: use this when the client expects one server transport object with \`transport: "streamable_http"\`. This shape describes the HTTP MCP transport directly.

Both shapes use the same endpoint and bearer token:

\`\`\`json
{
  "mcpServers": {
    "cyime-workspace": {
      "type": "http",
      "url": "${document.mcpUrl}",
      "headers": {
        "Authorization": "Bearer <CYIME_API_TOKEN>"
      }
    }
  }
}
\`\`\`

\`\`\`json
{
  "transport": "streamable_http",
  "url": "${document.mcpUrl}",
  "headers": {
    "Authorization": "Bearer <CYIME_API_TOKEN>"
  },
  "timeout": 5,
  "sse_read_timeout": 300
}
\`\`\`

## Browser OAuth Token Flow

When the client supports browser OAuth, use the OAuth 2.0 authorization-code flow with PKCE:

- Authorization URL: \`${document.oauthAuthorizeUrl}\`
- Token URL: \`${document.oauthTokenUrl}\`
- Response type: \`code\`
- Token grant type: \`authorization_code\`
- Recommended scopes: \`workspace:read\`, \`workspace:write\`, \`document:read\`, \`document:write\`, \`file:move\`, \`file:copy\`
- Optional destructive scope: \`file:delete\`. Request it only when the user wants AI clients to move files to trash.

The user completes Cyime login in the browser, then Cyime shows a frontend consent page with the requesting client, redirect URI, requested scopes, and token lifetime. An authorization code is created only after the user approves. The token endpoint returns a Cyime API token as \`access_token\` with \`token_type: Bearer\`. Store it in the client's secret store and send it only as \`Authorization: Bearer <access_token>\`.

For deployed Cyime instances, server operators must configure:

- \`PUBLIC_BASE_URL\`: externally reachable frontend origin. Anonymous authorization requests redirect to \`\${PUBLIC_BASE_URL}/login\`.
- \`API_BASE_URL\` or \`PUBLIC_API_BASE_URL\`: externally reachable backend origin. Used to generate auth URLs, provider callbacks, and OAuth return targets.
- \`CYIME_SKILL_OAUTH_REDIRECT_URIS\`: allowlist for production HTTPS \`redirect_uri\` values. Separate multiple values with commas. Loopback redirect URIs (\`localhost\` / \`127.0.0.1\`) and custom schemes are allowed by default.

## LobeHub Skill Token Configuration

When browser OAuth is unavailable, configure a secret skill variable:

- Key: \`CYIME_API_TOKEN\`
- Type: secret or password text
- Required: false when browser OAuth is available; true for manual-token clients
- Value: a Cyime API token created in Cyime user settings
- Recommended scopes: \`workspace:read\`, \`workspace:write\`, \`document:read\`, \`document:write\`, \`file:move\`, \`file:copy\`
- Optional destructive scope: \`file:delete\`. Enable it only when the user wants AI clients to move files to trash.

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
2. Locate known folders with \`cyime_list_files\`. Use \`cyime_search_files\` first when the target name, folder, or document content is only partially known.
3. Before editing an existing document, read it with \`cyime_read_markdown_document\`.
4. Convert user instructions into Markdown before writing.
5. Prefer incremental writes with \`cyime_patch_markdown_document\` when only part of a document changes.
6. Ask for confirmation before bulk copy/move operations or large rewrites.
7. Ask for explicit confirmation before using \`cyime_delete_file\`.

## MCP Tools

- \`cyime_search_files\`: search documents, folders, and media references by keyword. Use it when the target is not already known.
- \`cyime_list_files\`: list direct child folders and documents under the root or a known folder.
- \`cyime_create_folder\`: create a folder under the root or a parent folder.
- \`cyime_create_markdown_document\`: create a document from Markdown.
- \`cyime_read_markdown_document\`: read document content as Markdown.
- \`cyime_update_markdown_document\`: replace a whole document with Markdown. Use carefully.
- \`cyime_patch_markdown_document\`: apply focused Markdown edits. Prefer this for section-level changes.
- \`cyime_rename_file\`: rename a folder or document.
- \`cyime_move_file\`: move a folder or document to another folder or the root.
- \`cyime_copy_file\`: copy a folder or document.
- \`cyime_delete_file\`: move a folder or document to trash after explicit user confirmation.

Business errors from \`tools/call\` are returned as a normal JSON-RPC result with \`result.isError: true\`. Check \`result.isError\` before assuming the operation succeeded. \`tools/list\` includes MCP tool annotations such as \`readOnlyHint\` and \`destructiveHint\` for clients that use them.

MCP uses HTTP JSON-RPC. Send requests with \`POST ${document.mcpUrl}\` and \`Content-Type: application/json\`.

Tool scopes:

- \`cyime_list_files\`: \`workspace:read\`
- \`cyime_search_files\`: \`workspace:read\`
- \`cyime_create_folder\`: \`workspace:write\`
- \`cyime_create_markdown_document\`: \`workspace:write\`, \`document:write\`
- \`cyime_read_markdown_document\`: \`document:read\`
- \`cyime_update_markdown_document\`: \`document:write\`
- \`cyime_patch_markdown_document\`: \`document:read\`, \`document:write\`
- \`cyime_rename_file\`: \`workspace:write\`
- \`cyime_move_file\`: \`file:move\`
- \`cyime_copy_file\`: \`file:copy\`, \`workspace:write\`
- \`cyime_delete_file\`: \`file:delete\`

Example:

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
\`\`\`

REST requests and responses are documented at ${document.openapiUrl}. They use the same token and scopes as MCP.

## Safety Rules

- Only delete files when the user clearly asks for deletion and confirms it. Delete moves files to trash; this skill does not expose permanent deletion.
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
			'Search, read, create, organize, and update Cyime workspace documents and folders through MCP-first Markdown integrations.',
		mcpUrl: document.mcpUrl,
		apiBaseUrl: document.openApiRootUrl,
		restApiRootUrl: document.openApiRootUrl,
		openapiUrl: document.openapiUrl,
		mcpConfigTemplates: mcpConfigTemplates(document),
		auth: {
			type: 'oauth2',
			flow: 'authorization_code_pkce',
			authorizationUrl: document.oauthAuthorizeUrl,
			tokenUrl: document.oauthTokenUrl,
			scheme: 'bearer',
			header: 'Authorization',
			description:
				'Use browser OAuth to obtain a Cyime API token, then send it as Authorization: Bearer <token>. Manual CYIME_API_TOKEN secrets remain supported.'
		},
		oauth: {
			type: 'oauth2',
			flow: 'authorization_code_pkce',
			authorizationUrl: document.oauthAuthorizeUrl,
			tokenUrl: document.oauthTokenUrl,
			responseType: 'code',
			grantType: 'authorization_code',
			tokenType: 'Bearer',
			scopes: oauthScopes(),
			defaultScopes: [
				'workspace:read',
				'workspace:write',
				'document:read',
				'document:write',
				'file:move',
				'file:copy'
			],
			deploymentConfig: {
				publicBaseUrl: 'PUBLIC_BASE_URL',
				publicApiBaseUrl: 'API_BASE_URL or PUBLIC_API_BASE_URL',
				redirectUriAllowlist: 'CYIME_SKILL_OAUTH_REDIRECT_URIS'
			}
		},
		tokenConfig: {
			environmentVariable: 'CYIME_API_TOKEN',
			type: 'secret',
			required: false,
			header: 'Authorization',
			valueTemplate: 'Bearer ${CYIME_API_TOKEN}',
			description:
				'Fallback for clients without browser OAuth. Configure CYIME_API_TOKEN as a LobeHub skill secret or client environment secret. Never publish the token in skill.md or manifest.json.'
		},
		instructions: [
			'Use Cyime proactively when the user works with Cyime documents, notes, folders, drafts, or persistent writing output.',
			'Prefer the MCP endpoint and its tools when the client supports MCP.',
			'Use the REST Open API only as a fallback when MCP is unavailable.',
			'Use cyime_search_files when the target name, folder, or document content is only partially known.',
			'Do not call Cyime when the user explicitly says not to use Cyime or external tools.',
			'Read and write document content as Markdown.',
			'Prefer incremental markdown patch operations for focused edits.',
			'Use cyime_delete_file only after explicit user confirmation. It moves files to trash and does not permanently delete them.',
			'Suggest writing useful long-form or persistent output to Cyime when appropriate.'
		],
		capabilities: [
			'cyime_list_files',
			'cyime_search_files',
			'cyime_create_folder',
			'cyime_create_markdown_document',
			'cyime_read_markdown_document',
			'cyime_update_markdown_document',
			'cyime_patch_markdown_document',
			'cyime_rename_file',
			'cyime_move_file',
			'cyime_copy_file',
			'cyime_delete_file'
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
		security: [{ CyimeSkillOAuth: [] }, { BearerAuth: [] }],
		paths: {
			'/api/v1/open/files': {
				get: withParameters(
					operation(
						'listFiles',
						'List direct child folders and documents under the root or a known folder.',
						['workspace:read'],
						null,
						fileListSchema()
					),
					fileListParameters()
				)
			},
			'/api/v1/open/search': {
				get: withParameters(
					operation(
						'searchFiles',
						'Search workspace documents, folders, and media references by keyword.',
						['workspace:read'],
						null,
						searchResponseSchema()
					),
					searchParameters()
				)
			},
			'/api/v1/open/folders': {
				post: operation(
					'createFolder',
					'Create a workspace folder.',
					['workspace:write'],
					createFolderSchema(),
					fileOperationSchema()
				)
			},
			'/api/v1/open/documents': {
				post: operation(
					'createMarkdownDocument',
					'Create a document from Markdown.',
					['workspace:write', 'document:write'],
					createMarkdownDocumentSchema(),
					createMarkdownDocumentResponseSchema()
				)
			},
			'/api/v1/open/files/{id}': {
				patch: withParameters(
					operation(
						'renameFile',
						'Rename a folder or document without changing content or location.',
						['workspace:write'],
						renameFileSchema(),
						fileOperationSchema()
					),
					[pathIDParameter()]
				),
				delete: withParameters(
					operation(
						'deleteFile',
						'Move a folder or document to trash.',
						['file:delete'],
						null,
						deleteFileResponseSchema()
					),
					[pathIDParameter(), queryParameter('type', 'document', 'File type: folder or document.', true)]
				)
			},
			'/api/v1/open/files/{id}/move': {
				put: withParameters(
					operation(
						'moveFile',
						'Move a folder or document to another folder or the root.',
						['file:move'],
						moveFileSchema(),
						fileOperationSchema()
					),
					[pathIDParameter()]
				)
			},
			'/api/v1/open/files/{id}/copy': {
				post: withParameters(
					operation(
						'copyFile',
						'Copy a folder or document to another folder or the root.',
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
						'Replace a whole document with Markdown.',
						['document:write'],
						updateMarkdownSchema(),
						markdownUpdateResultSchema()
					),
					[pathIDParameter()]
				),
				patch: withParameters(
					operation(
						'patchMarkdownContent',
						'Apply focused Markdown patch operations.',
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
				CyimeSkillOAuth: {
					type: 'oauth2',
					description:
						'Browser OAuth authorization-code flow with PKCE. The token response access_token is a Cyime API token used as a Bearer token.',
					flows: {
						authorizationCode: {
							authorizationUrl: document.oauthAuthorizeUrl,
							tokenUrl: document.oauthTokenUrl,
							scopes: oauthScopes()
						}
					}
				},
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

function mcpConfigTemplates(document: SkillDocument) {
	return {
		serverMap: {
			name: 'MCP server map',
			description:
				'For clients that keep multiple MCP servers in one named mcpServers object keyed by server id.',
			config: {
				mcpServers: {
					'cyime-workspace': {
						type: 'http',
						url: document.mcpUrl,
						headers: {
							Authorization: 'Bearer ${CYIME_API_TOKEN}'
						}
					}
				}
			}
		},
		streamableHttp: {
			name: 'Streamable HTTP',
			description:
				'For clients that configure one MCP server as a direct Streamable HTTP transport object.',
			config: {
				transport: 'streamable_http',
				url: document.mcpUrl,
				headers: {
					Authorization: 'Bearer ${CYIME_API_TOKEN}'
				},
				timeout: 5,
				sse_read_timeout: 300
			}
		}
	};
}

function oauthScopes() {
	return {
		'workspace:read': 'Read workspace folders, documents, and search results.',
		'workspace:write': 'Create folders, create documents, and rename workspace items.',
		'document:read': 'Read Markdown document content.',
		'document:write': 'Create or update Markdown document content.',
		'file:move': 'Move files or folders.',
		'file:copy': 'Copy files or folders.',
		'file:delete': 'Move files or folders to trash. Request only when needed.'
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
		security: [{ CyimeSkillOAuth: scopes }, { BearerAuth: [] }],
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

function queryParameter(name: string, example: string, description: string, required = false) {
	return {
		name,
		in: 'query',
		required,
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

function searchParameters() {
	return [
		queryParameter('q', 'keyword', 'Search keywords.', true),
		queryParameter('limit', '10', 'Maximum results per category.')
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

function searchResponseSchema() {
	return objectSchema(
		{
			query: stringSchema('Normalized search query.'),
			documents: { type: 'array', items: searchDocumentItemSchema() },
			folders: { type: 'array', items: searchFolderItemSchema() },
			media: { type: 'array', items: searchMediaItemSchema() },
			total: { type: 'integer' }
		},
		['query', 'documents', 'folders', 'media', 'total']
	);
}

function searchDocumentItemSchema() {
	return objectSchema(
		{
			id: stringSchema('Document UUID.'),
			title: stringSchema('Document title.'),
			excerpt: stringSchema('Matched excerpt.'),
			documentType: stringSchema('Document type.'),
			preferredImageTargetId: stringSchema('Preferred image target ID.'),
			myRole: stringSchema('Current user role.'),
			publicAccess: stringSchema('Public access mode.'),
			publicUrl: stringSchema('Public view URL.'),
			folderId: nullableUUIDSchema('Parent folder ID.'),
			updatedAt: stringSchema('ISO timestamp.')
		},
		['id', 'title', 'excerpt', 'documentType', 'preferredImageTargetId', 'myRole', 'publicAccess', 'publicUrl', 'updatedAt']
	);
}

function searchFolderItemSchema() {
	return objectSchema(
		{
			id: stringSchema('Folder UUID.'),
			name: stringSchema('Folder name.'),
			parentId: nullableUUIDSchema('Parent folder ID.'),
			updatedAt: stringSchema('ISO timestamp.')
		},
		['id', 'name', 'updatedAt']
	);
}

function searchMediaItemSchema() {
	return objectSchema(
		{
			id: stringSchema('Media asset UUID.'),
			filename: stringSchema('Filename.'),
			kind: stringSchema('Media kind.'),
			mimeType: stringSchema('MIME type.'),
			documentId: nullableUUIDSchema('Related document ID.'),
			documentTitle: { type: ['string', 'null'], description: 'Related document title.' },
			updatedAt: stringSchema('ISO timestamp.')
		},
		['id', 'filename', 'kind', 'mimeType', 'updatedAt']
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
			createdAt: stringSchema('ISO timestamp.'),
			updatedAt: stringSchema('ISO timestamp.')
		},
		['id', 'type', 'title', 'createdAt', 'updatedAt']
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

function deleteFileResponseSchema() {
	return objectSchema(
		{
			success: { type: 'boolean' },
			message: stringSchema('Deletion result message.')
		},
		['success', 'message']
	);
}

function markdownContentSchema() {
	return objectSchema(
		{
			format: { type: 'string', enum: ['markdown'] },
			content: stringSchema('Markdown content.'),
			updatedAt: stringSchema('ISO timestamp.')
		},
		['format', 'content', 'updatedAt']
	);
}

function updateMarkdownSchema() {
	return objectSchema(
		{
			format: { type: 'string', enum: ['markdown'] },
			content: stringSchema('Full Markdown content.')
		},
		['format', 'content']
	);
}

function patchMarkdownSchema() {
	return objectSchema(
		{
			format: { type: 'string', enum: ['markdown'] },
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
			updatedAt: stringSchema('ISO timestamp.')
		},
		['success', 'updatedAt']
	);
}
