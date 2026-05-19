<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { toast } from 'svelte-sonner';
	import * as m from '$paraglide/messages';
	import UserAvatar from '$lib/components/common/UserAvatar.svelte';
	import ConfirmDialog from '$lib/components/common/ConfirmDialog.svelte';
	import ModalDialog from '$lib/components/common/ModalDialog.svelte';
	import { auth } from '$lib/stores/auth';
	import {
		getAdminUser,
		listAdminUserSessions,
		revokeAdminUserSession,
		updateAdminUserEmail,
		updateAdminUserDocumentQuota,
		verifyAdminUserEmail,
		purgeAdminUserMedia,
		purgeAdminUserDocuments,
		unregisterAdminUser,
		type AdminUserListItem,
		type AdminUserSession
	} from '$lib/api/admin';
	import ArrowLeft from '~icons/ph/arrow-left';
	import Files from '~icons/ph/files';
	import Trash from '~icons/ph/trash';
	import HardDrive from '~icons/ph/hard-drive';
	import UserMinus from '~icons/ph/user-minus';
	import CloudArrowDown from '~icons/ph/cloud-arrow-down';
	import Devices from '~icons/ph/devices';
	import ImageSquare from '~icons/ph/image-square';
	import CaretRight from '~icons/ph/caret-right';

	type QuotaMode = 'inherit' | 'custom' | 'unlimited';
	const SESSION_PAGE_SIZE = 5;

	let item = $state<AdminUserListItem | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let actionLoading = $state(false);
	let errorMessage = $state('');
	let quotaMode = $state<QuotaMode>('inherit');
	let quotaInput = $state('');
	let emailInput = $state('');
	let sessions = $state<AdminUserSession[]>([]);
	let sessionsLoading = $state(false);
	let sessionsError = $state('');
	let sessionsOpen = $state(false);
	let sessionsModalItems = $state<AdminUserSession[]>([]);
	let sessionsModalLoading = $state(false);
	let sessionsModalTotal = $state(0);
	let sessionsModalPage = $state(1);
	let savingEmail = $state(false);
	let verifyingEmail = $state(false);
	let revokingSessionId = $state('');

	// Action confirmation states
	let showPurgeMediaConfirm = $state(false);
	let showPurgeDocsConfirm = $state(false);
	let showUnregisterConfirm = $state(false);
	const isSelf = $derived(Boolean(item && $auth.user?.id === item.id));
	const sessionsModalPageCount = $derived(Math.max(1, Math.ceil(sessionsModalTotal / SESSION_PAGE_SIZE)));
	const sessionsModalPages = $derived(buildSessionPageItems(sessionsModalPage, sessionsModalPageCount));

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
			emailInput = detail.email ?? '';
			await loadSessionsSummary(userID);
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

	function formatDateTime(value: string): string {
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return value;
		return date.toLocaleString();
	}

	function sessionLabel(session: AdminUserSession): string {
		const label = session.deviceLabel.trim();
		if (label) return label;
		const agent = session.userAgent.trim();
		if (!agent) return m.admin_user_sessions_unknown_device();
		return agent.length > 48 ? `${agent.slice(0, 48)}...` : agent;
	}

	function buildSessionPageItems(currentPage: number, pageCount: number): Array<number | string> {
		if (pageCount <= 7) {
			return Array.from({ length: pageCount }, (_, index) => index + 1);
		}
		if (currentPage <= 4) {
			return [1, 2, 3, 4, 5, 'ellipsis-right', pageCount];
		}
		if (currentPage >= pageCount - 3) {
			return [1, 'ellipsis-left', pageCount - 4, pageCount - 3, pageCount - 2, pageCount - 1, pageCount];
		}
		return [1, 'ellipsis-left', currentPage - 1, currentPage, currentPage + 1, 'ellipsis-right', pageCount];
	}

	async function loadSessionsSummary(userID: string) {
		sessionsLoading = true;
		sessionsError = '';
		try {
			const payload = await listAdminUserSessions({ userID, limit: 5, offset: 0 });
			sessions = payload.items;
			sessionsModalTotal = payload.total;
		} catch (error) {
			sessionsError = error instanceof Error ? error.message : m.admin_user_sessions_load_failed();
			sessions = [];
			sessionsModalTotal = 0;
		} finally {
			sessionsLoading = false;
		}
	}

	async function openSessionsModal() {
		if (!item) return;
		sessionsOpen = true;
		await loadSessionsPage(1);
	}

	async function loadSessionsPage(page: number) {
		if (!item || sessionsModalLoading) return;
		const safePage = Math.max(1, page);
		sessionsModalLoading = true;
		try {
			const payload = await listAdminUserSessions({
				userID: item.id,
				limit: SESSION_PAGE_SIZE,
				offset: (safePage - 1) * SESSION_PAGE_SIZE
			});
			sessionsModalItems = payload.items;
			sessionsModalPage = safePage;
			sessionsModalTotal = payload.total;
		} catch (error) {
			toast.error(error instanceof Error ? error.message : m.admin_user_sessions_load_failed());
		} finally {
			sessionsModalLoading = false;
		}
	}

	async function handleRevokeSession(session: AdminUserSession) {
		if (!item || revokingSessionId) return;
		revokingSessionId = session.id;
		try {
			await revokeAdminUserSession(item.id, session.id);
			sessions = sessions.filter((entry) => entry.id !== session.id);
			sessionsModalItems = sessionsModalItems.filter((entry) => entry.id !== session.id);
			sessionsModalTotal = Math.max(0, sessionsModalTotal - 1);
			toast.success(m.admin_user_sessions_revoke_success());
			await loadSessionsSummary(item.id);
			if (sessionsOpen) {
				const nextPage = Math.min(sessionsModalPage, Math.max(1, Math.ceil(sessionsModalTotal / SESSION_PAGE_SIZE)));
				await loadSessionsPage(nextPage);
			}
		} catch (error) {
			toast.error(error instanceof Error ? error.message : m.admin_user_sessions_revoke_failed());
		} finally {
			revokingSessionId = '';
		}
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
			const updated = await updateAdminUserDocumentQuota(item.id, {
				documentQuotaMode: quotaMode,
				documentQuota: quotaValue
			});
			item = updated;
			quotaMode = normalizeMode(updated.documentQuotaMode);
			quotaInput = updated.documentQuota === null ? '' : String(updated.documentQuota);
			toast.success(m.admin_users_quota_saved());
		} catch (error) {
			toast.error(error instanceof Error ? error.message : m.admin_users_quota_save_failed());
		} finally {
			saving = false;
		}
	}

	async function handlePurgeMedia() {
		if (!item) return;
		actionLoading = true;
		try {
			await purgeAdminUserMedia(item.id);
			toast.success(m.admin_users_purge_media_success());
			showPurgeMediaConfirm = false;
		} catch (error) {
			toast.error(error instanceof Error ? error.message : m.admin_users_purge_media_failed());
		} finally {
			actionLoading = false;
		}
	}

	async function handlePurgeDocuments() {
		if (!item) return;
		actionLoading = true;
		try {
			await purgeAdminUserDocuments(item.id);
			toast.success(m.admin_users_purge_docs_success());
			showPurgeDocsConfirm = false;
			await loadUser(); // Refresh counts
		} catch (error) {
			toast.error(error instanceof Error ? error.message : m.admin_users_purge_docs_failed());
		} finally {
			actionLoading = false;
		}
	}

	async function handleUnregister() {
		if (!item || actionLoading || isSelf) return;
		actionLoading = true;
		try {
			await unregisterAdminUser(item.id);
			toast.success(m.admin_users_unregister_success());
			showUnregisterConfirm = false;
			await goto('/admin/users');
		} catch (error) {
			toast.error(error instanceof Error ? error.message : m.admin_users_unregister_failed());
		} finally {
			actionLoading = false;
		}
	}

	function closeSessionsModal() {
		sessionsOpen = false;
	}

	async function handleEmailSave(event: SubmitEvent) {
		event.preventDefault();
		if (!item) return;

		savingEmail = true;
		try {
			const updated = await updateAdminUserEmail(item.id, { email: emailInput });
			item = updated;
			emailInput = updated.email ?? '';
			toast.success(m.admin_user_account_email_saved());
		} catch (error) {
			toast.error(error instanceof Error ? error.message : m.admin_user_account_email_save_failed());
		} finally {
			savingEmail = false;
		}
	}

	async function handleVerifyEmail() {
		if (!item) return;
		verifyingEmail = true;
		try {
			const updated = await verifyAdminUserEmail(item.id);
			item = updated;
			emailInput = updated.email ?? '';
			toast.success(m.admin_user_account_email_verified_success());
		} catch (error) {
			toast.error(error instanceof Error ? error.message : m.admin_user_account_email_verified_failed());
		} finally {
			verifyingEmail = false;
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

					<div class="max-w-3xl">
						<div class="grid gap-6 sm:grid-cols-[minmax(0,180px)_minmax(0,1fr)] sm:items-start">
							<label for="quota-mode" class="text-sm font-medium text-zinc-600 dark:text-zinc-400">
								{m.admin_users_quota_title()}
							</label>

							<div class="space-y-4">
								<div class="flex flex-col gap-3 sm:flex-row sm:items-center">
									<select
										id="quota-mode"
										bind:value={quotaMode}
										class="min-w-0 flex-1 rounded-md border border-zinc-200 bg-transparent px-3 py-2 text-sm text-zinc-900 outline-none transition focus:border-cyan-500 focus:ring-1 focus:ring-cyan-500 dark:border-zinc-800 dark:text-zinc-100 dark:focus:border-cyan-500"
									>
										<option value="inherit">{m.admin_quota_inherit()}</option>
										<option value="custom">{m.admin_quota_custom()}</option>
										<option value="unlimited">{m.admin_quota_unlimited()}</option>
									</select>

									<button
										type="submit"
										class="inline-flex items-center justify-center whitespace-nowrap rounded-md bg-cyan-600 px-5 py-2 text-sm font-medium text-white transition hover:bg-cyan-500 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-cyan-600 dark:hover:bg-cyan-500"
										disabled={saving}
									>
										{saving ? m.common_loading() : m.admin_users_quota_save()}
									</button>
								</div>

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
				</div>
			</form>

			<div class="space-y-8 border-t border-zinc-200 pt-10 dark:border-zinc-800">
				<div class="space-y-1">
					<h2 class="text-lg font-semibold text-zinc-900 dark:text-zinc-100">{m.admin_user_sessions_title()}</h2>
					<p class="text-sm text-zinc-500 dark:text-zinc-400">{m.admin_user_sessions_description()}</p>
				</div>

				{#if sessionsLoading}
					<p class="text-sm text-zinc-500 dark:text-zinc-400">{m.common_loading()}</p>
				{:else if sessionsError}
					<p class="text-sm text-rose-600 dark:text-rose-300">{sessionsError}</p>
				{:else}
					<div class="space-y-3">
						{#if sessions.length === 0}
							<p class="text-sm text-zinc-500 dark:text-zinc-400">{m.admin_user_sessions_empty()}</p>
						{:else}
							{#each sessions as session (session.id)}
								<div class="flex items-start justify-between gap-4 border-b border-zinc-100 pb-3 dark:border-zinc-800/70">
									<div class="min-w-0">
										<div class="flex items-center gap-2 text-zinc-900 dark:text-zinc-100">
											<Devices class="h-4 w-4 text-zinc-400 dark:text-zinc-500" />
											<p class="truncate text-sm font-medium">{sessionLabel(session)}</p>
										</div>
										<p class="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{formatDateTime(session.lastSeenAt)}</p>
									</div>
									<button
										type="button"
										class="shrink-0 rounded-md border border-zinc-200 px-3 py-1.5 text-xs font-medium text-zinc-600 transition hover:bg-zinc-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
										disabled={revokingSessionId === session.id}
										onclick={() => void handleRevokeSession(session)}
									>
										{revokingSessionId === session.id ? m.common_loading() : m.admin_user_sessions_revoke_action()}
									</button>
								</div>
							{/each}
						{/if}

						{#if sessionsModalTotal > 5}
							<button
								type="button"
								class="inline-flex items-center gap-1 text-sm font-medium text-cyan-700 transition hover:text-cyan-600 dark:text-cyan-300 dark:hover:text-cyan-200"
								onclick={() => void openSessionsModal()}
							>
								{m.admin_user_sessions_more({ count: sessionsModalTotal })}
								<CaretRight class="h-4 w-4" />
							</button>
						{/if}
					</div>
				{/if}
			</div>

			<div class="space-y-8 border-t border-zinc-200 pt-10 dark:border-zinc-800">
				<div class="flex items-start justify-between gap-4">
					<div class="space-y-1">
						<h2 class="text-lg font-semibold text-zinc-900 dark:text-zinc-100">{m.admin_user_media_title()}</h2>
						<p class="text-sm text-zinc-500 dark:text-zinc-400">{m.admin_user_media_description()}</p>
					</div>
					<button
						type="button"
						class="inline-flex items-center gap-2 rounded-md border border-zinc-200 px-4 py-2 text-sm font-medium text-zinc-700 transition hover:bg-zinc-50 dark:border-zinc-700 dark:text-zinc-200 dark:hover:bg-zinc-800"
						onclick={() => item && goto(`/admin/users/${item.id}/media`)}
					>
						<ImageSquare class="h-4 w-4" />
						{m.admin_user_media_open()}
					</button>
				</div>
			</div>

			<div class="space-y-8 border-t border-zinc-200 pt-10 dark:border-zinc-800">
				<div class="space-y-1">
					<h2 class="text-lg font-semibold text-zinc-900 dark:text-zinc-100">{m.admin_user_account_title()}</h2>
					<p class="text-sm text-zinc-500 dark:text-zinc-400">{m.admin_user_account_description()}</p>
				</div>

				<div class="space-y-6">
					<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
						<div class="space-y-1 sm:w-1/3">
							<h3 class="text-base font-medium text-zinc-900 dark:text-zinc-100">{m.user_profile_email_title()}</h3>
							<p class="text-xs text-zinc-500 dark:text-zinc-400">{m.admin_user_account_email_hint()}</p>
						</div>
						<form class="flex w-full flex-1 gap-3 sm:max-w-md" onsubmit={handleEmailSave}>
							<input
								bind:value={emailInput}
								type="email"
								class="w-full flex-1 rounded-lg border border-zinc-200 bg-transparent px-4 py-2 text-sm text-zinc-900 outline-none transition focus:border-sky-300 focus:ring-2 focus:ring-sky-100 dark:border-zinc-800 dark:text-zinc-100 dark:focus:border-sky-400 dark:focus:ring-sky-950/40"
								placeholder="user@example.com"
							/>
							<button
								type="submit"
								class="shrink-0 rounded-lg bg-sky-500 px-5 py-2 text-sm font-medium text-white shadow-sm transition hover:bg-sky-600 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-sky-500 dark:hover:bg-sky-400"
								disabled={savingEmail}
							>
								{savingEmail ? m.common_saving() : m.common_save()}
							</button>
						</form>
					</div>

					<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
						<div class="space-y-1 sm:w-1/3">
							<h3 class="flex items-center gap-2 text-base font-medium text-zinc-900 dark:text-zinc-100">
								{m.admin_user_account_email_verify_title()}
								{#if item.emailVerified}
									<span class="rounded-full bg-emerald-100 px-2 py-0.5 text-[11px] font-medium text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300">
										{m.admin_user_account_email_verified_badge()}
									</span>
								{:else}
									<span class="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-medium text-amber-700 dark:bg-amber-950/40 dark:text-amber-300">
										{m.admin_user_account_email_unverified_badge()}
									</span>
								{/if}
							</h3>
							<p class="text-xs text-zinc-500 dark:text-zinc-400">{m.admin_user_account_email_verify_hint()}</p>
						</div>
						<div class="flex w-full flex-1 justify-end gap-3 sm:max-w-md">
							<button
								type="button"
								class="shrink-0 rounded-lg border border-zinc-200 px-5 py-2 text-sm font-medium text-zinc-700 transition hover:bg-zinc-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-zinc-700 dark:text-zinc-200 dark:hover:bg-zinc-800"
								disabled={verifyingEmail || item.emailVerified || !item.email}
								onclick={() => void handleVerifyEmail()}
							>
								{verifyingEmail ? m.common_loading() : m.admin_user_account_email_verify_action()}
							</button>
						</div>
					</div>
				</div>
			</div>

			<div class="mt-20 space-y-8">
				<div class="space-y-1">
					<h2 class="text-sm font-bold uppercase tracking-widest text-zinc-400 dark:text-zinc-500">
						{m.admin_users_danger_zone()}
					</h2>
				</div>

				<div class="space-y-6">
					<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
						<div class="space-y-1">
							<div class="flex items-center gap-2 text-zinc-900 dark:text-zinc-100">
								<CloudArrowDown class="h-4 w-4" />
								<p class="text-sm font-semibold">{m.admin_users_purge_media_title()}</p>
							</div>
							<p class="text-xs text-zinc-500 dark:text-zinc-400">
								{m.admin_users_purge_media_hint()}
							</p>
						</div>
						<button
							type="button"
							class="inline-flex h-9 items-center justify-center rounded-md border border-zinc-200 bg-transparent px-4 text-xs font-medium text-zinc-600 transition hover:border-rose-200 hover:bg-rose-50 hover:text-rose-600 dark:border-zinc-800 dark:text-zinc-400 dark:hover:border-rose-900/50 dark:hover:bg-rose-950/20 dark:hover:text-rose-400"
							onclick={() => (showPurgeMediaConfirm = true)}
							disabled={actionLoading}
						>
							{m.admin_users_purge_media_btn()}
						</button>
					</div>

					<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
						<div class="space-y-1">
							<div class="flex items-center gap-2 text-zinc-900 dark:text-zinc-100">
								<Trash class="h-4 w-4" />
								<p class="text-sm font-semibold">{m.admin_users_purge_docs_title()}</p>
							</div>
							<p class="text-xs text-zinc-500 dark:text-zinc-400">
								{m.admin_users_purge_docs_hint()}
							</p>
						</div>
						<button
							type="button"
							class="inline-flex h-9 items-center justify-center rounded-md border border-zinc-200 bg-transparent px-4 text-xs font-medium text-zinc-600 transition hover:border-rose-200 hover:bg-rose-50 hover:text-rose-600 dark:border-zinc-800 dark:text-zinc-400 dark:hover:border-rose-900/50 dark:hover:bg-rose-950/20 dark:hover:text-rose-400"
							onclick={() => (showPurgeDocsConfirm = true)}
							disabled={actionLoading}
						>
							{m.admin_users_purge_docs_btn()}
						</button>
					</div>

					<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
						<div class="space-y-1">
							<div class="flex items-center gap-2 text-rose-600 dark:text-rose-400">
								<UserMinus class="h-4 w-4" />
								<p class="text-sm font-semibold">{m.admin_users_unregister_title()}</p>
							</div>
							<p class="text-xs text-zinc-500 dark:text-zinc-400">
								{m.admin_users_unregister_hint()}
							</p>
						</div>
						<button
							type="button"
							class="inline-flex h-9 items-center justify-center rounded-md border border-rose-200 bg-rose-50 px-4 text-xs font-medium text-rose-600 transition hover:bg-rose-100 dark:border-rose-900/50 dark:bg-rose-950/20 dark:text-rose-400 dark:hover:bg-rose-900/40"
							onclick={() => (showUnregisterConfirm = true)}
							disabled={actionLoading || isSelf}
						>
							{m.admin_users_unregister_btn()}
						</button>
					</div>
				</div>
			</div>
		</div>
	{/if}
</section>

<ModalDialog
	open={sessionsOpen}
	title={m.admin_user_sessions_modal_title()}
	maxWidthClass="max-w-2xl"
	onClose={closeSessionsModal}
>
	<div class="space-y-4">
		<div class="flex items-center justify-between gap-3">
			<h3 class="text-base font-semibold text-zinc-900 dark:text-zinc-100">{m.admin_user_sessions_modal_title()}</h3>
			<button
				type="button"
				class="text-sm text-zinc-500 transition hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100"
				onclick={closeSessionsModal}
			>
				{m.common_cancel()}
			</button>
		</div>

		{#if sessionsModalItems.length === 0 && sessionsModalLoading}
			<p class="text-sm text-zinc-500 dark:text-zinc-400">{m.common_loading()}</p>
		{:else}
			<div class="max-h-[60vh] space-y-3 overflow-y-auto pr-1">
				{#each sessionsModalItems as session (session.id)}
					<div class="flex items-start justify-between gap-4 border-b border-zinc-100 pb-3 dark:border-zinc-800/70">
						<div class="min-w-0">
							<p class="text-sm font-medium text-zinc-900 dark:text-zinc-100">{sessionLabel(session)}</p>
							<p class="mt-1 truncate text-xs text-zinc-500 dark:text-zinc-400">{session.userAgent || m.admin_user_sessions_unknown_device()}</p>
							<p class="mt-1 text-xs text-zinc-500 dark:text-zinc-400">
								{m.admin_user_sessions_last_seen()}：{formatDateTime(session.lastSeenAt)}
							</p>
						</div>
						<button
							type="button"
							class="shrink-0 rounded-md border border-zinc-200 px-3 py-1.5 text-xs font-medium text-zinc-600 transition hover:bg-zinc-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
							disabled={revokingSessionId === session.id}
							onclick={() => void handleRevokeSession(session)}
						>
							{revokingSessionId === session.id ? m.common_loading() : m.admin_user_sessions_revoke_action()}
						</button>
					</div>
				{/each}

			</div>
		{/if}

		<div class="flex flex-col gap-3 pt-1 sm:flex-row sm:items-center sm:justify-between">
			<p class="text-xs text-zinc-500 dark:text-zinc-400">
				{m.admin_user_sessions_visible_count({
					shown: String((sessionsModalPage - 1) * SESSION_PAGE_SIZE + sessionsModalItems.length),
					total: String(sessionsModalTotal)
				})}
			</p>
			<div class="flex flex-wrap gap-1.5">
				{#each sessionsModalPages as pageItem (pageItem)}
					{#if typeof pageItem === 'number'}
						<button
							type="button"
							class={`grid h-8 min-w-8 place-items-center rounded-md border px-2 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-60 ${
								pageItem === sessionsModalPage
									? 'border-cyan-500 bg-cyan-600 text-white dark:border-cyan-500 dark:bg-cyan-600'
									: 'border-zinc-200 text-zinc-700 hover:bg-zinc-50 dark:border-zinc-700 dark:text-zinc-200 dark:hover:bg-zinc-800'
							}`}
							disabled={sessionsModalLoading || pageItem === sessionsModalPage}
							onclick={() => void loadSessionsPage(pageItem)}
							aria-label={`Page ${pageItem}`}
						>
							{pageItem}
						</button>
					{:else}
						<span class="grid h-8 min-w-8 place-items-center px-1 text-sm text-zinc-400 dark:text-zinc-500">...</span>
					{/if}
				{/each}
			</div>
		</div>
	</div>
</ModalDialog>

<ConfirmDialog
	open={showPurgeMediaConfirm}
	title={m.admin_users_purge_media_confirm_title()}
	message={m.admin_users_purge_media_confirm_msg()}
	confirmText={m.admin_users_purge_media_confirm_btn()}
	loading={actionLoading}
	onConfirm={handlePurgeMedia}
	onCancel={() => (showPurgeMediaConfirm = false)}
/>

<ConfirmDialog
	open={showPurgeDocsConfirm}
	title={m.admin_users_purge_docs_confirm_title()}
	message={m.admin_users_purge_docs_confirm_msg()}
	confirmText={m.admin_users_purge_docs_confirm_btn()}
	loading={actionLoading}
	onConfirm={handlePurgeDocuments}
	onCancel={() => (showPurgeDocsConfirm = false)}
/>

<ConfirmDialog
	open={showUnregisterConfirm}
	title={m.admin_users_unregister_confirm_title()}
	message={m.admin_users_unregister_confirm_msg()}
	confirmText={m.admin_users_unregister_confirm_btn()}
	loading={actionLoading}
	disabled={isSelf}
	onConfirm={handleUnregister}
	onCancel={() => (showUnregisterConfirm = false)}
/>
