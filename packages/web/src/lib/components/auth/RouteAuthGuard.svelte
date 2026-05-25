<script lang="ts">
	import * as m from '$paraglide/messages';
	import { browser } from '$app/environment';
	import { goto } from '$app/navigation';
	import { auth } from '$lib/stores/auth';

	type Props = {
		mode?: 'required' | 'optional' | 'guest';
	};

	let { children, mode = 'required' }: Props & { children: import('svelte').Snippet } = $props();
	const requiresAuth = $derived(mode === 'required');
	const requiresGuest = $derived(mode === 'guest');
	// Show loading/placeholder when: required auth is loading or pending redirect to login,
	// or guest mode has an authenticated user being redirected to workspace.
	const showLoadingScreen = $derived(
		(requiresAuth && ($auth.loading || !$auth.authenticated)) ||
			(requiresGuest && !$auth.loading && $auth.authenticated)
	);

	$effect(() => {
		if (!browser) return;
		if (requiresAuth && !$auth.loading && !$auth.authenticated) {
			goto('/login', { replaceState: true });
		}
		if (requiresGuest && !$auth.loading && $auth.authenticated) {
			goto('/workspace', { replaceState: true });
		}
	});
</script>

{#if showLoadingScreen}
	<div class="flex h-screen w-full items-center justify-center bg-gray-50 dark:bg-gray-900">
		<p class="text-lg text-gray-600 dark:text-gray-300">{m.workspace_loading()}</p>
	</div>
{:else}
	{@render children()}
{/if}
