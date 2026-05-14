<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { toast } from 'svelte-sonner';
	import * as m from '$paraglide/messages';
	import UserAvatar from '$lib/components/common/UserAvatar.svelte';
	import { getAdminUser, updateAdminUserDocumentQuota, type AdminUserListItem } from '$lib/api/admin';
	import ArrowLeft from '~icons/ph/arrow-left';
	import Files from '~icons/ph/files';
	import Trash from '~icons/ph/trash';
	import HardDrive from '~icons/ph/hard-drive';

	type QuotaMode = 'inherit' | 'custom' | 'unlimited';

	let item = $state<AdminUserListItem | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let errorMessage = $state('');
	let quotaMode = $state<QuotaMode>('inherit');
	let quotaInput = $state('');

	onMount(() => {
		void loadUser();
	});

async function loadUser() {
		loading = true;
		errorMessage = '';
		const userID = $page.params.id;
		if (!userID) {
			errorMessage = m.admin_user_detail_load_failed();
			item = null;
			loading = false;
			return;
		}
		try {
			const detail = await getAdminUser(userID);
			item = detail;
			quotaMode = normalizeMode(detail.documentQuotaMode);
			quotaInput = detail.documentQuota === null ? '' : String(detail.documentQuota);
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : m.admin_user_detail_load_failed();
			item = null;
		} finally {
			loading = false;
		}
	}

	function normalizeMode(mode: string): QuotaMode {
		if (mode === 'custom' || mode === 'unlimited') return mode;
		return 'inherit';
	}

	function effectiveQuotaLabel(): string {
		if (!item) return '-';
		if (item.unlimited) return m.admin_quota_unlimited();
		return item.effectiveDocumentQuota === null ? '-' : String(item.effectiveDocumentQuota);
	}

	async function handleSave(event: SubmitEvent) {
		event.preventDefault();
		if (!item) return;

		let quotaValue: number | null = null;
		if (quotaMode === 'custom') {
			const trimmed = quotaInput.trim();
			if (!/^\d+$/.test(trimmed)) {
				toast.error(m.admin_users_quota_invalid());
				return;
			}
			quotaValue = Number.parseInt(trimmed, 10);
		}

		saving = true;
		try {
			item = await updateAdminUserDocumentQuota(item.id, {
				documentQuotaMode: quotaMode,
				documentQuota: quotaValue
			});
			quotaMode = normalizeMode(item.documentQuotaMode);
			quotaInput = item.documentQuota === null ? '' : String(item.documentQuota);
			toast.success(m.admin_users_quota_saved());
		} catch (error) {
			toast.error(error instanceof Error ? error.message : m.admin_users_quota_save_failed());
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>{m.admin_user_detail_title()} - {m.admin_page_title()}</title>
</svelte:head>

<section class="space-y-6">
	<button
		type="button"
		class="inline-flex items-center gap-2 text-sm font-medium text-zinc-600 transition hover:text-zinc-900 dark:text-zinc-300 dark:hover:text-zinc-100"
		onclick={() => goto('/admin/users')}
	>
		<ArrowLeft class="h-4 w-4" />
		{m.admin_users_back_to_list()}
	</button>

	{#if loading}
		<div class="rounded-xl border border-dashed border-zinc-200 px-4 py-6 text-sm text-zinc-500 dark:border-zinc-700 dark:text-zinc-400">
			{m.common_loading()}
		</div>
	{:else if errorMessage}
		<div class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-6 text-sm text-rose-700 dark:border-rose-900/50 dark:bg-rose-950/20 dark:text-rose-300">
			{errorMessage}
		</div>
	{:else if item}
		<div class="space-y-8">
			<div>
				<div class="flex items-center gap-3">
					<UserAvatar size={48} name={item.displayName} avatarUrl={item.avatarUrl} />
					<div class="min-w-0">
						<p class="truncate text-base font-semibold text-zinc-900 dark:text-zinc-100">
							{item.displayName || m.user_common_default_name()}
						</p>
						<p class="truncate text-sm text-zinc-500 dark:text-zinc-400">
							{item.email || m.user_common_no_email()}
						</p>
					</div>
				</div>

				<div class="mt-10 flex flex-col gap-8 sm:flex-row sm:items-start sm:gap-0">
					<div class="flex-1">
						<div class="flex items-center gap-2 text-zinc-400 dark:text-zinc-500">
							<Files class="h-3.5 w-3.5" />
							<p class="text-[10px] font-bold uppercase tracking-widest">{m.admin_users_active_documents()}</p>
						</div>
						<p class="mt-1 text-2xl font-semibold text-zinc-900 dark:text-zinc-100">{item.activeDocumentCount}</p>
					</div>

					<div class="mx-10 hidden h-8 w-px self-center bg-zinc-100 dark:bg-zinc-800/50 sm:block"></div>

					<div class="flex-1">
						<div class="flex items-center gap-2 text-zinc-400 dark:text-zinc-500">
							<Trash class="h-3.5 w-3.5" />
							<p class="text-[10px] font-bold uppercase tracking-widest">{m.admin_users_trashed_documents()}</p>
						</div>
						<p class="mt-1 text-2xl font-semibold text-zinc-900 dark:text-zinc-100">{item.trashedDocumentCount}</p>
					</div>

					<div class="mx-10 hidden h-8 w-px self-center bg-zinc-100 dark:bg-zinc-800/50 sm:block"></div>

					<div class="flex-1">
						<div class="flex items-center gap-2 text-zinc-400 dark:text-zinc-500">
							<HardDrive class="h-3.5 w-3.5" />
							<p class="text-[10px] font-bold uppercase tracking-widest">{m.admin_users_effective_quota()}</p>
						</div>
						<p class="mt-1 text-2xl font-semibold text-zinc-900 dark:text-zinc-100">{effectiveQuotaLabel()}</p>
					</div>
				</div>
			</div>

			<form class="mt-12 space-y-10" onsubmit={handleSave}>
				<div class="space-y-8">
					<div class="space-y-1">
						<h2 class="text-lg font-semibold text-zinc-900 dark:text-zinc-100">{m.admin_users_quota_title()}</h2>
						<p class="text-sm text-zinc-500 dark:text-zinc-400">{m.admin_user_detail_quota_hint()}</p>
					</div>

					<div class="max-w-2xl">
						<div class="grid gap-6 sm:grid-cols-[minmax(0,180px)_minmax(0,1fr)] sm:items-center">
							<label for="quota-mode" class="text-sm font-medium text-zinc-600 dark:text-zinc-400">
								{m.admin_users_quota_title()}
							</label>

							<div class="space-y-4">
								<select
									id="quota-mode"
									bind:value={quotaMode}
									class="w-full rounded-md border border-zinc-200 bg-transparent px-3 py-2 text-sm text-zinc-900 outline-none transition focus:border-cyan-500 focus:ring-1 focus:ring-cyan-500 dark:border-zinc-800 dark:text-zinc-100 dark:focus:border-cyan-500"
								>
									<option value="inherit">{m.admin_quota_inherit()}</option>
									<option value="custom">{m.admin_quota_custom()}</option>
									<option value="unlimited">{m.admin_quota_unlimited()}</option>
								</select>

								{#if quotaMode === 'custom'}
									<input
										bind:value={quotaInput}
										type="number"
										min="0"
										step="1"
										class="w-full rounded-md border border-zinc-200 bg-transparent px-3 py-2 text-sm text-zinc-900 outline-none transition focus:border-cyan-500 focus:ring-1 focus:ring-cyan-500 dark:border-zinc-800 dark:text-zinc-100 dark:focus:border-cyan-500"
										placeholder="0"
									/>
								{/if}
							</div>
						</div>
					</div>

					<div class="flex items-center justify-end gap-3 pt-6">
						<button
							type="button"
							class="inline-flex items-center justify-center rounded-md px-4 py-2 text-sm font-medium text-zinc-500 transition hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100"
							onclick={() => goto('/admin/users')}
						>
							{m.common_cancel()}
						</button>
						<button
							type="submit"
							class="inline-flex items-center justify-center rounded-md bg-cyan-600 px-6 py-2 text-sm font-medium text-white transition hover:bg-cyan-500 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-cyan-600 dark:hover:bg-cyan-500"
							disabled={saving}
						>
							{saving ? m.common_loading() : m.admin_users_quota_save()}
						</button>
					</div>
				</div>
			</form>
		</div>
	{/if}
</section>
