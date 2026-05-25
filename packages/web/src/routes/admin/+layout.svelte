<script lang="ts">
	import { browser } from '$app/environment';
	import { goto } from '$app/navigation';
	import * as m from '$paraglide/messages';
	import RouteAuthGuard from '$lib/components/auth/RouteAuthGuard.svelte';
	import SettingsShell from '$lib/components/settings/SettingsShell.svelte';
	import { auth } from '$lib/stores/auth';
	import Gauge from '~icons/ph/gauge';
	import UsersThree from '~icons/ph/users-three';

	let { children } = $props();

	const navItems = [
		{ href: '/admin', label: m.admin_nav_overview(), icon: Gauge },
		{ href: '/admin/users', label: m.admin_nav_users(), icon: UsersThree }
	];
	const hasAdminAccess = $derived($auth.user?.adminAccess?.hasAccess === true);

	$effect(() => {
		if (!browser) return;
		if (!$auth.loading && $auth.authenticated && !hasAdminAccess) {
			void goto('/user', { replaceState: true });
		}
	});
</script>

<RouteAuthGuard mode="required">
	{#if !$auth.loading && hasAdminAccess}
		<SettingsShell {navItems}>
			{@render children()}
		</SettingsShell>
	{:else}
		<div class="flex h-screen w-full items-center justify-center bg-gray-50 dark:bg-gray-900">
			<p class="text-lg text-gray-600 dark:text-gray-300">{m.workspace_loading()}</p>
		</div>
	{/if}
</RouteAuthGuard>
