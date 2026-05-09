import { Server } from '@hocuspocus/server';
import * as Y from 'yjs';
import jwt from 'jsonwebtoken';
import axios from 'axios';
import dotenv from 'dotenv';
import type { IncomingMessage, ServerResponse } from 'node:http';
import { WebSocketServer, WebSocket } from 'ws';

dotenv.config();

// 配置
const PORT = parseInt(process.env.PORT || '3001', 10);
const GO_API_URL = process.env.GO_API_URL || 'http://localhost:8080/api/v1';
const COLLABORATION_ENABLED = (() => {
	const raw = (process.env.COLLABORATION_ENABLED || '').trim().toLowerCase();
	if (raw === '') {
		return true;
	}
	return ['1', 'true', 'yes', 'y', 'on'].includes(raw);
})();
const REALTIME_SAVE_DEBOUNCE_MS = Math.max(
	1000,
	parseInt(process.env.REALTIME_SAVE_DEBOUNCE_MS || '10000', 10)
);
const REALTIME_SAVE_MAX_DEBOUNCE_MS = Math.max(
	REALTIME_SAVE_DEBOUNCE_MS,
	parseInt(process.env.REALTIME_SAVE_MAX_DEBOUNCE_MS || String(REALTIME_SAVE_DEBOUNCE_MS), 10)
);
const JWT_SECRET = (() => {
	const secret = process.env.JWT_SECRET_KEY || process.env.JWT_SECRET;
	if (!secret) {
		throw new Error('JWT secret not configured. Please set JWT_SECRET_KEY or JWT_SECRET.');
	}
	return secret;
})();

interface TokenPayload {
	sub: string;
	email?: string;
	[key: string]: unknown;
}

interface UserACL {
	myRole: 'viewer' | 'editor' | 'collaborator' | 'owner';
	canRead: boolean;
	canEdit: boolean;
	canManageMembers: boolean;
}

interface RealtimeContext {
	userId: string;
	token: string;
	documentId: string;
	acl: UserACL;
	socketId?: string;
	// Connection-local fallback snapshot. The authoritative version is tracked
	// per document in-process so multiple collaborators don't race forever with
	// stale per-socket copies.
	yjsVersion: number;
}

// ACL re-validation cache. We can't trust the ACL captured at connect time —
// the document owner may revoke access mid-session. A 30s TTL keeps the cost
// down (one fetch per document per 30s per user) while still bounding how
// long a revoked user can keep editing.
const ACL_CACHE_TTL_MS = 30_000;
const ACL_CACHE_MAX_ENTRIES = (() => {
	const parsed = parseInt(process.env.ACL_CACHE_MAX_ENTRIES || '10000', 10);
	return Number.isFinite(parsed) && parsed > 0 ? parsed : 10000;
})();
const PRESENCE_SESSION_TTL_MS = 45_000;
const PRESENCE_WS_MAX_PAYLOAD_BYTES = parsePositiveInteger(
	process.env.PRESENCE_WS_MAX_PAYLOAD_BYTES,
	4096
);
const PRESENCE_WS_RATE_LIMIT_WINDOW_MS = parsePositiveInteger(
	process.env.PRESENCE_WS_RATE_LIMIT_WINDOW_MS,
	10_000
);
const PRESENCE_WS_MAX_MESSAGES_PER_WINDOW = parsePositiveInteger(
	process.env.PRESENCE_WS_MAX_MESSAGES_PER_WINDOW,
	20
);
const PRESENCE_MAX_SESSIONS_PER_DOCUMENT = Math.max(
	1,
	parseInt(process.env.PRESENCE_MAX_SESSIONS_PER_DOCUMENT || '12', 10)
);

function parsePositiveInteger(value: string | undefined, fallback: number): number {
	const parsed = Number.parseInt(value ?? '', 10);
	return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}
const aclCache = new Map<string, { acl: UserACL; expiresAt: number }>();

function aclCacheKey(documentId: string, userId: string): string {
	return `${userId}:${documentId}`;
}

function setACLCacheEntry(key: string, acl: UserACL, expiresAt: number): void {
	// Refresh insertion order for simple LRU-style eviction. This keeps the
	// cache bounded without scanning every entry on latency-sensitive ACL checks.
	aclCache.delete(key);
	aclCache.set(key, { acl, expiresAt });

	while (aclCache.size > ACL_CACHE_MAX_ENTRIES) {
		const oldestKey = aclCache.keys().next().value;
		if (oldestKey === undefined) {
			break;
		}
		aclCache.delete(oldestKey);
	}
}

async function getUserACLFresh(
	documentId: string,
	userId: string,
	token: string
): Promise<UserACL | null> {
	const now = Date.now();
	const key = aclCacheKey(documentId, userId);
	const cached = aclCache.get(key);
	if (cached) {
		if (cached.expiresAt > now) {
			return cached.acl;
		}
		aclCache.delete(key);
	}
	const acl = await getUserACL(documentId, token);
	if (acl) {
		setACLCacheEntry(key, acl, now + ACL_CACHE_TTL_MS);
	}
	return acl;
}

function invalidateACLCache(documentId: string, userId: string): void {
	aclCache.delete(aclCacheKey(documentId, userId));
}

const collaborationSockets = new Map<string, Set<string>>();
const documentYjsVersions = new Map<string, number>();
const documentCanonicalContentSnapshots = new Map<
	string,
	{ contentJSON: string; updatedAt: number; userId: string }
>();
const documentPresenceSessions = new Map<
	string,
	Map<string, { userId: string; lastSeenAt: number }>
>();
const documentPresence = new Map<string, Map<string, number>>();
const presenceSubscribers = new Map<string, Set<WebSocket>>();
const presenceClientMeta = new WeakMap<
	WebSocket,
	{
		token: string;
		userId: string;
		documentId?: string;
		rateWindowStartedAt: number;
		messageCount: number;
	}
>();
const presenceWebSocketServer = new WebSocketServer({
	noServer: true,
	maxPayload: PRESENCE_WS_MAX_PAYLOAD_BYTES
});

function getPresenceMessageString(raw: Buffer): string {
	return raw.toString('utf8');
}

function isPresenceRateLimited(meta: { rateWindowStartedAt: number; messageCount: number }): boolean {
	const now = Date.now();
	if (now - meta.rateWindowStartedAt > PRESENCE_WS_RATE_LIMIT_WINDOW_MS) {
		meta.rateWindowStartedAt = now;
		meta.messageCount = 0;
	}

	meta.messageCount += 1;
	return meta.messageCount > PRESENCE_WS_MAX_MESSAGES_PER_WINDOW;
}

function isPresenceSubscribeMessage(
	message: unknown
): message is { type: 'subscribe'; documentId: string } {
	return (
		typeof message === 'object' &&
		message !== null &&
		(message as { type?: unknown }).type === 'subscribe' &&
		typeof (message as { documentId?: unknown }).documentId === 'string' &&
		(message as { documentId: string }).documentId.trim().length > 0
	);
}

function logRealtimeSave(event: string, details: Record<string, unknown> = {}): void {
	console.debug('[RealtimeSave]', event, details);
}

function verifyJWT(token: string): TokenPayload | null {
	try {
		return jwt.verify(token, JWT_SECRET) as TokenPayload;
	} catch (error) {
		console.error('JWT verification failed:', error);
		return null;
	}
}

function extractTokenFromUrl(url?: string): string | null {
	if (!url) return null;
	try {
		const parsed = new URL(`ws://localhost${url}`);
		return parsed.searchParams.get('token');
	} catch (error) {
		console.error('Failed to parse token from URL:', error);
		return null;
	}
}

async function getUserACL(documentId: string, token: string): Promise<UserACL | null> {
	try {
		const response = await axios.get(`${GO_API_URL}/workspace/documents/${documentId}/acl`, {
			headers: {
				Authorization: `Bearer ${token}`
			},
			timeout: 5000
		});
		return response.data as UserACL;
	} catch (error) {
		console.error(`Failed to get user ACL for doc ${documentId}:`, error);
		return null;
	}
}

async function loadYjsState(
	documentId: string,
	token: string
): Promise<{ yjsState: string; yjsStateVector: string; yjsVersion: number }> {
	try {
		const response = await axios.get(`${GO_API_URL}/realtime/documents/${documentId}/state`, {
			headers: {
				Authorization: `Bearer ${token}`
			},
			timeout: 5000
		});
		const data = response.data ?? {};
		return {
			yjsState: typeof data.yjsState === 'string' ? data.yjsState : '',
			yjsStateVector: typeof data.yjsStateVector === 'string' ? data.yjsStateVector : '',
			// New rows from a freshly-migrated DB have yjs_version = 1; the
			// "no row exists yet" path is signalled by a 404 (caught below)
			// and we return 0 so the first save can create the row.
			yjsVersion: typeof data.yjsVersion === 'number' ? data.yjsVersion : 0
		};
	} catch (error) {
		// 404 means the row does not exist yet; that's a normal "fresh
		// document" state and the first save should create it. For any other
		// error we still return zeros, but log loudly so it's not silent.
		const status =
			error && typeof error === 'object' && 'response' in error
				? (error as { response?: { status?: number } }).response?.status
				: undefined;
		if (status !== 404) {
			console.error(`Failed to load Yjs state for doc ${documentId}:`, error);
		}
		return { yjsState: '', yjsStateVector: '', yjsVersion: 0 };
	}
}

class YjsSaveConflictError extends Error {
	currentVersion: number;

	constructor(currentVersion: number) {
		super(`yjs version conflict (current ${currentVersion})`);
		this.name = 'YjsSaveConflictError';
		this.currentVersion = currentVersion;
	}
}

function isYjsSaveConflictError(error: unknown): error is YjsSaveConflictError {
	return error instanceof YjsSaveConflictError;
}

async function saveYjsState(
	documentId: string,
	token: string,
	yjsState: string,
	yjsStateVector: string,
	expectedYjsVersion: number,
	contentJson?: unknown
): Promise<number> {
	try {
		const response = await axios.put(
			`${GO_API_URL}/realtime/documents/${documentId}/state`,
			{
				yjsState,
				yjsStateVector,
				expectedYjsVersion,
				...(contentJson === undefined ? {} : { contentJson })
			},
			{
				headers: {
					Authorization: `Bearer ${token}`
				},
				timeout: 5000,
				// Treat 4xx as a value, not a thrown error, so we can branch on
				// 409 without losing the response body.
				validateStatus: (status) => status >= 200 && status < 500
			}
		);

		if (response.status === 409) {
			const current =
				typeof response.data?.currentYjsVersion === 'number'
					? response.data.currentYjsVersion
					: expectedYjsVersion;
			throw new YjsSaveConflictError(current);
		}
		if (response.status >= 400) {
			const body = response.data ? JSON.stringify(response.data) : '';
			throw new Error(`Yjs save failed with status ${response.status}: ${body}`);
		}

		const newVersion =
			typeof response.data?.yjsVersion === 'number'
				? response.data.yjsVersion
				: expectedYjsVersion + 1;
		return newVersion;
	} catch (error) {
		// Re-throw so Hocuspocus marks the document as still-dirty and retries
		// on the next debounce window. Swallowing the error here is what made
		// the previous version silently lose edits.
		if (error instanceof YjsSaveConflictError) {
			throw error;
		}
		if (error instanceof Error) {
			throw error;
		}
		throw new Error(`Yjs save failed: ${String(error)}`);
	}
}

function normalizeDocumentId(rawName?: string): string {
	if (!rawName) return '';
	return rawName.startsWith('doc:') ? rawName.slice(4) : rawName;
}

process.on('unhandledRejection', (reason) => {
	if (isYjsSaveConflictError(reason)) {
		console.warn(
			`[Realtime] Suppressed unhandled Yjs save conflict at process boundary (server ${reason.currentVersion})`
		);
		return;
	}

	console.error('[Realtime] Unhandled promise rejection:', reason);
});

process.on('uncaughtException', (error) => {
	if (isYjsSaveConflictError(error)) {
		console.warn(
			`[Realtime] Suppressed uncaught Yjs save conflict at process boundary (server ${error.currentVersion})`
		);
		return;
	}

	console.error('[Realtime] Uncaught exception:', error);
	process.exit(1);
});

function setCORSHeaders(request: IncomingMessage, response: ServerResponse) {
	const origin = request.headers.origin;
	if (origin) {
		response.setHeader('Access-Control-Allow-Origin', origin);
		response.setHeader('Access-Control-Allow-Credentials', 'true');
	} else {
		response.setHeader('Access-Control-Allow-Origin', '*');
	}
	response.setHeader('Vary', 'Origin');
	response.setHeader('Access-Control-Allow-Headers', 'Authorization, Content-Type, X-Presence-Session-Id');
	response.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
}

function getLoadedDocumentEntry(documentId: string): [string, any] | null {
	for (const entry of (server.hocuspocus as any).documents.entries() as IterableIterator<[string, any]>) {
		if (normalizeDocumentId(entry[0]) === documentId) {
			return entry;
		}
	}
	return null;
}

async function triggerImmediateDocumentPersist(
	documentId: string,
	context: RealtimeContext,
	requestHeaders: IncomingMessage['headers'],
	requestParameters: URLSearchParams = new URLSearchParams()
): Promise<void> {
	const loadedDocumentEntry = getLoadedDocumentEntry(documentId);
	if (!loadedDocumentEntry) {
		const loadedDocumentNames = Array.from(
			(server.hocuspocus as any).documents.keys() as IterableIterator<string>
		);
		throw new Error(
			`document ${documentId} is not loaded in realtime server (loaded: ${loadedDocumentNames.join(', ') || 'none'})`
		);
	}

	const [documentName, document] = loadedDocumentEntry;
	document.broadcastStateless(
		JSON.stringify({
			type: 'document-save-request-accepted',
			documentId,
			acceptedAt: new Date().toISOString()
		})
	);

	await (server.hocuspocus as any).storeDocumentHooks(
		document,
		{
			clientsCount: document.getConnectionsCount(),
			context,
			document,
			documentName,
			instance: server.hocuspocus,
			requestHeaders,
			requestParameters,
			socketId: context.socketId ?? ''
		},
		true
	);
}

function addCollaborationSocket(documentId: string, socketId: string) {
	let members = collaborationSockets.get(documentId);
	if (!members) {
		members = new Set<string>();
		collaborationSockets.set(documentId, members);
	}
	members.add(socketId);
}

function removeCollaborationSocket(documentId: string, socketId: string) {
	const members = collaborationSockets.get(documentId);
	if (!members) {
		return;
	}
	members.delete(socketId);
	if (members.size === 0) {
		collaborationSockets.delete(documentId);
	}
}

function getCollaborationSocketCount(documentId: string): number {
	return collaborationSockets.get(documentId)?.size ?? 0;
}

function addDocumentPresence(documentId: string, userId: string) {
	let users = documentPresence.get(documentId);
	if (!users) {
		users = new Map<string, number>();
		documentPresence.set(documentId, users);
	}

	users.set(userId, (users.get(userId) ?? 0) + 1);
}

function removeDocumentPresence(documentId: string, userId: string) {
	const users = documentPresence.get(documentId);
	if (!users) {
		return;
	}

	const nextCount = (users.get(userId) ?? 0) - 1;
	if (nextCount > 0) {
		users.set(userId, nextCount);
		return;
	}

	users.delete(userId);
	if (users.size === 0) {
		documentPresence.delete(documentId);
	}
}

function getDocumentPresenceCount(documentId: string): number {
	return documentPresence.get(documentId)?.size ?? 0;
}

function broadcastPresence(documentId: string) {
	const subscribers = presenceSubscribers.get(documentId);
	if (!subscribers || subscribers.size === 0) {
		return;
	}

	const payload = JSON.stringify({
		type: 'presence',
		documentId,
		connectedCount: getDocumentPresenceCount(documentId)
	});

	for (const subscriber of subscribers) {
		if (subscriber.readyState === WebSocket.OPEN) {
			subscriber.send(payload);
		}
	}
}

function removePresenceSubscriber(socket: WebSocket) {
	const meta = presenceClientMeta.get(socket);
	if (!meta?.documentId) {
		return;
	}

	removeDocumentPresence(meta.documentId, meta.userId);
	broadcastPresence(meta.documentId);

	const subscribers = presenceSubscribers.get(meta.documentId);
	if (!subscribers) {
		return;
	}

	subscribers.delete(socket);
	if (subscribers.size === 0) {
		presenceSubscribers.delete(meta.documentId);
	}
}

function getTrackedYjsVersion(documentId: string, fallback = 0): number {
	return documentYjsVersions.get(documentId) ?? fallback;
}

function setTrackedYjsVersion(documentId: string, version: number): void {
	documentYjsVersions.set(documentId, version);
}

function clearTrackedYjsVersion(documentId: string): void {
	documentYjsVersions.delete(documentId);
}

function setTrackedCanonicalContentSnapshot(
	documentId: string,
	contentJSON: string,
	userId: string
): void {
	documentCanonicalContentSnapshots.set(documentId, {
		contentJSON,
		updatedAt: Date.now(),
		userId
	});
}

function getTrackedCanonicalContentSnapshot(documentId: string): string | null {
	return documentCanonicalContentSnapshots.get(documentId)?.contentJSON ?? null;
}

function clearTrackedCanonicalContentSnapshot(documentId: string): void {
	documentCanonicalContentSnapshots.delete(documentId);
}

function pruneExpiredPresenceSessions(documentId: string, now = Date.now()): void {
	const sessions = documentPresenceSessions.get(documentId);
	if (!sessions) {
		return;
	}

	for (const [sessionId, session] of sessions) {
		if (now-session.lastSeenAt >= PRESENCE_SESSION_TTL_MS) {
			sessions.delete(sessionId);
		}
	}

	if (sessions.size === 0) {
		documentPresenceSessions.delete(documentId);
	}
}

function upsertPresenceSession(
	documentId: string,
	sessionId: string,
	userId: string,
	now = Date.now()
): { connectedCount: number; accepted: boolean } {
	let sessions = documentPresenceSessions.get(documentId);
	if (!sessions) {
		sessions = new Map<string, { userId: string; lastSeenAt: number }>();
		documentPresenceSessions.set(documentId, sessions);
	}

	pruneExpiredPresenceSessions(documentId, now);
	if (!sessions.has(sessionId) && sessions.size >= PRESENCE_MAX_SESSIONS_PER_DOCUMENT) {
		return {
			connectedCount: sessions.size,
			accepted: false
		};
	}

	sessions.set(sessionId, { userId, lastSeenAt: now });
	return {
		connectedCount: sessions.size,
		accepted: true
	};
}

function removePresenceSession(documentId: string, sessionId: string): number {
	const sessions = documentPresenceSessions.get(documentId);
	if (!sessions) {
		return 0;
	}

	sessions.delete(sessionId);
	if (sessions.size === 0) {
		documentPresenceSessions.delete(documentId);
		return 0;
	}

	return sessions.size;
}

function getActivePresenceSessionCount(documentId: string, now = Date.now()): number {
	pruneExpiredPresenceSessions(documentId, now);
	return documentPresenceSessions.get(documentId)?.size ?? 0;
}

const server = new Server({
	port: PORT,
	address: '0.0.0.0',
	timeout: 30000,
	debounce: REALTIME_SAVE_DEBOUNCE_MS,
	maxDebounce: REALTIME_SAVE_MAX_DEBOUNCE_MS,
	async onUpgrade(data: any) {
		const request = data.request as IncomingMessage;
		const requestURL = new URL(request.url ?? '/', `http://${request.headers.host ?? 'localhost'}`);
		if (requestURL.pathname !== '/api/v1/realtime/presence/ws') {
			return;
		}

		presenceWebSocketServer.handleUpgrade(data.request, data.socket, data.head, (ws: WebSocket) => {
			presenceWebSocketServer.emit('connection', ws, data.request);
		});

		throw null;
	},

	// 认证 - 从 WebSocket URL 或 token 字段提取 JWT
	async onAuthenticate(data: any) {
		if (!COLLABORATION_ENABLED) {
			const error = new Error('collaboration-disabled') as Error & { reason?: string };
			error.reason = 'collaboration-disabled';
			throw error;
		}

		const token = (data?.token as string | undefined) || extractTokenFromUrl(data?.request?.url);

		if (!token) {
			throw new Error('No authentication token provided');
		}

		const payload = verifyJWT(token);
		if (!payload?.sub) {
			throw new Error('Invalid or expired token');
		}

		const documentId = normalizeDocumentId(data?.documentName);
		if (!documentId) {
			throw new Error('Missing document ID');
		}

		// Force a fresh fetch on connect so the cache cannot replay a stale
		// "canEdit" decision from a previous session.
		invalidateACLCache(documentId, payload.sub);
		const acl = await getUserACLFresh(documentId, payload.sub, token);
		if (!acl?.canRead) {
			const error = new Error('No read permission for this document') as Error & {
				reason?: string;
			};
			error.reason = 'permission-denied';
			throw error;
		}

		data.connectionConfig.readOnly = !acl.canEdit;

		return {
			userId: payload.sub,
			token,
			documentId,
			acl,
			socketId: data?.socketId,
			yjsVersion: 0
		} satisfies RealtimeContext;
	},

	// 连接建立前，认证上下文尚未写入；这里只保留轻量日志。
	async onConnect(data: any) {
		const documentId = normalizeDocumentId(data?.documentName);
		console.log(`[DOC:${documentId}] Incoming connection ${data?.socketId ?? 'unknown-socket'}`);
	},

	async connected(data: any) {
		const context = data.context as RealtimeContext | undefined;
		const documentId = context?.documentId || normalizeDocumentId(data?.documentName);
		const socketId = context?.socketId || data?.socketId;

		if (documentId && socketId) {
			addCollaborationSocket(documentId, socketId);
		}

		if (documentId && context?.userId) {
			console.log(
				`[DOC:${documentId}] User ${context.userId} connected with role: ${context.acl.myRole} (${getCollaborationSocketCount(documentId)} collab sockets)`
			);
		}
	},

	// 加载文档 - 从 Go API 拉 Yjs state
	async onLoadDocument(data: any) {
		const document = data.document as Y.Doc;
		const context = data.context as RealtimeContext | undefined;
		const documentId = context?.documentId || normalizeDocumentId(data?.documentName);
		const token = context?.token;

		if (!documentId || !token) {
			console.warn('Missing context for loading document');
			return;
		}

		const state = await loadYjsState(documentId, token);
		if (state.yjsState) {
			Y.applyUpdate(document, Buffer.from(state.yjsState, 'base64'));
			console.log(
				`[DOC:${documentId}] Loaded Yjs state from database (version ${state.yjsVersion})`
			);
		} else {
			console.log(`[DOC:${documentId}] No existing Yjs state found, starting fresh`);
		}
		// Stash the version on the connection context so the next save can
		// echo it back as the optimistic-concurrency token. Also keep the
		// process-local per-document tracker in sync so all collaborators on
		// this realtime node share the freshest known version.
		if (context) {
			context.yjsVersion = state.yjsVersion;
		}
		setTrackedYjsVersion(documentId, state.yjsVersion);
	},

	// 保存文档 - 节流保存到 Go API
	async onStoreDocument(data: any) {
		const document = data.document as Y.Doc;
		const context = data.context as RealtimeContext | undefined;
		const documentId = context?.documentId || normalizeDocumentId(data?.documentName);
		const userId = context?.userId;
		const token = context?.token;

		if (!documentId || !token || !userId || !context) {
			console.warn('Missing context for storing document');
			return;
		}

		// Re-validate ACL on every save. The captured-at-connect ACL is stale
		// the moment the document owner removes the editor. A 30s TTL keeps
		// the cost down without leaving a wide window for revoked users.
		const freshACL = await getUserACLFresh(documentId, userId, token);
		if (!freshACL?.canEdit) {
			console.warn(
				`[DOC:${documentId}] User ${userId} lost edit permission; refusing to persist (had role ${context.acl.myRole})`
			);
			// Update the cached context so subsequent reads also see the
			// downgraded permission, and bubble up so Hocuspocus knows the
			// store didn't succeed.
			if (freshACL) {
				context.acl = freshACL;
			}
			throw new Error('edit permission revoked');
		}
		// Keep the context in sync with the latest ACL view.
		context.acl = freshACL;

		const yjsState = Buffer.from(Y.encodeStateAsUpdate(document)).toString('base64');
		const yjsStateVector = Buffer.from(Y.encodeStateVector(document)).toString('base64');
		const trackedCanonicalContent = getTrackedCanonicalContentSnapshot(documentId);

		const expectedYjsVersion = getTrackedYjsVersion(documentId, context.yjsVersion);

		try {
			let contentJsonPayload: unknown;
			if (trackedCanonicalContent) {
				try {
					contentJsonPayload = JSON.parse(trackedCanonicalContent);
				} catch (parseError) {
					console.warn(
						`[DOC:${documentId}] Ignoring invalid tracked canonical content snapshot:`,
						parseError
					);
				}
			}
			logRealtimeSave('store-start', {
				documentId,
				userId,
				expectedYjsVersion,
				hasCanonicalSnapshot: contentJsonPayload !== undefined
			});
			data.document.broadcastStateless(
				JSON.stringify({
					type: 'document-persisting',
					documentId,
					startedAt: new Date().toISOString()
				})
			);
			const newVersion = await saveYjsState(
				documentId,
				token,
				yjsState,
				yjsStateVector,
				expectedYjsVersion,
				contentJsonPayload
			);
			context.yjsVersion = newVersion;
			setTrackedYjsVersion(documentId, newVersion);
			logRealtimeSave('store-success', {
				documentId,
				userId,
				newVersion,
				hasCanonicalSnapshot: contentJsonPayload !== undefined
			});
			data.document.broadcastStateless(
				JSON.stringify({
					type: 'document-persisted',
					documentId,
					yjsVersion: newVersion,
					savedAt: new Date().toISOString()
				})
			);
		} catch (error) {
			if (error instanceof YjsSaveConflictError) {
				// Someone else (or a racing save) bumped the version. Re-load
				// the latest state, merge it into our in-memory doc via
				// Yjs CRDT semantics, and let Hocuspocus retry on the next
				// debounce. Throwing keeps the doc marked dirty so the retry
				// actually happens.
				console.warn(
					`[DOC:${documentId}] Yjs save conflict (had ${context.yjsVersion}, server ${error.currentVersion}); reloading`
				);
				logRealtimeSave('store-conflict', {
					documentId,
					userId,
					hadVersion: context.yjsVersion,
					serverVersion: error.currentVersion
				});
				const fresh = await loadYjsState(documentId, token);
				if (fresh.yjsState) {
					try {
						Y.applyUpdate(document, Buffer.from(fresh.yjsState, 'base64'));
					} catch (mergeErr) {
						console.error(`[DOC:${documentId}] Failed to merge fresh state:`, mergeErr);
					}
				}
				context.yjsVersion = fresh.yjsVersion;
				setTrackedYjsVersion(documentId, fresh.yjsVersion);
				// The newer state is already durable on the server. Merging it back
				// into the in-memory doc lets the normal update/debounce flow
				// schedule the next save without crashing the process.
				return;
			}
			throw error;
		}
	},

	async onStateless(data: any) {
		const context = data.context as RealtimeContext | undefined;
		const documentId = context?.documentId || normalizeDocumentId(data?.documentName);
		if (!documentId || !context?.userId) {
			return;
		}

		let message:
			| {
					type?: string;
					documentId?: string;
					contentJson?: unknown;
			  }
			| null = null;
		try {
			message = JSON.parse(data.payload) as {
				type?: string;
				documentId?: string;
				contentJson?: unknown;
			};
		} catch {
			return;
		}

		if (message?.documentId !== documentId) {
			return;
		}

		if (message.type === 'document-content-snapshot') {
			if (message.contentJson === undefined) {
				return;
			}

			try {
				const serialized = JSON.stringify(message.contentJson);
				setTrackedCanonicalContentSnapshot(documentId, serialized, context.userId);
				logRealtimeSave('canonical-snapshot-updated', {
					documentId,
					userId: context.userId,
					size: serialized.length
				});
			} catch (error) {
				console.warn(`[DOC:${documentId}] Failed to serialize canonical content snapshot:`, error);
			}
			return;
		}

		if (message.type === 'manual-save-request') {
			logRealtimeSave('manual-save-request', {
				documentId,
				userId: context.userId
			});
			await triggerImmediateDocumentPersist(
				documentId,
				context,
				data.connection?.request?.headers ?? {},
				new URLSearchParams()
			);
		}
	},

	async onDisconnect(data: any) {
		const context = data.context as RealtimeContext | undefined;
		const documentId = context?.documentId || normalizeDocumentId(data?.documentName);
		const userId = context?.userId;
		const socketId = context?.socketId || data?.socketId;

		if (documentId && socketId) {
			removeCollaborationSocket(documentId, socketId);
			if (getCollaborationSocketCount(documentId) === 0) {
				clearTrackedYjsVersion(documentId);
				clearTrackedCanonicalContentSnapshot(documentId);
			}
		}

		if (documentId && userId) {
			console.log(`[DOC:${documentId}] User ${userId} disconnected (${getCollaborationSocketCount(documentId)} collab sockets)`);
		}
	},

	async onRequest(data: any) {
		const request = data.request as IncomingMessage;
		const response = data.response as ServerResponse;
		const requestURL = new URL(request.url ?? '/', `http://${request.headers.host ?? 'localhost'}`);

		setCORSHeaders(request, response);

		if (request.method === 'OPTIONS') {
			response.writeHead(204);
			response.end();
			throw null;
		}

		const isPresenceRequest = requestURL.pathname === '/api/v1/realtime/presence';
		const isPersistNowRequest = requestURL.pathname === '/api/v1/realtime/persist-now';
		if (!isPresenceRequest && !isPersistNowRequest) {
			return;
		}

		if (!COLLABORATION_ENABLED) {
			response.writeHead(404, { 'Content-Type': 'application/json' });
			response.end(JSON.stringify({ error: 'Collaboration is disabled' }));
			throw null;
		}

		const documentId = normalizeDocumentId(requestURL.searchParams.get('documentId') ?? '');
		if (!documentId) {
			response.writeHead(400, { 'Content-Type': 'application/json' });
			response.end(JSON.stringify({ error: 'documentId is required' }));
			throw null;
		}

		const authHeader = request.headers.authorization ?? '';
		const token = authHeader.startsWith('Bearer ') ? authHeader.slice(7).trim() : '';
		const payload = token ? verifyJWT(token) : null;
		if (!payload?.sub) {
			response.writeHead(401, { 'Content-Type': 'application/json' });
			response.end(JSON.stringify({ error: 'Unauthorized' }));
			throw null;
		}

		const acl = await getUserACL(documentId, token);
		if (!acl?.canRead) {
			response.writeHead(403, { 'Content-Type': 'application/json' });
			response.end(JSON.stringify({ error: 'Forbidden' }));
			throw null;
		}

		if (isPersistNowRequest) {
			if (request.method !== 'POST') {
				response.writeHead(405, { 'Content-Type': 'application/json' });
				response.end(JSON.stringify({ error: 'Method not allowed' }));
				throw null;
			}
			if (!acl.canEdit) {
				response.writeHead(403, { 'Content-Type': 'application/json' });
				response.end(JSON.stringify({ error: 'Forbidden' }));
				throw null;
			}

			const context: RealtimeContext = {
				userId: payload.sub,
				token,
				documentId,
				acl,
				yjsVersion: getTrackedYjsVersion(documentId)
			};

			try {
				logRealtimeSave('manual-save-http-request', {
					documentId,
					userId: payload.sub
				});
				await triggerImmediateDocumentPersist(documentId, context, request.headers, requestURL.searchParams);
				response.writeHead(200, { 'Content-Type': 'application/json' });
				response.end(JSON.stringify({ ok: true, documentId }));
			} catch (error) {
				console.error(`[DOC:${documentId}] Failed to force immediate persist:`, error);
				response.writeHead(409, { 'Content-Type': 'application/json' });
				response.end(
					JSON.stringify({
						error: error instanceof Error ? error.message : 'Failed to persist document immediately',
						details: error instanceof Error ? error.stack ?? null : null,
						documentId
					})
				);
			}
			throw null;
		}

		const sessionIdHeader = request.headers['x-presence-session-id'];
		const sessionId =
			typeof sessionIdHeader === 'string'
				? sessionIdHeader.trim()
				: Array.isArray(sessionIdHeader)
					? (sessionIdHeader[0] ?? '').trim()
					: '';

		if (request.method === 'PUT' && sessionId) {
			const { connectedCount, accepted } = upsertPresenceSession(documentId, sessionId, payload.sub);
			if (!accepted) {
				response.writeHead(429, { 'Content-Type': 'application/json' });
				response.end(
					JSON.stringify({
						error: 'Too many active sessions for document',
						documentId,
						connectedCount,
						maxSessions: PRESENCE_MAX_SESSIONS_PER_DOCUMENT
					})
				);
				throw null;
			}
			response.writeHead(200, { 'Content-Type': 'application/json' });
			response.end(
				JSON.stringify({
					documentId,
					connectedCount,
					hasCollaboration: getCollaborationSocketCount(documentId) > 0
				})
			);
			throw null;
		}

		if (request.method === 'DELETE' && sessionId) {
			const connectedCount = removePresenceSession(documentId, sessionId);
			response.writeHead(200, { 'Content-Type': 'application/json' });
			response.end(
				JSON.stringify({
					documentId,
					connectedCount,
					hasCollaboration: getCollaborationSocketCount(documentId) > 0
				})
			);
			throw null;
		}

		response.writeHead(200, { 'Content-Type': 'application/json' });
		response.end(
			JSON.stringify({
				documentId,
				connectedCount: getActivePresenceSessionCount(documentId),
				hasCollaboration: getCollaborationSocketCount(documentId) > 0
			})
		);
		throw null;
	}
});

presenceWebSocketServer.on('connection', (socket: WebSocket, request: IncomingMessage) => {
	const requestURL = new URL(request.url ?? '/', `http://${request.headers.host ?? 'localhost'}`);
	const token = requestURL.searchParams.get('token')?.trim() ?? '';
	const payload = token ? verifyJWT(token) : null;

	if (!payload?.sub) {
		socket.close(4401, 'unauthorized');
		return;
	}

	presenceClientMeta.set(socket, {
		token,
		userId: payload.sub,
		rateWindowStartedAt: Date.now(),
		messageCount: 0
	});

	socket.on('error', (error) => {
		console.warn('[Presence] WebSocket error:', error instanceof Error ? error.message : error);
	});

	socket.on('message', async (raw: Buffer) => {
		try {
			const meta = presenceClientMeta.get(socket);
			if (!meta?.token) {
				socket.close(4401, 'unauthorized');
				return;
			}

			if (isPresenceRateLimited(meta)) {
				socket.close(4408, 'rate limit exceeded');
				return;
			}

			if (raw.byteLength > PRESENCE_WS_MAX_PAYLOAD_BYTES) {
				socket.close(1009, 'message too large');
				return;
			}

			const message = JSON.parse(getPresenceMessageString(raw)) as unknown;
			if (!isPresenceSubscribeMessage(message)) {
				return;
			}

			const documentId = normalizeDocumentId(message.documentId);
			const acl = await getUserACL(documentId, meta.token);
			if (!acl?.canRead) {
				socket.close(4403, 'forbidden');
				return;
			}

			removePresenceSubscriber(socket);

			let subscribers = presenceSubscribers.get(documentId);
			if (!subscribers) {
				subscribers = new Set<WebSocket>();
				presenceSubscribers.set(documentId, subscribers);
			}
			subscribers.add(socket);
			presenceClientMeta.set(socket, { ...meta, documentId });
			addDocumentPresence(documentId, meta.userId);
			broadcastPresence(documentId);
		} catch (error) {
			console.error('[Presence] Failed to handle message:', error);
		}
	});

	socket.on('close', () => {
		removePresenceSubscriber(socket);
	});
});

server
	.listen()
	.then(() => {
		console.log(`🚀 Realtime server listening on port ${PORT}`);
		console.log(`📡 WebSocket endpoint: ws://0.0.0.0:${PORT}`);
		console.log(`🔗 Backend API: ${GO_API_URL}`);
	})
	.catch((error: unknown) => {
		console.error('Failed to start server:', error);
		process.exit(1);
	});
