<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import * as m from '$paraglide/messages';
	import { listAdminUserMedia } from '$lib/api/admin';
	import ArrowLeft from '~icons/ph/arrow-left';
	import FileImage from '~icons/ph/file-image';
	import FileVideo from '~icons/ph/file-video';
	import File from '~icons/ph/file';

	const PAGE_SIZE = 20;

	let items = $state<Awaited<ReturnType<typeof listAdminUserMedia>>['items']>([]);
	let total = $state(0);
	let hasMore = $state(false);
	let offset = $state(0);
	let loading = $state(true);
	let loadingMore = $state(false);
	let errorMessage = $state('');
	let queryInput = $state('');
	let query = $state('');
	let kind = $state<'all' | 'image' | 'video' | 'file'>('all');
	let status = $state<'all' | 'ready' | 'pending_delete' | 'deleted' | 'failed'>('all');
	let loadMoreAnchor = $state<HTMLDivElement | null>(null);

	onMount(() => {
		void loadMedia(true);
	});

	$effect(() => {
		if (!hasMore || !loadMoreAnchor) return;
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries.some((entry) => entry.isIntersecting)) {
					void loadMedia(false);
				}
			},
			{ rootMargin: '320px 0px' }
		);
		observer.observe(loadMoreAnchor);
		return () => observer.disconnect();
	});

	function formatBytes(bytes: number): string {
		if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
		const units = ['B', 'KB', 'MB', 'GB'];
		let value = bytes;
		let idx = 0;
		while (value >= 1024 && idx < units.length - 1) {
			value /= 1024;
			idx += 1;
		}
		return `${value.toFixed(value >= 10 || idx === 0 ? 0 : 1)} ${units[idx]}`;
	}

	function formatDate(value: string): string {
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return value;
		return date.toLocaleString();
	}

	async function loadMedia(reset: boolean) {
		if (!reset && (loading || loadingMore || !hasMore)) return;
		if (reset) {
			loading = true;
			errorMessage = '';
			offset = 0;
		} else {
			loadingMore = true;
		}

		const userID = $page.params.id;
		if (!userID) {
			errorMessage = m.admin_user_media_load_failed();
			loading = false;
			loadingMore = false;
			return;
		}

		try {
			const payload = await listAdminUserMedia({
				userID,
				q: query,
				kind,
				status,
				limit: PAGE_SIZE,
				offset: reset ? 0 : offset
			});
			items = reset ? payload.items : [...items, ...payload.items];
			total = payload.total;
			hasMore = payload.hasMore;
			offset = (reset ? 0 : offset) + payload.items.length;
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : m.admin_user_media_load_failed();
			if (reset) {
				items = [];
				total = 0;
				hasMore = false;
				offset = 0;
			}
		} finally {
			loading = false;
			loadingMore = false;
		}
	}

	function handleSearchSubmit(event: SubmitEvent) {
		event.preventDefault();
		query = queryInput.trim();
		void loadMedia(true);
	}
</script>

<svelte:head>
	<title>{m.admin_user_media_page_title()} - {m.admin_page_title()}</title>
</svelte:head>

<section class="space-y-6">
	<button
		type="button"
		class="inline-flex items-center gap-2 text-sm font-medium text-zinc-600 transition hover:text-zinc-900 dark:text-zinc-300 dark:hover:text-zinc-100"
		onclick={() => goto(`/admin/users/${$page.params.id}`)}
	>
		<ArrowLeft class="h-4 w-4" />
		{m.admin_user_media_back_to_detail()}
	</button>

	<div class="space-y-1">
		<h1 class="text-xl font-semibold text-zinc-900 dark:text-zinc-100">{m.admin_user_media_title()}</h1>
		<p class="text-sm text-zinc-500 dark:text-zinc-400">{m.admin_user_media_description()}</p>
	</div>

	<form class="flex flex-col gap-3 sm:flex-row" onsubmit={handleSearchSubmit}>
		<input
			bind:value={queryInput}
			type="search"
			class="w-full rounded-md border border-zinc-200 bg-white px-4 py-2 text-sm text-zinc-900 outline-none transition focus:border-cyan-400 focus:ring-2 focus:ring-cyan-200 dark:border-zinc-600 dark:bg-zinc-900 dark:text-zinc-100 dark:focus:border-cyan-500 dark:focus:ring-cyan-900/40"
			placeholder={m.admin_user_media_search_placeholder()}
		/>
		<select
			bind:value={kind}
			class="rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900 outline-none transition focus:border-cyan-400 focus:ring-2 focus:ring-cyan-200 dark:border-zinc-600 dark:bg-zinc-900 dark:text-zinc-100 dark:focus:border-cyan-500 dark:focus:ring-cyan-900/40"
			onchange={() => void loadMedia(true)}
		>
			<option value="all">{m.user_media_filter_kind_all()}</option>
			<option value="image">{m.user_media_filter_kind_image()}</option>
			<option value="video">{m.user_media_filter_kind_video()}</option>
			<option value="file">{m.user_media_filter_kind_file()}</option>
		</select>
		<select
			bind:value={status}
			class="rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900 outline-none transition focus:border-cyan-400 focus:ring-2 focus:ring-cyan-200 dark:border-zinc-600 dark:bg-zinc-900 dark:text-zinc-100 dark:focus:border-cyan-500 dark:focus:ring-cyan-900/40"
			onchange={() => void loadMedia(true)}
		>
			<option value="all">{m.user_media_filter_status_all()}</option>
			<option value="ready">{m.user_media_status_ready()}</option>
			<option value="pending_delete">{m.user_media_status_pending_delete()}</option>
			<option value="deleted">{m.user_media_status_deleted()}</option>
			<option value="failed">{m.user_media_status_failed()}</option>
		</select>
		<button
			type="submit"
			class="inline-flex min-w-20 items-center justify-center whitespace-nowrap rounded-md bg-cyan-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-cyan-500 dark:bg-cyan-600 dark:hover:bg-cyan-500"
		>
			{m.common_search_placeholder()}
		</button>
	</form>

	{#if loading}
		<p class="text-sm text-zinc-500 dark:text-zinc-400">{m.common_loading()}</p>
	{:else if errorMessage}
		<p class="text-sm text-rose-600 dark:text-rose-300">{errorMessage}</p>
	{:else if items.length === 0}
		<p class="text-sm text-zinc-500 dark:text-zinc-400">{m.admin_user_media_empty()}</p>
	{:else}
		<div class="border-t border-zinc-200 dark:border-zinc-700">
			<div class="flex items-center justify-between px-4 py-3 text-sm text-zinc-500 dark:text-zinc-400">
				<p>{m.admin_user_media_total({ count: String(total) })}</p>
				{#if query}
					<p>{m.admin_users_searching({ query })}</p>
				{/if}
			</div>

			{#each items as item (item.id)}
				<div class="flex items-start justify-between gap-4 border-b border-zinc-200 px-4 py-3 dark:border-zinc-700">
					<div class="flex min-w-0 items-start gap-3">
						<div class="mt-0.5 text-zinc-400 dark:text-zinc-500">
							{#if item.kind === 'image'}
								<FileImage class="h-5 w-5" />
							{:else if item.kind === 'video'}
								<FileVideo class="h-5 w-5" />
							{:else}
								<File class="h-5 w-5" />
							{/if}
						</div>
						<div class="min-w-0">
							<p class="truncate text-sm font-medium text-zinc-900 dark:text-zinc-100">{item.filename}</p>
							<p class="mt-1 text-xs text-zinc-500 dark:text-zinc-400">
								{item.mimeType} · {formatBytes(item.fileSize)} · {formatDate(item.createdAt)}
							</p>
						</div>
					</div>
					<div class="shrink-0 text-right text-xs text-zinc-500 dark:text-zinc-400">
						<p>{item.status}</p>
						<p class="mt-1">{m.user_media_meta_reference()}：{item.referenceCount}</p>
					</div>
				</div>
			{/each}
		</div>

		<div bind:this={loadMoreAnchor} class="h-8"></div>
		{#if loadingMore}
			<p class="text-sm text-zinc-500 dark:text-zinc-400">{m.common_loading()}</p>
		{/if}
	{/if}
</section>
