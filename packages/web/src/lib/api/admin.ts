import { apiFetch } from '$lib/api';

export type AdminOverview = {
	userCount: number;
	adminCount: number;
	globalDocumentQuota: number | null;
	globalUnlimited: boolean;
};

export type AdminUserListItem = {
	id: string;
	email: string | null;
	emailVerified: boolean;
	displayName: string | null;
	avatarUrl: string | null;
	adminAccess: {
		hasAccess: boolean;
		role: string | null;
	};
	documentQuotaMode: 'inherit' | 'custom' | 'unlimited' | string;
	documentQuota: number | null;
	effectiveDocumentQuota: number | null;
	unlimited: boolean;
	activeDocumentCount: number;
	trashedDocumentCount: number;
	usedDocumentCount: number;
};

export type AdminUserListResponse = {
	items: AdminUserListItem[];
	hasMore: boolean;
	nextOffset: number;
	total: number;
	globalDocumentQuota: number | null;
	globalUnlimited: boolean;
};

export type UpdateAdminUserDocumentQuotaRequest = {
	documentQuotaMode: 'inherit' | 'custom' | 'unlimited';
	documentQuota: number | null;
};

export type UpdateAdminUserEmailRequest = {
	email: string;
};

export type AdminUserSession = {
	id: string;
	deviceLabel: string;
	userAgent: string;
	lastSeenAt: string;
	expiresAt: string;
	createdAt: string;
};

export type AdminUserSessionListResponse = {
	items: AdminUserSession[];
	hasMore: boolean;
	nextOffset: number;
	total: number;
};

export type AdminUserMediaListResponse = {
	items: Array<{
		id: string;
		kind: string;
		filename: string;
		mimeType: string;
		fileSize: number;
		visibility: string;
		status: string;
		referenceCount: number;
		deletable: boolean;
		createdAt: string;
		updatedAt: string;
		thumbnailUrl?: string;
	}>;
	hasMore: boolean;
	total: number;
};

function buildErrorMessage(error: unknown, fallback: string): string {
	if (error && typeof error === 'object') {
		const maybeMessage =
			(error as { error?: string; message?: string }).error ||
			(error as { error?: string; message?: string }).message;
		if (typeof maybeMessage === 'string' && maybeMessage.trim() !== '') {
			return maybeMessage;
		}
	}
	return fallback;
}

export async function getAdminOverview(): Promise<AdminOverview> {
	const response = await apiFetch('/api/v1/admin/overview');
	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(buildErrorMessage(error, 'Failed to load admin overview'));
	}
	return response.json();
}

export async function listAdminUsers(params: {
	limit?: number;
	offset?: number;
	q?: string;
}): Promise<AdminUserListResponse> {
	const search = new URLSearchParams();
	if (typeof params.limit === 'number') search.set('limit', String(params.limit));
	if (typeof params.offset === 'number') search.set('offset', String(params.offset));
	if (params.q && params.q.trim() !== '') search.set('q', params.q.trim());

	const suffix = search.toString();
	const response = await apiFetch(`/api/v1/admin/users${suffix ? `?${suffix}` : ''}`);
	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(buildErrorMessage(error, 'Failed to load admin users'));
	}
	return response.json();
}

export async function getAdminUser(userID: string): Promise<AdminUserListItem> {
	const response = await apiFetch(`/api/v1/admin/users/${userID}`);
	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(buildErrorMessage(error, 'Failed to load admin user detail'));
	}
	return response.json();
}

export async function listAdminUserSessions(params: {
	userID: string;
	limit?: number;
	offset?: number;
}): Promise<AdminUserSessionListResponse> {
	const search = new URLSearchParams();
	if (typeof params.limit === 'number') search.set('limit', String(params.limit));
	if (typeof params.offset === 'number') search.set('offset', String(params.offset));

	const suffix = search.toString();
	const response = await apiFetch(
		`/api/v1/admin/users/${params.userID}/sessions${suffix ? `?${suffix}` : ''}`
	);
	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(buildErrorMessage(error, 'Failed to load admin user sessions'));
	}
	return response.json();
}

export async function revokeAdminUserSession(userID: string, sessionID: string): Promise<void> {
	const response = await apiFetch(`/api/v1/admin/users/${userID}/sessions/${sessionID}`, {
		method: 'DELETE'
	});
	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(buildErrorMessage(error, 'Failed to revoke admin user session'));
	}
}

export async function listAdminUserMedia(params: {
	userID: string;
	kind?: 'all' | 'image' | 'video' | 'file';
	status?: 'all' | 'ready' | 'pending_delete' | 'deleted' | 'failed';
	q?: string;
	limit?: number;
	offset?: number;
}): Promise<AdminUserMediaListResponse> {
	const search = new URLSearchParams();
	if (params.kind) search.set('kind', params.kind);
	if (params.status) search.set('status', params.status);
	if (params.q && params.q.trim() !== '') search.set('q', params.q.trim());
	if (typeof params.limit === 'number') search.set('limit', String(params.limit));
	if (typeof params.offset === 'number') search.set('offset', String(params.offset));

	const suffix = search.toString();
	const response = await apiFetch(
		`/api/v1/admin/users/${params.userID}/media${suffix ? `?${suffix}` : ''}`
	);
	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(buildErrorMessage(error, 'Failed to load admin user media'));
	}
	return response.json();
}

export async function updateAdminUserDocumentQuota(
	userID: string,
	request: UpdateAdminUserDocumentQuotaRequest
): Promise<AdminUserListItem> {
	const response = await apiFetch(`/api/v1/admin/users/${userID}/document-quota`, {
		method: 'PUT',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(request)
	});

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(buildErrorMessage(error, 'Failed to update admin user quota'));
	}

	return response.json();
}

export async function updateAdminUserEmail(
	userID: string,
	request: UpdateAdminUserEmailRequest
): Promise<AdminUserListItem> {
	const response = await apiFetch(`/api/v1/admin/users/${userID}/email`, {
		method: 'PUT',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(request)
	});

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(buildErrorMessage(error, 'Failed to update admin user email'));
	}

	return response.json();
}

export async function verifyAdminUserEmail(userID: string): Promise<AdminUserListItem> {
	const response = await apiFetch(`/api/v1/admin/users/${userID}/verify-email`, {
		method: 'POST'
	});

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(buildErrorMessage(error, 'Failed to verify admin user email'));
	}

	return response.json();
}

export async function purgeAdminUserMedia(userID: string): Promise<void> {
	const response = await apiFetch(`/api/v1/admin/users/${userID}/purge-media`, {
		method: 'POST'
	});

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(buildErrorMessage(error, 'Failed to purge user media'));
	}
}

export async function purgeAdminUserDocuments(userID: string): Promise<void> {
	const response = await apiFetch(`/api/v1/admin/users/${userID}/purge-documents`, {
		method: 'POST'
	});

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(buildErrorMessage(error, 'Failed to purge user documents'));
	}
}

export async function unregisterAdminUser(userID: string): Promise<void> {
	const response = await apiFetch(`/api/v1/admin/users/${userID}`, {
		method: 'DELETE'
	});

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(buildErrorMessage(error, 'Failed to unregister user'));
	}
}
