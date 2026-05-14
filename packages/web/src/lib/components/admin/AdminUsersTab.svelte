<script lang="ts">
	import { browser } from '$app/environment';
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import * as m from '$paraglide/messages';
	import { listAdminUsers, type AdminUserListItem } from '$lib/api/admin';
	import AdminUserListHeader from '$lib/components/admin/AdminUserListHeader.svelte';
	import AdminUserListItemRow from '$lib/components/admin/AdminUserListItem.svelte';

	const PAGE_SIZE = 20;

	let items = $state<AdminUserListItem[]>([]);
	let total = $state(0);
	let hasMore = $state(false);
	let offset = $state(0);
	let loading = $state(true);
	let loadingMore = $state(false);
	let errorMessage = $state('');
	let queryInput = $state('');
	let query = $state('');
	let globalDocumentQuota = $state<number | null>(null);
	let globalUnlimited = $state(true);
	let loadMoreAnchor = $state<HTMLDivElement | null>(null);

	onMount(() => {
		void loadUsers(true);
	});

	$effect(() => {
		if (!browser || !hasMore || !loadMoreAnchor) {
			return;
		}

		const observer = new IntersectionObserver(
			(entries) => {
				if (entries.some((entry) => entry.isIntersecting)) {
					void loadUsers(false);
				}
			},
			{
				rootMargin: '320px 0px'
			}
		);

		observer.observe(loadMoreAnchor);
		return () => {
			observer.disconnect();
		};
	});

	async function loadUsers(reset: boolean) {
		if (!reset && (loading || loadingMore || !hasMore)) {
			return;
		}

		if (reset) {
			loading = true;
			errorMessage = '';
			offset = 0;
		} else {
			loadingMore = true;
		}

		try {
			const data = await listAdminUsers({
				limit: PAGE_SIZE,
				offset: reset ? 0 : offset,
				q: query
			});
			globalDocumentQuota = data.globalDocumentQuota;
			globalUnlimited = data.globalUnlimited;
			items = reset ? data.items : [...items, ...data.items];
			total = data.total;
			hasMore = data.hasMore;
			offset = data.nextOffset;
		} catch (error) {
			const message = error instanceof Error ? error.message : m.admin_users_load_failed();
			if (reset) {
				errorMessage = message;
				items = [];
				total = 0;
				hasMore = false;
				offset = 0;
			} else {
				toast.error(message);
			}
		} finally {
			loading = false;
			loadingMore = false;
		}
	}

	function handleSearchSubmit(event: SubmitEvent) {
		event.preventDefault();
		query = queryInput.trim();
		void loadUsers(true);
	}
</script>

<div class="space-y-6">
	<p class="text-sm text-zinc-500 dark:text-zinc-400">
		{m.admin_users_global_quota_hint({
			value: globalUnlimited ? m.admin_quota_unlimited() : String(globalDocumentQuota ?? '-')
		})}
	</p>

	<form class="flex flex-col gap-3 sm:flex-row" onsubmit={handleSearchSubmit}>
		<input
			bind:value={queryInput}
			type="search"
			class="w-full rounded-md border border-zinc-200 bg-white px-4 py-2 text-sm text-zinc-900 outline-none transition focus:border-cyan-400 focus:ring-2 focus:ring-cyan-200 dark:border-zinc-600 dark:bg-zinc-900 dark:text-zinc-100 dark:focus:border-cyan-500 dark:focus:ring-cyan-900/40"
			placeholder={m.admin_users_search_placeholder()}
		/>
		<button
			type="submit"
			class="inline-flex min-w-20 items-center justify-center whitespace-nowrap rounded-md bg-cyan-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-cyan-500 dark:bg-cyan-600 dark:hover:bg-cyan-500"
		>
			{m.common_search_placeholder()}
		</button>
	</form>

	{#if loading}
		<div class="border border-dashed border-zinc-200 px-4 py-6 text-sm text-zinc-500 dark:border-zinc-700 dark:text-zinc-400">
			{m.common_loading()}
		</div>
	{:else if errorMessage}
		<div class="border border-rose-200 bg-rose-50 px-4 py-6 text-sm text-rose-700 dark:border-rose-900/50 dark:bg-rose-950/20 dark:text-rose-300">
			{errorMessage}
		</div>
	{:else if items.length === 0}
		<div class="border border-dashed border-zinc-200 px-4 py-10 text-sm text-zinc-500 dark:border-zinc-700 dark:text-zinc-400">
			{m.admin_users_empty()}
		</div>
	{:else}
		<div class="border-t border-zinc-200 dark:border-zinc-700">
			<div class="flex items-center justify-between px-4 py-3 text-sm text-zinc-500 dark:text-zinc-400">
				<p>{m.admin_users_total({ count: String(total) })}</p>
				{#if query}
					<p>{m.admin_users_searching({ query })}</p>
				{/if}
			</div>
			<AdminUserListHeader />
			{#each items as item (item.id)}
				<AdminUserListItemRow {item} />
			{/each}
		</div>

		<div bind:this={loadMoreAnchor} class="h-8"></div>
		{#if loadingMore}
			<p class="text-sm text-zinc-500 dark:text-zinc-400">{m.common_loading()}</p>
		{/if}
	{/if}
</div>
