import { apiFetch } from '$lib/api';

export type AuthSession = {
	id: string;
	deviceLabel: string;
	userAgent: string;
	lastSeenAt: string;
	expiresAt: string;
	createdAt: string;
	current: boolean;
};

type AuthSessionListResponse = {
	items: AuthSession[];
};

export type SkillOAuthRequest = {
	id: string;
	clientId: string;
	redirectUri: string;
	scopes: string[];
	expiresAt: string;
	tokenExpiresInSeconds: number;
};

type SkillOAuthDecisionResponse = {
	redirectUrl: string;
};

async function parseJSONOrThrow<T>(response: Response, fallbackMessage: string): Promise<T> {
	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error.error || error.message || fallbackMessage);
	}
	return response.json() as Promise<T>;
}

export async function listAuthSessions(): Promise<AuthSession[]> {
	const response = await apiFetch('/api/v1/auth/sessions');
	const payload = await parseJSONOrThrow<AuthSessionListResponse>(response, 'Failed to load sessions');
	return payload.items;
}

export async function revokeAuthSession(sessionId: string): Promise<void> {
	const response = await apiFetch(`/api/v1/auth/sessions/${sessionId}`, {
		method: 'DELETE'
	});
	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error.error || error.message || 'Failed to revoke session');
	}
}

export async function revokeOtherAuthSessions(): Promise<number> {
	const response = await apiFetch('/api/v1/auth/sessions/others', {
		method: 'DELETE'
	});
	const payload = await parseJSONOrThrow<{ revokedCount: number }>(
		response,
		'Failed to revoke other sessions'
	);
	return payload.revokedCount;
}

export async function getSkillOAuthRequest(requestId: string): Promise<SkillOAuthRequest> {
	const response = await apiFetch(`/api/v1/auth/skill/oauth/requests/${requestId}`);
	return parseJSONOrThrow<SkillOAuthRequest>(
		response,
		'Failed to load authorization request'
	);
}

export async function approveSkillOAuthRequest(requestId: string): Promise<string> {
	const response = await apiFetch(`/api/v1/auth/skill/oauth/requests/${requestId}/approve`, {
		method: 'POST'
	});
	const payload = await parseJSONOrThrow<SkillOAuthDecisionResponse>(
		response,
		'Failed to approve authorization request'
	);
	return payload.redirectUrl;
}

export async function denySkillOAuthRequest(requestId: string): Promise<string> {
	const response = await apiFetch(`/api/v1/auth/skill/oauth/requests/${requestId}/deny`, {
		method: 'POST'
	});
	const payload = await parseJSONOrThrow<SkillOAuthDecisionResponse>(
		response,
		'Failed to deny authorization request'
	);
	return payload.redirectUrl;
}
