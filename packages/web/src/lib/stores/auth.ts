import { get, writable } from 'svelte/store';
import { browser } from '$app/environment';
import { resolveApiUrl } from '$lib/config/api';

export type User = {
	id: string;
	displayName: string | null;
	email: string | null;
	avatarUrl: string | null;
	adminAccess: {
		hasAccess: boolean;
		role: string | null;
	};
};

type AuthState = {
	authenticated: boolean;
	user: User | null;
	loading: boolean;
};

// Keep the rotating access token outside the Svelte store. Token refresh should
// not invalidate page effects; only real session/UI changes should notify subscribers.
let accessToken: string | null = null;
let refreshTimerId: NodeJS.Timeout | null = null;
let refreshRetryTimerId: NodeJS.Timeout | null = null;
let refreshPromise: Promise<string | null> | null = null;

const REFRESH_RETRY_DELAY_MS = 30_000;
const MIN_REFRESH_RETRY_DELAY_MS = 1_000;
const REFRESH_UNAUTHORIZED_RETRY_DELAY_MS = 500;
const EXPIRY_SKEW_MS = 10_000;

type RefreshError = Error & {
	status?: number;
};

type NavigatorWithLocks = Navigator & {
	locks?: {
		request<T>(name: string, callback: () => Promise<T>): Promise<T>;
	};
};

function parseJwt(token: string): { exp?: number } {
	try {
		const base64Url = token.split('.')[1];
		const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
		const jsonPayload = JSON.parse(atob(base64));
		return { exp: jsonPayload.exp };
	} catch (e) {
		return {};
	}
}

function getTokenExpiresAt(token: string): number | null {
	const { exp } = parseJwt(token);
	return exp ? exp * 1000 : null;
}

function setAccessToken(token: string | null) {
	accessToken = token;
}

function wait(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

async function withCrossTabRefreshLock<T>(callback: () => Promise<T>): Promise<T> {
	if (!browser) {
		return callback();
	}

	const locks = (navigator as NavigatorWithLocks).locks;
	if (!locks) {
		return callback();
	}

	return locks.request('cyime-auth-refresh', callback);
}

function createAuthStore() {
	const { subscribe, set, update } = writable<AuthState>({
		authenticated: false,
		user: null,
		loading: true
	});

	async function _fetchUser(token: string): Promise<User> {
		const response = await fetch(resolveApiUrl('/api/v1/user/me'), {
			headers: { Authorization: `Bearer ${token}` },
			credentials: 'include'
		});
		if (!response.ok) throw new Error('Failed to fetch user profile');
		const user: User = await response.json();
		return user;
	}

	function clearRefreshRetryTimer() {
		if (refreshRetryTimerId) {
			clearTimeout(refreshRetryTimerId);
			refreshRetryTimerId = null;
		}
	}

	function scheduleRefreshRetry(token: string) {
		clearRefreshRetryTimer();

		const expiresAt = getTokenExpiresAt(token);
		if (!expiresAt) return;

		const remainingLifetimeMs = expiresAt - Date.now();
		if (remainingLifetimeMs <= 0) return;

		const retryBeforeExpiryMs = remainingLifetimeMs - EXPIRY_SKEW_MS;
		const retryIn =
			retryBeforeExpiryMs > 0
				? Math.min(REFRESH_RETRY_DELAY_MS, retryBeforeExpiryMs)
				: Math.min(MIN_REFRESH_RETRY_DELAY_MS, remainingLifetimeMs);

		refreshRetryTimerId = setTimeout(() => {
			void refreshToken().catch((error) => {
				console.error('Scheduled token refresh retry failed:', error);
			});
		}, retryIn);
	}

	async function requestNewAccessToken(): Promise<string> {
		const response = await withCrossTabRefreshLock(() =>
			fetch(resolveApiUrl('/api/v1/auth/refresh'), {
				method: 'POST',
				credentials: 'include'
			})
		);

		if (!response.ok) {
			const error = new Error('Refresh failed') as RefreshError;
			error.status = response.status;
			throw error;
		}

		const { accessToken } = await response.json();
		return accessToken;
	}

	async function performRefreshToken() {
		console.log('Attempting to refresh token...');
		try {
			let newAccessToken: string;
			try {
				newAccessToken = await requestNewAccessToken();
			} catch (error) {
				const refreshError = error as RefreshError;
				if (refreshError.status === 401) {
					await wait(REFRESH_UNAUTHORIZED_RETRY_DELAY_MS);
					newAccessToken = await requestNewAccessToken();
				} else {
					throw error;
				}
			}

			setAccessToken(newAccessToken);
			scheduleRefresh(newAccessToken);
			const currentState = get({ subscribe });
			if (currentState.loading || !currentState.authenticated || !currentState.user) {
				try {
					const user = await _fetchUser(newAccessToken);
					set({ authenticated: true, user, loading: false });
				} catch (profileError) {
					console.error('Failed to refresh user profile after token refresh:', profileError);
					update((state) => ({ ...state, authenticated: true, loading: false }));
				}
			}
			console.log('Token refreshed successfully.');
			return newAccessToken; // Return the new token on success
		} catch (error) {
			console.error('Could not refresh token:', error);
			const refreshError = error as RefreshError;
			if (refreshError.status === 401 || refreshError.status === 403) {
				await logout();
			} else {
				const currentToken = accessToken;
				if (currentToken) {
					scheduleRefreshRetry(currentToken);
				}
			}
			throw error; // Propagate the error
		}
	}

	function refreshToken() {
		if (!refreshPromise) {
			refreshPromise = performRefreshToken().finally(() => {
				refreshPromise = null;
			});
		}
		return refreshPromise;
	}

	function scheduleRefresh(token: string) {
		if (refreshTimerId) {
			clearTimeout(refreshTimerId);
		}
		clearRefreshRetryTimer();

		const expiresAt = getTokenExpiresAt(token);
		if (!expiresAt) return;
		const now = Date.now();
		const expiresIn = expiresAt - now;

		// Schedule refresh for 85% of the token's remaining lifetime.
		const timeout = expiresIn * 0.85;

		if (timeout > 0) {
			refreshTimerId = setTimeout(refreshToken, timeout);
		}
	}

	async function init() {
		if (!browser) {
			update((state) => ({ ...state, loading: false }));
			return;
		}

		// Try to restore session from the backend using the refresh token cookie.
		try {
			const newAccessToken = await requestNewAccessToken();
			const user = await _fetchUser(newAccessToken);
			setAccessToken(newAccessToken);
			set({ authenticated: true, user, loading: false });
			scheduleRefresh(newAccessToken);
			console.log('Session restored successfully.');
			return;
		} catch (error) {
			// Session restoration failed, user is not logged in
			console.log('No active session found.');
		}

		// No active session, just set loading to false
		update((state) => ({ ...state, loading: false }));
	}

	async function loginAndFetchUser(token: string) {
		const { exp } = parseJwt(token);
		if (!exp || exp * 1000 < Date.now()) {
			logout();
			return;
		}

		try {
			const user = await _fetchUser(token);
			setAccessToken(token);
			set({ authenticated: true, user, loading: false });
			scheduleRefresh(token); // Schedule the first refresh on successful login.
		} catch (error) {
			console.error('Failed to log in:', error);
			logout();
		}
	}

	async function refreshUser() {
		const token = accessToken;
		if (!token) {
			return null;
		}

		const user = await _fetchUser(token);
		update((state) => ({ ...state, user }));
		return user;
	}

	function setUser(user: User | null) {
		update((state) => ({ ...state, user }));
	}

	function getAccessToken() {
		return accessToken;
	}

	async function logout() {
		if (refreshTimerId) {
			clearTimeout(refreshTimerId);
			refreshTimerId = null;
		}
		clearRefreshRetryTimer();

		try {
			// Inform the backend to revoke the refresh token.
			const response = await fetch(resolveApiUrl('/api/v1/auth/logout'), {
				method: 'POST',
				credentials: 'include'
			});
			if (!response.ok) {
				// We can log this error, but we still want to clear the local state.
				console.error('Backend logout failed:', response.statusText);
			}
		} catch (error) {
			// Also log network errors etc.
			console.error('Error during logout API call:', error);
		} finally {
			// Always clear the local state to log the user out on the frontend.
			setAccessToken(null);
			set({ authenticated: false, user: null, loading: false });
		}
	}

	return {
		subscribe,
		init,
		loginAndFetchUser,
		refreshUser,
		setUser,
		getAccessToken,
		logout,
		refreshToken // Expose the refresh function
	};
}

export const auth = createAuthStore();

auth.init();
