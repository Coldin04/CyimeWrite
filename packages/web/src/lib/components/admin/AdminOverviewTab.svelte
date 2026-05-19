<script lang="ts">
	import { onMount } from 'svelte';
	import * as m from '$paraglide/messages';
	import { getAdminOverview, type AdminOverview } from '$lib/api/admin';

	let overview = $state<AdminOverview | null>(null);
	let loading = $state(true);
	let errorMessage = $state('');

	onMount(() => {
		void loadOverview();
	});

	async function loadOverview() {
		loading = true;
		errorMessage = '';
		try {
			overview = await getAdminOverview();
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : m.admin_overview_load_failed();
		} finally {
			loading = false;
		}
	}
</script>

<div class="space-y-4">
	{#if loading}
		<div class="rounded-xl border border-dashed border-zinc-200 px-4 py-6 text-sm text-zinc-500 dark:border-zinc-700 dark:text-zinc-400">
			{m.common_loading()}
		</div>
	{:else if errorMessage}
		<div class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-6 text-sm text-rose-700 dark:border-rose-900/50 dark:bg-rose-950/20 dark:text-rose-300">
			{errorMessage}
		</div>
	{:else if overview}
		<div class="grid gap-4 md:grid-cols-3">
			<div class="rounded-xl border border-zinc-200 p-5 dark:border-zinc-800">
				<p class="text-xs font-medium uppercase tracking-wide text-zinc-500 dark:text-zinc-400">{m.admin_overview_user_count()}</p>
				<p class="mt-2 text-3xl font-semibold text-zinc-900 dark:text-zinc-100">{overview.userCount}</p>
				<p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">{m.admin_overview_user_count_hint()}</p>
			</div>

			<div class="rounded-xl border border-zinc-200 p-5 dark:border-zinc-800">
				<p class="text-xs font-medium uppercase tracking-wide text-zinc-500 dark:text-zinc-400">{m.admin_overview_admin_count()}</p>
				<p class="mt-2 text-3xl font-semibold text-zinc-900 dark:text-zinc-100">{overview.adminCount}</p>
				<p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">{m.admin_overview_admin_count_hint()}</p>
			</div>

			<div class="rounded-xl border border-zinc-200 p-5 dark:border-zinc-800">
				<p class="text-xs font-medium uppercase tracking-wide text-zinc-500 dark:text-zinc-400">{m.admin_overview_global_quota()}</p>
				<p class="mt-2 text-3xl font-semibold text-zinc-900 dark:text-zinc-100">
					{overview.globalUnlimited ? m.admin_quota_unlimited() : overview.globalDocumentQuota}
				</p>
				<p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">
					{overview.globalUnlimited ? m.admin_overview_global_quota_unlimited_hint() : m.admin_overview_global_quota_limited_hint()}
				</p>
			</div>
		</div>
	{/if}
</div>

