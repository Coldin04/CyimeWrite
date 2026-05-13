<script lang="ts">
	import { apiFetch } from '$lib/api';
	import * as m from '$paraglide/messages';
	import User from '~icons/ph/user';

	const avatarBlobCache = new Map<string, string>();
	const avatarFetchInFlight = new Map<string, Promise<string>>();

	interface Props {
		name?: string | null;
		avatarUrl?: string | null;
		size?: number;
		className?: string;
	}

	let { name = null, avatarUrl = null, size = 64, className = '' }: Props = $props();

	let loadFailed = $state(false);
	let loaded = $state(false);
	let resolvedSrc = $state('');
	let imgEl = $state<HTMLImageElement | null>(null);
	const normalizedUrl = $derived((avatarUrl || '').trim());
	const displayName = $derived((name || '').trim());
	const fallbackIconSize = $derived(Math.max(18, Math.round(size * 0.42)));

	async function resolveAvatarSource(url: string): Promise<string> {
		if (!url.startsWith('/api/v1/user/avatar/content')) {
			return url;
		}

		const cached = avatarBlobCache.get(url);
		if (cached) {
			return cached;
		}

		const inFlight = avatarFetchInFlight.get(url);
		if (inFlight) {
			return inFlight;
		}

		const request = (async () => {
			const resp = await apiFetch(url);
			if (!resp.ok) {
				throw new Error('avatar fetch failed');
			}
			const blob = await resp.blob();
			const objectURL = URL.createObjectURL(blob);
			avatarBlobCache.set(url, objectURL);
			return objectURL;
		})();

		avatarFetchInFlight.set(url, request);
		try {
			return await request;
		} finally {
			avatarFetchInFlight.delete(url);
		}
	}

	$effect(() => {
		const nextURL = normalizedUrl;
		loadFailed = false;
		loaded = false;
		resolvedSrc = '';

		if (!nextURL) return;

		let disposed = false;
		void (async () => {
			try {
				const finalSrc = await resolveAvatarSource(nextURL);
				if (disposed) return;
				resolvedSrc = finalSrc;
			} catch {
				if (disposed) return;
				loadFailed = true;
			}
		})();

		return () => {
			disposed = true;
		};
	});

	$effect(() => {
		if (imgEl && imgEl.complete && imgEl.naturalWidth > 0) {
			loaded = true;
		}
	});
</script>

<div
	class={`relative grid shrink-0 aspect-square place-content-center overflow-hidden rounded-full bg-sky-100 dark:bg-sky-900 ${className}`}
	style={`width:${size}px;height:${size}px;min-width:${size}px;min-height:${size}px;`}
>
	{#if resolvedSrc && !loadFailed}
		{#if !loaded}
			<div
				class="absolute inset-0 animate-pulse bg-sky-200/80 dark:bg-sky-800/70"
				aria-hidden="true"
			></div>
		{/if}
		<img
			bind:this={imgEl}
			src={resolvedSrc}
			alt={m.greeting_avatar_alt({ name: displayName || m.common_user() })}
			class="h-full w-full rounded-full object-cover transition-opacity duration-200"
			class:opacity-0={!loaded}
			class:opacity-100={loaded}
			decoding="async"
			fetchpriority="low"
			referrerpolicy="no-referrer"
			onload={() => {
				loaded = true;
			}}
			onerror={() => {
				loadFailed = true;
			}}
		/>
	{:else}
		<User
			class="text-sky-600 dark:text-sky-300"
			style={`width:${fallbackIconSize}px;height:${fallbackIconSize}px;`}
			aria-label={m.common_user()}
		/>
	{/if}
</div>
