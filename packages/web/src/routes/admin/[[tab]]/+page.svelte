<script lang="ts">
	import { page } from '$app/stores';
	import * as m from '$paraglide/messages';
	import AdminOverviewTab from '$lib/components/admin/AdminOverviewTab.svelte';
	import AdminUsersTab from '$lib/components/admin/AdminUsersTab.svelte';

	let tab = $derived($page.params.tab || 'overview');

	const titles: Record<string, any> = {
		get overview() { return m.admin_nav_overview(); },
		get users() { return m.admin_nav_users(); }
	};

	const descriptions: Record<string, any> = {
		get overview() { return m.admin_overview_description(); },
		get users() { return m.admin_users_description(); }
	};
</script>

<svelte:head>
	<title>{titles[tab] || m.admin_panel_title()} - {m.admin_page_title()}</title>
</svelte:head>

<section class="space-y-6">
	<div>
		<h1 class="text-2xl font-bold text-zinc-900 dark:text-zinc-100">{titles[tab] || m.admin_panel_title()}</h1>
		<p class="mt-1 text-sm text-zinc-600 dark:text-zinc-400">
			{descriptions[tab] || ''}
		</p>
	</div>

	{#if tab === 'overview'}
		<AdminOverviewTab />
	{:else if tab === 'users'}
		<AdminUsersTab />
	{/if}
</section>
