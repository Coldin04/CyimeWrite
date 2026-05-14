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

function buildErrorMessage(error: unknown, fallback: string): string {
	if (error && typeof error === 'object') {
		const maybeMessage = (error as { error?: string; message?: string }).error || (error as { error?: string; message?: string }).message;
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
