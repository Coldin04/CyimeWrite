<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import * as m from '$paraglide/messages';
	import Logo from '$lib/components/common/Logo.svelte';
	import {
		approveSkillOAuthRequest,
		denySkillOAuthRequest,
		getSkillOAuthRequest,
		type SkillOAuthRequest
	} from '$lib/api/auth';
	import ArrowSquareOut from '~icons/ph/arrow-square-out';
	import Check from '~icons/ph/check';
	import Key from '~icons/ph/key';
	import ShieldCheck from '~icons/ph/shield-check';
	import WarningCircle from '~icons/ph/warning-circle';
	import X from '~icons/ph/x';

	let requestId = $state('');
	let request = $state<SkillOAuthRequest | null>(null);
	let loading = $state(true);
	let errorMessage = $state('');
	let pendingDecision = $state<'approve' | 'deny' | null>(null);

	const clientName = $derived(
		request?.clientId?.trim() || m.skill_oauth_consent_unknown_client()
	);
	const tokenLifetimeDays = $derived(
		request ? Math.round(request.tokenExpiresInSeconds / 86_400) : 0
	);
	const redirectOrigin = $derived(formatRedirectOrigin(request?.redirectUri ?? ''));
	const expiresAtLabel = $derived(formatDateTime(request?.expiresAt ?? ''));

	onMount(() => {
		requestId = $page.url.searchParams.get('request_id') ?? '';
		void loadRequest();
	});

	async function loadRequest() {
		if (!requestId) {
			errorMessage = m.skill_oauth_consent_invalid_request();
			loading = false;
			return;
		}

		try {
			request = await getSkillOAuthRequest(requestId);
		} catch (error) {
			errorMessage =
				error instanceof Error ? error.message : m.skill_oauth_consent_load_failed();
		} finally {
			loading = false;
		}
	}

	async function decide(decision: 'approve' | 'deny') {
		if (!request) return;
		pendingDecision = decision;
		errorMessage = '';
		try {
			const redirectUrl =
				decision === 'approve'
					? await approveSkillOAuthRequest(request.id)
					: await denySkillOAuthRequest(request.id);
			window.location.href = redirectUrl;
		} catch (error) {
			errorMessage =
				error instanceof Error ? error.message : m.skill_oauth_consent_load_failed();
			pendingDecision = null;
		}
	}

	function formatRedirectOrigin(value: string) {
		if (!value) return '';
		try {
			const parsed = new URL(value);
			return `${parsed.protocol}//${parsed.host}`;
		} catch {
			return value;
		}
	}

	function formatDateTime(value: string) {
		if (!value) return '';
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return value;
		return date.toLocaleString(undefined, {
			year: 'numeric',
			month: '2-digit',
			day: '2-digit',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	function scopeLabel(scope: string) {
		switch (scope) {
			case 'workspace:read':
				return m.skill_oauth_consent_scope_workspace_read();
			case 'workspace:write':
				return m.skill_oauth_consent_scope_workspace_write();
			case 'document:read':
				return m.skill_oauth_consent_scope_document_read();
			case 'document:write':
				return m.skill_oauth_consent_scope_document_write();
			case 'file:move':
				return m.skill_oauth_consent_scope_file_move();
			case 'file:copy':
				return m.skill_oauth_consent_scope_file_copy();
			case 'file:delete':
				return m.skill_oauth_consent_scope_file_delete();
			default:
				return m.skill_oauth_consent_scope_unknown({ scope });
		}
	}
</script>

<svelte:head>
	<title>{m.page_title_skill_oauth_consent()}</title>
</svelte:head>

<div class="min-h-screen bg-slate-50 px-4 py-8 dark:bg-slate-950 sm:py-12">
	<div class="mx-auto w-full max-w-2xl">
		<div class="mb-8 flex justify-center">
			<Logo labelClass="text-4xl font-bold" />
		</div>

		<section
			class="rounded-lg border border-slate-200 bg-white p-6 shadow-sm dark:border-slate-800 dark:bg-slate-900 sm:p-8"
		>
			{#if loading}
				<div class="flex min-h-64 items-center justify-center">
					<p class="text-sm text-slate-500 dark:text-slate-400">{m.common_loading()}</p>
				</div>
			{:else if errorMessage || !request}
				<div class="flex min-h-64 flex-col items-center justify-center text-center">
					<WarningCircle class="h-9 w-9 text-amber-500" />
					<h1 class="mt-4 text-xl font-semibold text-slate-900 dark:text-slate-100">
						{m.skill_oauth_consent_invalid_request()}
					</h1>
					{#if errorMessage}
						<p class="mt-2 max-w-md text-sm leading-6 text-slate-500 dark:text-slate-400">
							{errorMessage}
						</p>
					{/if}
				</div>
			{:else}
				<div class="flex items-start gap-4">
					<div
						class="grid h-11 w-11 shrink-0 place-content-center rounded-lg bg-cyan-50 text-cyan-700 dark:bg-cyan-950/60 dark:text-cyan-300"
					>
						<ShieldCheck class="h-6 w-6" />
					</div>
					<div class="min-w-0">
						<h1 class="text-2xl font-semibold text-slate-950 dark:text-slate-50">
							{m.skill_oauth_consent_title()}
						</h1>
						<p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">
							{m.skill_oauth_consent_subtitle()}
						</p>
					</div>
				</div>

				<div class="mt-8 space-y-5">
					<div>
						<p class="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400">
							{m.skill_oauth_consent_client_label()}
						</p>
						<p class="mt-1 break-words text-base font-medium text-slate-950 dark:text-slate-50">
							{clientName}
						</p>
					</div>

					<div>
						<p class="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400">
							{m.skill_oauth_consent_redirect_label()}
						</p>
						<div class="mt-1 flex min-w-0 items-center gap-2 text-sm text-slate-700 dark:text-slate-300">
							<ArrowSquareOut class="h-4 w-4 shrink-0 text-slate-400" />
							<span class="min-w-0 break-all">{request.redirectUri}</span>
						</div>
						{#if redirectOrigin}
							<p class="mt-1 text-xs text-slate-500 dark:text-slate-400">{redirectOrigin}</p>
						{/if}
					</div>

					<div>
						<p class="text-xs font-semibold uppercase text-slate-500 dark:text-slate-400">
							{m.skill_oauth_consent_permissions_label()}
						</p>
						<ul class="mt-3 grid gap-2 sm:grid-cols-2">
							{#each request.scopes as scope}
								<li
									class="flex min-h-10 items-center gap-2 rounded-lg border border-slate-200 px-3 py-2 text-sm text-slate-700 dark:border-slate-800 dark:text-slate-300"
								>
									<Check class="h-4 w-4 shrink-0 text-cyan-600 dark:text-cyan-300" />
									<span>{scopeLabel(scope)}</span>
								</li>
							{/each}
						</ul>
					</div>
				</div>

				<div
					class="mt-8 rounded-lg border border-cyan-100 bg-cyan-50 p-4 text-sm text-cyan-900 dark:border-cyan-950 dark:bg-cyan-950/40 dark:text-cyan-100"
				>
					<div class="flex gap-3">
						<Key class="mt-0.5 h-4 w-4 shrink-0" />
						<div class="min-w-0 space-y-1">
							<p>{m.skill_oauth_consent_token_lifetime({ days: tokenLifetimeDays })}</p>
							<p>{m.skill_oauth_consent_security_note()}</p>
							{#if expiresAtLabel}
								<p class="text-cyan-800/80 dark:text-cyan-100/75">
									{m.skill_oauth_consent_expiry_hint({ time: expiresAtLabel })}
								</p>
							{/if}
						</div>
					</div>
				</div>

				{#if errorMessage}
					<p class="mt-4 text-sm text-red-600 dark:text-red-400">{errorMessage}</p>
				{/if}

				<div class="mt-8 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
					<button
						type="button"
						class="inline-flex h-10 items-center justify-center gap-2 rounded-lg border border-slate-300 px-4 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-60 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-800"
						disabled={pendingDecision !== null}
						onclick={() => void decide('deny')}
					>
						<X class="h-4 w-4" />
						{pendingDecision === 'deny'
							? m.skill_oauth_consent_denying()
							: m.skill_oauth_consent_deny()}
					</button>
					<button
						type="button"
						class="inline-flex h-10 items-center justify-center gap-2 rounded-lg bg-cyan-600 px-4 text-sm font-semibold text-white transition-colors hover:bg-cyan-700 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-cyan-500 dark:text-slate-950 dark:hover:bg-cyan-400"
						disabled={pendingDecision !== null}
						onclick={() => void decide('approve')}
					>
						<ShieldCheck class="h-4 w-4" />
						{pendingDecision === 'approve'
							? m.skill_oauth_consent_approving()
							: m.skill_oauth_consent_approve()}
					</button>
				</div>
			{/if}
		</section>
	</div>
</div>
