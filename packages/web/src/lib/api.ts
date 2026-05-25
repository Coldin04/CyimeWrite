import { auth } from '$lib/stores/auth';
import { resolveApiUrl } from '$lib/config/api';

/**
 * A custom fetch wrapper that automatically adds the Authorization header
 * and handles token refreshing and request retrying on 401 errors.
 * @param url The URL to fetch.
 * @param options The options for the fetch request.
 * @returns A Promise that resolves to the Response object.
 */
export async function apiFetch(url: string, options: RequestInit = {}): Promise<Response> {
	const resolvedUrl = resolveApiUrl(url);

	// Get the current non-reactive access token.
	const token = auth.getAccessToken();

	// Set up the headers.
	const headers = new Headers(options.headers);
	if (token) {
		headers.set('Authorization', `Bearer ${token}`);
	}
	options.headers = headers;
	if (options.credentials === undefined) {
		options.credentials = 'include';
	}

	// Make the initial request.
	let response = await fetch(resolvedUrl, options);

	// If the response is a 401 Unauthorized, share a single refresh attempt
	// across all concurrent callers and retry exactly once.
	if (response.status === 401) {
		try {
			const currentToken = auth.getAccessToken();
			const newAccessToken = currentToken && currentToken !== token
				? currentToken
				: await auth.refreshToken();
			if (newAccessToken) {
				headers.set('Authorization', `Bearer ${newAccessToken}`);
				options.headers = headers;
				response = await fetch(resolvedUrl, options);
			}
		} catch (error) {
			// Refresh failed. Return the original 401 to the caller so the UI can react;
			// the auth store only logs out on confirmed unauthorized refresh responses.
			console.error('Failed to retry request after token refresh.', error);
			return response;
		}
	}

	return response;
}
