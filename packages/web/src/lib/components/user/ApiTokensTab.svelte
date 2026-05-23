<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import * as m from '$paraglide/messages';
	import PencilSimple from '~icons/ph/pencil-simple';
	import Broadcast from '~icons/ph/broadcast';
	import PlugsConnected from '~icons/ph/plugs-connected';
	import PuzzlePiece from '~icons/ph/puzzle-piece';
	import Trash from '~icons/ph/trash';
	import { apiBaseUrl } from '$lib/config/api';
	import ConfirmDialog from '$lib/components/common/ConfirmDialog.svelte';
	import {
		createApiToken,
		deleteRevokedApiToken,
		getApiTokens,
		revokeApiToken,
		updateApiToken,
		type ApiToken,
		type ApiTokenScope
	} from '$lib/api/user';

	const scopeOptions: { value: ApiTokenScope; label: string }[] = [
		{ value: 'workspace:read', label: 'workspace:read' },
		{ value: 'workspace:write', label: 'workspace:write' },
		{ value: 'document:read', label: 'document:read' },
		{ value: 'document:write', label: 'document:write' },
		{ value: 'file:move', label: 'file:move' },
		{ value: 'file:copy', label: 'file:copy' },
		{ value: 'file:delete', label: 'file:delete' }
	];

	const readScopes: ApiTokenScope[] = ['workspace:read', 'document:read'];
	const workspaceEditScopes: ApiTokenScope[] = [
		'workspace:read',
		'workspace:write',
		'document:read',
		'document:write',
		'file:move',
		'file:copy'
	];
	const skillPagePath = '/skill.md';
	const mcpEndpoint = `${apiBaseUrl}/api/v1/mcp`;
	const tokenPlaceholder = '<CYIME_API_TOKEN>';

	let items = $state<ApiToken[]>([]);
	let loading = $state(true);
	let creating = $state(false);
	let revokingId = $state('');
	let deletingId = $state('');
	let updatingId = $state('');
	let name = $state('');
	let expiry = $state('30');
	let selectedScopes = $state<ApiTokenScope[]>([...workspaceEditScopes]);
	let createdToken = $state('');
	let editingToken = $state<ApiToken | null>(null);
	let revokeTarget = $state<ApiToken | null>(null);
	let deleteRecordTarget = $state<ApiToken | null>(null);
	let editName = $state('');
	let editableScopes = $state<ApiTokenScope[]>([]);
	let frontendOrigin = $state('');
	const skillPageUrl = $derived(frontendOrigin ? `${frontendOrigin}${skillPagePath}` : skillPagePath);

	onMount(() => {
		frontendOrigin = window.location.origin;
		void loadTokens();
	});

	async function loadTokens() {
		loading = true;
		try {
			items = await getApiTokens();
		} catch (error) {
			toast.error(error instanceof Error ? error.message : m.user_api_tokens_load_failed());
		} finally {
			loading = false;
		}
	}

	function toggleScope(scope: ApiTokenScope) {
		selectedScopes = selectedScopes.includes(scope)
			? selectedScopes.filter((item) => item !== scope)
			: [...selectedScopes, scope];
	}

	function toggleEditScope(scope: ApiTokenScope) {
		editableScopes = editableScopes.includes(scope)
			? editableScopes.filter((item) => item !== scope)
			: [...editableScopes, scope];
	}

	function buildExpiresAt(): string | null {
		if (expiry === 'never') return null;
		const days = Number(expiry);
		if (!Number.isFinite(days) || days <= 0) return null;
		const expiresAt = new Date();
		expiresAt.setDate(expiresAt.getDate() + days);
		return expiresAt.toISOString();
	}

	function buildMcpServerMapConfig(tokenValue: string): string {
		const rawToken = tokenValue.trim() || tokenPlaceholder;
		return JSON.stringify(
			{
				mcpServers: {
					'cyime-workspace': {
						type: 'http',
						url: mcpEndpoint,
						headers: {
							Authorization: `Bearer ${rawToken}`
						}
					}
				}
			},
			null,
			2
		);
	}

	function buildStreamableHttpMcpConfig(tokenValue: string): string {
		const rawToken = tokenValue.trim() || tokenPlaceholder;
		return JSON.stringify(
			{
				transport: 'streamable_http',
				url: mcpEndpoint,
				headers: {
					Authorization: `Bearer ${rawToken}`
				},
				timeout: 5,
				sse_read_timeout: 300
			},
			null,
			2
		);
	}

	async function copyMcpServerMapConfig(
		tokenValue = createdToken,
		successMessage = m.user_api_tokens_server_map_mcp_copied()
	) {
		if (!tokenValue.trim()) return;
		try {
			await navigator.clipboard.writeText(buildMcpServerMapConfig(tokenValue));
			toast.success(successMessage);
		} catch {
			toast.error(m.user_api_tokens_mcp_copy_failed());
		}
	}

	async function copyStreamableHttpMcpConfig(
		tokenValue = createdToken,
		successMessage = m.user_api_tokens_streamable_http_mcp_copied()
	) {
		if (!tokenValue.trim()) return;
		try {
			await navigator.clipboard.writeText(buildStreamableHttpMcpConfig(tokenValue));
			toast.success(successMessage);
		} catch {
			toast.error(m.user_api_tokens_mcp_copy_failed());
		}
	}

	async function copySkillImportPrompt() {
		const prompt = m.user_api_tokens_skill_import_prompt({ url: skillPageUrl });
		try {
			await navigator.clipboard.writeText(prompt);
			toast.success(m.user_api_tokens_skill_prompt_copied());
		} catch {
			toast.error(m.user_api_tokens_skill_prompt_copy_failed());
		}
	}

	async function handleCreate() {
		if (creating) return;
		if (selectedScopes.length === 0) {
			toast.error(m.user_api_tokens_scope_required());
			return;
		}

		creating = true;
		createdToken = '';
		try {
			const created = await createApiToken({
				name,
				scopes: selectedScopes,
				expiresAt: buildExpiresAt()
			});
			createdToken = created.token;
			name = '';
			selectedScopes = [...workspaceEditScopes];
			expiry = '30';
			await loadTokens();
			toast.success(m.user_api_tokens_created());
		} catch (error) {
			toast.error(error instanceof Error ? error.message : m.user_api_tokens_create_failed());
		} finally {
			creating = false;
		}
	}

	function openRevokeConfirm(item: ApiToken) {
		revokeTarget = item;
	}

	function closeRevokeConfirm() {
		if (revokingId) return;
		revokeTarget = null;
	}

	async function confirmRevoke() {
		if (!revokeTarget) return;
		const target = revokeTarget;
		revokingId = target.id;
		try {
			await revokeApiToken(target.id);
			items = items.map((token) =>
				token.id === target.id ? { ...token, revokedAt: new Date().toISOString() } : token
			);
			toast.success(m.user_api_tokens_revoked());
			revokeTarget = null;
		} catch (error) {
			toast.error(error instanceof Error ? error.message : m.user_api_tokens_revoke_failed());
		} finally {
			revokingId = '';
		}
	}

	function openDeleteRecordConfirm(item: ApiToken) {
		deleteRecordTarget = item;
	}

	function closeDeleteRecordConfirm() {
		if (deletingId) return;
		deleteRecordTarget = null;
	}

	async function confirmDeleteRecord() {
		if (!deleteRecordTarget) return;
		const target = deleteRecordTarget;
		deletingId = target.id;
		try {
			await deleteRevokedApiToken(target.id);
			items = items.filter((token) => token.id !== target.id);
			toast.success(m.user_api_tokens_record_deleted());
			deleteRecordTarget = null;
		} catch (error) {
			toast.error(error instanceof Error ? error.message : m.user_api_tokens_record_delete_failed());
		} finally {
			deletingId = '';
		}
	}

	function openEditDialog(item: ApiToken) {
		editingToken = item;
		editName = item.name;
		editableScopes = [...item.scopes];
	}

	function closeEditDialog() {
		if (updatingId) return;
		editingToken = null;
		editName = '';
		editableScopes = [];
	}

	async function handleUpdate() {
		if (!editingToken || updatingId) return;
		if (editableScopes.length === 0) {
			toast.error(m.user_api_tokens_scope_required());
			return;
		}

		updatingId = editingToken.id;
		try {
			const updated = await updateApiToken(editingToken.id, {
				name: editName,
				scopes: editableScopes
			});
			items = items.map((token) => (token.id === updated.id ? updated : token));
			toast.success(m.user_api_tokens_updated());
			closeEditDialog();
		} catch (error) {
			toast.error(error instanceof Error ? error.message : m.user_api_tokens_update_failed());
		} finally {
			updatingId = '';
		}
	}

	async function copyCreatedToken() {
		if (!createdToken) return;
		try {
			await navigator.clipboard.writeText(createdToken);
			toast.success(m.user_api_tokens_copied());
		} catch {
			toast.error(m.user_api_tokens_copy_failed());
		}
	}

	function formatDateTime(value?: string | null): string {
		if (!value) return m.user_api_tokens_never();
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return value;
		return date.toLocaleString();
	}
</script>

<div class="space-y-6">
	<section class="space-y-4">
		<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
			<div>
				<h2 class="text-base font-semibold text-zinc-900 dark:text-zinc-100">
					{m.user_api_tokens_create_title()}
				</h2>
				<p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">
					{m.user_api_tokens_create_description()}
				</p>
			</div>
			<div class="flex items-center gap-2">
				<span class="text-xs font-medium text-zinc-500 dark:text-zinc-400">
					{m.user_api_tokens_import_to()}
				</span>
				<span
					class="inline-flex"
					title={createdToken
						? m.user_api_tokens_server_map_mcp_import_description()
						: m.user_api_tokens_create_before_copy_hint()}
				>
					<button
						type="button"
						class="inline-flex h-9 w-9 items-center justify-center rounded-md text-cyan-700 transition hover:bg-cyan-50 disabled:cursor-not-allowed disabled:opacity-40 dark:text-cyan-300 dark:hover:bg-cyan-950/30"
						disabled={!createdToken}
						aria-label={createdToken
							? m.user_api_tokens_server_map_mcp_import_description()
							: m.user_api_tokens_create_before_copy_hint()}
						onclick={() => copyMcpServerMapConfig()}
					>
						<PlugsConnected class="h-4 w-4 shrink-0" />
					</button>
				</span>
				<span
					class="inline-flex"
					title={createdToken
						? m.user_api_tokens_streamable_http_mcp_import_description()
						: m.user_api_tokens_create_before_copy_hint()}
				>
					<button
						type="button"
						class="inline-flex h-9 w-9 items-center justify-center rounded-md text-cyan-700 transition hover:bg-cyan-50 disabled:cursor-not-allowed disabled:opacity-40 dark:text-cyan-300 dark:hover:bg-cyan-950/30"
						disabled={!createdToken}
						aria-label={createdToken
							? m.user_api_tokens_streamable_http_mcp_import_description()
							: m.user_api_tokens_create_before_copy_hint()}
						onclick={() => copyStreamableHttpMcpConfig()}
					>
						<Broadcast class="h-4 w-4 shrink-0" />
					</button>
				</span>
				<button
					type="button"
					class="inline-flex h-9 w-9 items-center justify-center rounded-md text-zinc-700 transition hover:bg-zinc-50 dark:text-zinc-200 dark:hover:bg-zinc-800"
					aria-label={m.user_api_tokens_skill_page_action()}
					title={m.user_api_tokens_skill_page_action()}
					onclick={copySkillImportPrompt}
				>
					<PuzzlePiece class="h-4 w-4 shrink-0" />
				</button>
			</div>
		</div>

		<div class="grid gap-4 md:grid-cols-[minmax(0,1fr)_180px]">
			<label class="space-y-1">
				<span class="text-sm font-medium text-zinc-900 dark:text-zinc-100">
					{m.user_api_tokens_name_label()}
				</span>
				<input
					class="w-full rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900 outline-none transition focus:border-cyan-500 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
					bind:value={name}
					placeholder={m.user_api_tokens_name_placeholder()}
				/>
			</label>
			<label class="space-y-1">
				<span class="text-sm font-medium text-zinc-900 dark:text-zinc-100">
					{m.user_api_tokens_expiry_label()}
				</span>
				<select
					class="w-full rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900 outline-none transition focus:border-cyan-500 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
					bind:value={expiry}
				>
					<option value="7">{m.user_api_tokens_expiry_7d()}</option>
					<option value="30">{m.user_api_tokens_expiry_30d()}</option>
					<option value="90">{m.user_api_tokens_expiry_90d()}</option>
					<option value="never">{m.user_api_tokens_expiry_never()}</option>
				</select>
			</label>
		</div>

		<div class="space-y-2">
			<div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
				<p class="text-sm font-medium text-zinc-900 dark:text-zinc-100">
					{m.user_api_tokens_scopes_label()}
				</p>
				<div class="flex flex-wrap gap-2">
					<button
						type="button"
						class="rounded-lg border border-zinc-200 px-3 py-1.5 text-xs font-medium text-zinc-700 transition hover:bg-zinc-50 dark:border-zinc-700 dark:text-zinc-200 dark:hover:bg-zinc-800"
						onclick={() => (selectedScopes = [...workspaceEditScopes])}
					>
						{m.user_api_tokens_preset_edit()}
					</button>
					<button
						type="button"
						class="rounded-lg border border-zinc-200 px-3 py-1.5 text-xs font-medium text-zinc-700 transition hover:bg-zinc-50 dark:border-zinc-700 dark:text-zinc-200 dark:hover:bg-zinc-800"
						onclick={() => (selectedScopes = [...readScopes])}
					>
						{m.user_api_tokens_preset_readonly()}
					</button>
				</div>
			</div>
			<p class="text-xs text-zinc-500 dark:text-zinc-400">
				{m.user_api_tokens_edit_hint()}
			</p>
			<div class="flex max-w-2xl flex-wrap gap-2">
				{#each scopeOptions as scope (scope.value)}
					{@const selected = selectedScopes.includes(scope.value)}
					<label
						class={`inline-flex cursor-pointer items-center rounded-full border px-3 py-1.5 text-xs font-medium transition ${
							selected
								? 'border-cyan-200 bg-cyan-50 text-cyan-700 dark:border-cyan-900/60 dark:bg-cyan-950/30 dark:text-cyan-200'
								: 'border-zinc-200 bg-white text-zinc-600 hover:border-zinc-300 hover:bg-zinc-50 dark:border-zinc-800 dark:bg-zinc-950 dark:text-zinc-300 dark:hover:border-zinc-700 dark:hover:bg-zinc-900'
						}`}
					>
						<input
							class="sr-only"
							type="checkbox"
							checked={selected}
							onchange={() => toggleScope(scope.value)}
						/>
						<span>{scope.label}</span>
					</label>
				{/each}
			</div>
		</div>

		<div class="flex flex-wrap gap-2">
			<button
				type="button"
				class="rounded-lg bg-cyan-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-cyan-700 disabled:cursor-not-allowed disabled:opacity-60"
				disabled={creating}
				onclick={() => handleCreate()}
			>
				{creating ? m.common_loading() : m.user_api_tokens_create_action()}
			</button>
		</div>

		{#if createdToken}
			<div class="rounded-lg border border-amber-200 bg-amber-50 p-3 dark:border-amber-900/60 dark:bg-amber-950/20">
				<p class="text-sm font-medium text-amber-900 dark:text-amber-100">
					{m.user_api_tokens_created_once_title()}
				</p>
				<div class="mt-2 flex flex-col gap-2 sm:flex-row">
					<code class="min-w-0 flex-1 overflow-x-auto rounded bg-white px-3 py-2 text-xs text-zinc-900 dark:bg-zinc-900 dark:text-zinc-100">
						{createdToken}
					</code>
					<button
						type="button"
						class="rounded-lg border border-amber-300 px-3 py-2 text-sm font-medium text-amber-900 transition hover:bg-amber-100 dark:border-amber-800 dark:text-amber-100 dark:hover:bg-amber-950/40"
						onclick={copyCreatedToken}
					>
						{m.user_api_tokens_copy_action()}
					</button>
					<button
						type="button"
						class="rounded-lg border border-amber-300 px-3 py-2 text-sm font-medium text-amber-900 transition hover:bg-amber-100 dark:border-amber-800 dark:text-amber-100 dark:hover:bg-amber-950/40"
						aria-label={m.user_api_tokens_server_map_mcp_import_description()}
						title={m.user_api_tokens_server_map_mcp_import_description()}
						onclick={() => copyMcpServerMapConfig()}
					>
						{m.user_api_tokens_copy_server_map_mcp_action()}
					</button>
					<button
						type="button"
						class="rounded-lg border border-amber-300 px-3 py-2 text-sm font-medium text-amber-900 transition hover:bg-amber-100 dark:border-amber-800 dark:text-amber-100 dark:hover:bg-amber-950/40"
						aria-label={m.user_api_tokens_streamable_http_mcp_import_description()}
						title={m.user_api_tokens_streamable_http_mcp_import_description()}
						onclick={() => copyStreamableHttpMcpConfig()}
					>
						{m.user_api_tokens_copy_streamable_http_mcp_action()}
					</button>
				</div>
			</div>
		{/if}
	</section>

	<section class="space-y-3">
		<h2 class="text-base font-semibold text-zinc-900 dark:text-zinc-100">
			{m.user_api_tokens_list_title()}
		</h2>
		{#if loading}
			<div class="rounded-xl border border-dashed border-zinc-200 px-4 py-6 text-sm text-zinc-500 dark:border-zinc-700 dark:text-zinc-400">
				{m.common_loading()}
			</div>
		{:else if items.length === 0}
			<div class="rounded-xl border border-dashed border-zinc-200 px-4 py-6 text-sm text-zinc-500 dark:border-zinc-700 dark:text-zinc-400">
				{m.user_api_tokens_empty()}
			</div>
		{:else}
			<div class="space-y-3">
				{#each items as item (item.id)}
					<article class="rounded-xl border border-zinc-200 p-4 dark:border-zinc-800">
						<div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
							<div class="min-w-0 space-y-2">
								<div class="flex flex-wrap items-center gap-2">
									<h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
										{item.name}
									</h3>
									{#if item.revokedAt}
										<span class="rounded-full bg-rose-100 px-2 py-0.5 text-xs font-medium text-rose-700 dark:bg-rose-950/40 dark:text-rose-200">
											{m.user_api_tokens_status_revoked()}
										</span>
									{/if}
								</div>
								<p class="font-mono text-xs text-zinc-500 dark:text-zinc-400">
									{item.tokenPrefix}...
								</p>
								<p class="text-xs text-zinc-500 dark:text-zinc-400">
									{m.user_api_tokens_meta_expires({ time: formatDateTime(item.expiresAt) })}
									·
									{m.user_api_tokens_meta_last_used({ time: formatDateTime(item.lastUsedAt) })}
								</p>
								<div class="flex flex-wrap gap-1">
									{#each item.scopes as scope (scope)}
										<span class="rounded bg-zinc-100 px-2 py-1 text-xs text-zinc-600 dark:bg-zinc-800 dark:text-zinc-300">
											{scope}
										</span>
									{/each}
								</div>
							</div>
							<div class="flex gap-1 md:justify-end">
								{#if item.revokedAt}
									<button
										type="button"
										class="inline-flex h-9 w-9 items-center justify-center rounded-md text-rose-600 transition hover:bg-rose-50 disabled:cursor-not-allowed disabled:opacity-40 dark:text-rose-300 dark:hover:bg-rose-950/20"
										disabled={deletingId === item.id}
										aria-label={m.user_api_tokens_delete_record_action()}
										title={m.user_api_tokens_delete_record_action()}
										onclick={() => openDeleteRecordConfirm(item)}
									>
										<Trash class={`h-4 w-4 shrink-0 ${deletingId === item.id ? 'animate-pulse' : ''}`} />
									</button>
								{:else}
									<button
										type="button"
										class="inline-flex h-9 w-9 items-center justify-center rounded-md text-zinc-500 transition hover:bg-zinc-100 hover:text-zinc-900 disabled:cursor-not-allowed disabled:opacity-40 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
										disabled={updatingId === item.id}
										aria-label={m.user_api_tokens_edit_action()}
										title={m.user_api_tokens_edit_action()}
										onclick={() => openEditDialog(item)}
									>
										<PencilSimple class="h-4 w-4 shrink-0" />
									</button>
									<button
										type="button"
										class="inline-flex h-9 w-9 items-center justify-center rounded-md text-rose-600 transition hover:bg-rose-50 disabled:cursor-not-allowed disabled:opacity-40 dark:text-rose-300 dark:hover:bg-rose-950/20"
										disabled={revokingId === item.id}
										aria-label={m.user_api_tokens_revoke_action()}
										title={m.user_api_tokens_revoke_action()}
										onclick={() => openRevokeConfirm(item)}
									>
										<Trash class={`h-4 w-4 shrink-0 ${revokingId === item.id ? 'animate-pulse' : ''}`} />
									</button>
								{/if}
							</div>
						</div>
					</article>
				{/each}
			</div>
		{/if}
	</section>
</div>

<ConfirmDialog
	open={!!revokeTarget}
	message={revokeTarget ? m.user_api_tokens_confirm_revoke({ name: revokeTarget.name }) : ''}
	confirmText={m.user_api_tokens_revoke_action()}
	loading={!!revokingId}
	onCancel={closeRevokeConfirm}
	onConfirm={confirmRevoke}
/>

<ConfirmDialog
	open={!!deleteRecordTarget}
	message={deleteRecordTarget ? m.user_api_tokens_confirm_delete_record({ name: deleteRecordTarget.name }) : ''}
	confirmText={m.user_api_tokens_delete_record_action()}
	loading={!!deletingId}
	onCancel={closeDeleteRecordConfirm}
	onConfirm={confirmDeleteRecord}
/>

{#if editingToken}
	<div
		class="fixed inset-0 z-[130] flex items-center justify-center bg-black/45 p-4"
		role="presentation"
		onclick={closeEditDialog}
	>
		<div
			class="w-full max-w-xl rounded-xl border border-zinc-200 bg-white shadow-2xl dark:border-zinc-800 dark:bg-zinc-950"
			role="dialog"
			aria-modal="true"
			aria-label={m.user_api_tokens_edit_title()}
			tabindex="-1"
			onclick={(event) => event.stopPropagation()}
			onkeydown={(event) => {
				if (event.key === 'Escape') closeEditDialog();
			}}
		>
			<header class="border-b border-zinc-200 px-5 py-4 dark:border-zinc-800">
				<h3 class="text-base font-semibold text-zinc-900 dark:text-zinc-100">
					{m.user_api_tokens_edit_title()}
				</h3>
				<p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">
					{m.user_api_tokens_edit_description()}
				</p>
			</header>

			<form
				class="space-y-4 p-5"
				onsubmit={(event) => {
					event.preventDefault();
					void handleUpdate();
				}}
			>
				<label class="space-y-1">
					<span class="text-sm font-medium text-zinc-900 dark:text-zinc-100">
						{m.user_api_tokens_name_label()}
					</span>
					<input
						class="w-full rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900 outline-none transition focus:border-cyan-500 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
						bind:value={editName}
						placeholder={m.user_api_tokens_name_placeholder()}
					/>
				</label>

				<div class="space-y-2">
					<div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
						<p class="text-sm font-medium text-zinc-900 dark:text-zinc-100">
							{m.user_api_tokens_scopes_label()}
						</p>
						<div class="flex flex-wrap gap-2">
							<button
								type="button"
								class="rounded-lg border border-zinc-200 px-3 py-1.5 text-xs font-medium text-zinc-700 transition hover:bg-zinc-50 dark:border-zinc-700 dark:text-zinc-200 dark:hover:bg-zinc-800"
								onclick={() => (editableScopes = [...workspaceEditScopes])}
							>
								{m.user_api_tokens_preset_edit()}
							</button>
							<button
								type="button"
								class="rounded-lg border border-zinc-200 px-3 py-1.5 text-xs font-medium text-zinc-700 transition hover:bg-zinc-50 dark:border-zinc-700 dark:text-zinc-200 dark:hover:bg-zinc-800"
								onclick={() => (editableScopes = [...readScopes])}
							>
								{m.user_api_tokens_preset_readonly()}
							</button>
						</div>
					</div>
					<div class="flex max-w-lg flex-wrap gap-2">
						{#each scopeOptions as scope (scope.value)}
							{@const selected = editableScopes.includes(scope.value)}
							<label
								class={`inline-flex cursor-pointer items-center rounded-full border px-3 py-1.5 text-xs font-medium transition ${
									selected
										? 'border-cyan-200 bg-cyan-50 text-cyan-700 dark:border-cyan-900/60 dark:bg-cyan-950/30 dark:text-cyan-200'
										: 'border-zinc-200 bg-white text-zinc-600 hover:border-zinc-300 hover:bg-zinc-50 dark:border-zinc-800 dark:bg-zinc-950 dark:text-zinc-300 dark:hover:border-zinc-700 dark:hover:bg-zinc-900'
								}`}
							>
								<input
									class="sr-only"
									type="checkbox"
									checked={selected}
									onchange={() => toggleEditScope(scope.value)}
								/>
								<span>{scope.label}</span>
							</label>
						{/each}
					</div>
				</div>

				<div class="flex flex-wrap justify-end gap-2">
					<button
						type="button"
						class="rounded-lg border border-zinc-200 px-4 py-2 text-sm font-medium text-zinc-700 transition hover:bg-zinc-50 dark:border-zinc-700 dark:text-zinc-200 dark:hover:bg-zinc-800"
						onclick={closeEditDialog}
					>
						{m.common_cancel()}
					</button>
					<button
						type="submit"
						class="rounded-lg bg-cyan-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-cyan-700 disabled:cursor-not-allowed disabled:opacity-60"
						disabled={!!updatingId}
					>
						{updatingId ? m.common_saving() : m.common_save()}
					</button>
				</div>
			</form>
		</div>
	</div>
{/if}
