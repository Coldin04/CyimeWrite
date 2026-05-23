import { apiFetch } from '$lib/api';

export type UserProfile = {
	id: string;
	displayName: string | null;
	email: string | null;
	avatarUrl: string | null;
	adminAccess: {
		hasAccess: boolean;
		role: string | null;
	};
};

export type UserOverview = {
	activeDocumentCount: number;
	trashedDocumentCount: number;
	documentLimit: number | null;
	unlimited: boolean;
};

export type ImageBedConfig = {
	id: string;
	name: string;
	providerType: 'see' | 'lsky' | string;
	baseUrl: string;
	apiToken: string;
	hasApiToken: boolean;
	isEnabled: boolean;
	storageId?: number;
	strategyId?: string;
	fieldValues?: Record<string, string>;
};

export type UpsertImageBedConfigRequest = Omit<ImageBedConfig, 'id' | 'hasApiToken'>;

export type ImageBedProviderField = {
	key: string;
	type: 'text' | 'password' | 'url' | 'number' | string;
	label: string;
	labelKey?: string;
	placeholder?: string;
	placeholderKey?: string;
	helpText?: string;
	helpTextKey?: string;
	inputMode?: 'none' | 'text' | 'tel' | 'url' | 'email' | 'numeric' | 'decimal' | 'search' | string;
	required: boolean;
};

export type ImageBedProvider = {
	providerType: string;
	displayName: string;
	description: string;
	fields: ImageBedProviderField[];
};

export type ApiTokenScope =
	| 'workspace:read'
	| 'workspace:write'
	| 'document:read'
	| 'document:write'
	| 'file:move'
	| 'file:copy'
	| 'file:delete'
	| string;

export type ApiToken = {
	id: string;
	name: string;
	tokenPrefix: string;
	scopes: ApiTokenScope[];
	lastUsedAt?: string | null;
	lastUsedIp?: string;
	expiresAt?: string | null;
	revokedAt?: string | null;
	createdAt: string;
	updatedAt: string;
};

export type CreatedApiToken = ApiToken & {
	token: string;
};

async function parseUserResponse(response: Response): Promise<UserProfile> {
	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error.error || error.message || 'Failed to update user profile');
	}

	return response.json();
}

export async function updateDisplayName(displayName: string): Promise<UserProfile> {
	const response = await apiFetch('/api/v1/user/profile', {
		method: 'PUT',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify({ displayName })
	});

	return parseUserResponse(response);
}

export async function uploadAvatar(file: File): Promise<UserProfile> {
	const formData = new FormData();
	formData.set('file', file);

	const response = await apiFetch('/api/v1/user/avatar', {
		method: 'POST',
		body: formData
	});

	return parseUserResponse(response);
}

export async function setGitHubAvatar(username: string): Promise<UserProfile> {
	const response = await apiFetch('/api/v1/user/avatar/github', {
		method: 'PUT',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify({ username })
	});

	return parseUserResponse(response);
}

export async function getUserOverview(): Promise<UserOverview> {
	const response = await apiFetch('/api/v1/user/overview');

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error.error || error.message || 'Failed to load user overview');
	}

	return response.json();
}

export async function getImageBedConfigs(): Promise<ImageBedConfig[]> {
	const response = await apiFetch('/api/v1/user/image-beds');

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error.error || error.message || 'Failed to load image bed configs');
	}

	const payload = await response.json();
	return payload.items ?? [];
}

export async function getImageBedProviders(): Promise<ImageBedProvider[]> {
	const response = await apiFetch('/api/v1/user/image-beds/providers');

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error.error || error.message || 'Failed to load image bed providers');
	}

	const payload = await response.json();
	return payload.items ?? [];
}

export async function createImageBedConfig(request: UpsertImageBedConfigRequest): Promise<ImageBedConfig> {
	const response = await apiFetch('/api/v1/user/image-beds', {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(request)
	});

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error.error || error.message || 'Failed to create image bed config');
	}

	return response.json();
}

export async function updateImageBedConfig(
	id: string,
	request: UpsertImageBedConfigRequest
): Promise<ImageBedConfig> {
	const response = await apiFetch(`/api/v1/user/image-beds/${id}`, {
		method: 'PUT',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(request)
	});

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error.error || error.message || 'Failed to update image bed config');
	}

	return response.json();
}

export async function deleteImageBedConfig(id: string): Promise<void> {
	const response = await apiFetch(`/api/v1/user/image-beds/${id}`, {
		method: 'DELETE'
	});

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error.error || error.message || 'Failed to delete image bed config');
	}
}

export async function getApiTokens(): Promise<ApiToken[]> {
	const response = await apiFetch('/api/v1/user/api-tokens');

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error.error || error.message || 'Failed to load API tokens');
	}

	const payload = await response.json();
	return payload.items ?? [];
}

export async function createApiToken(request: {
	name: string;
	scopes: ApiTokenScope[];
	expiresAt?: string | null;
}): Promise<CreatedApiToken> {
	const response = await apiFetch('/api/v1/user/api-tokens', {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(request)
	});

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error.error || error.message || 'Failed to create API token');
	}

	return response.json();
}

export async function updateApiToken(
	id: string,
	request: {
		name: string;
		scopes: ApiTokenScope[];
	}
): Promise<ApiToken> {
	const response = await apiFetch(`/api/v1/user/api-tokens/${id}`, {
		method: 'PUT',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(request)
	});

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error.error || error.message || 'Failed to update API token');
	}

	return response.json();
}

export async function revokeApiToken(id: string): Promise<void> {
	const response = await apiFetch(`/api/v1/user/api-tokens/${id}`, {
		method: 'DELETE'
	});

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error.error || error.message || 'Failed to revoke API token');
	}
}

export async function deleteRevokedApiToken(id: string): Promise<void> {
	const response = await apiFetch(`/api/v1/user/api-tokens/${id}/record`, {
		method: 'DELETE'
	});

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error.error || error.message || 'Failed to delete API token record');
	}
}
