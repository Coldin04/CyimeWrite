<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import * as m from '$paraglide/messages';
	import { apiBaseUrl } from '$lib/config/api';
	import Check from '~icons/ph/check';
	import Copy from '~icons/ph/copy';

	type ConfigTarget = 'lobechat' | 'cherry';

	const mcpEndpoint = `${apiBaseUrl}/api/v1/mcp`;
	const tokenPlaceholder = '<CYIME_API_TOKEN>';

	let frontendOrigin = $state('');
	let token = $state('');
	let target = $state<ConfigTarget>('lobechat');
	let copiedTarget = $state<ConfigTarget | ''>('');

	const authHeader = $derived(`Bearer ${token.trim() || tokenPlaceholder}`);
	const lobeChatConfig = $derived(
		JSON.stringify(
			{
				mcpServers: {
					'cyime-workspace': {
						type: 'http',
						url: mcpEndpoint,
						headers: {
							Authorization: authHeader
						}
					}
				}
			},
			null,
			2
		)
	);
	const cherryConfig = $derived(
		JSON.stringify(
			{
				name: 'Cyime Workspace',
				type: 'streamableHttp',
				description: 'Cyime workspace MCP server',
				provider: 'Cyime',
				providerUrl: frontendOrigin || 'http://localhost:5173',
				baseUrl: mcpEndpoint,
				headers: {
					Authorization: authHeader
				}
			},
			null,
			2
		)
	);
	const activeConfig = $derived(target === 'lobechat' ? lobeChatConfig : cherryConfig);

	onMount(() => {
		frontendOrigin = window.location.origin;
	});

	async function copyConfig(configTarget: ConfigTarget) {
		const value = configTarget === 'lobechat' ? lobeChatConfig : cherryConfig;
		try {
			await navigator.clipboard.writeText(value);
			copiedTarget = configTarget;
			toast.success(m.user_mcp_config_copied());
			window.setTimeout(() => {
				if (copiedTarget === configTarget) copiedTarget = '';
			}, 1500);
		} catch {
			toast.error(m.user_mcp_config_copy_failed());
		}
	}
</script>

<div class="space-y-5">
	<section class="space-y-4 rounded-xl border border-zinc-200 p-4 dark:border-zinc-800">
		<div class="grid gap-4 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
			<label class="space-y-1">
				<span class="text-sm font-medium text-zinc-900 dark:text-zinc-100">
					{m.user_mcp_config_endpoint_label()}
				</span>
				<input
					class="w-full rounded-lg border border-zinc-200 bg-zinc-50 px-3 py-2 font-mono text-xs text-zinc-700 outline-none dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-200"
					value={mcpEndpoint}
					readonly
				/>
			</label>

			<label class="space-y-1">
				<span class="text-sm font-medium text-zinc-900 dark:text-zinc-100">
					{m.user_mcp_config_token_label()}
				</span>
				<input
					class="w-full rounded-lg border border-zinc-200 bg-white px-3 py-2 font-mono text-xs text-zinc-900 outline-none transition focus:border-cyan-500 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
					bind:value={token}
					placeholder={tokenPlaceholder}
					autocomplete="off"
					spellcheck="false"
				/>
			</label>
		</div>
	</section>

	<section class="overflow-hidden rounded-xl border border-zinc-200 dark:border-zinc-800">
		<div class="flex flex-col gap-3 border-b border-zinc-200 p-4 dark:border-zinc-800 sm:flex-row sm:items-center sm:justify-between">
			<div class="inline-grid grid-cols-2 rounded-lg bg-zinc-100 p-1 dark:bg-zinc-900">
				<button
					type="button"
					class={`rounded-md px-3 py-1.5 text-sm font-medium transition ${
						target === 'lobechat'
							? 'bg-white text-zinc-900 shadow-sm dark:bg-zinc-800 dark:text-zinc-100'
							: 'text-zinc-600 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100'
					}`}
					onclick={() => (target = 'lobechat')}
				>
					LobeChat
				</button>
				<button
					type="button"
					class={`rounded-md px-3 py-1.5 text-sm font-medium transition ${
						target === 'cherry'
							? 'bg-white text-zinc-900 shadow-sm dark:bg-zinc-800 dark:text-zinc-100'
							: 'text-zinc-600 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100'
					}`}
					onclick={() => (target = 'cherry')}
				>
					Cherry Studio
				</button>
			</div>

			<button
				type="button"
				class="inline-flex h-9 items-center justify-center gap-2 rounded-lg bg-cyan-600 px-3 text-sm font-medium text-white transition hover:bg-cyan-700"
				onclick={() => copyConfig(target)}
			>
				{#if copiedTarget === target}
					<Check class="h-4 w-4 shrink-0" />
					<span>{m.user_mcp_config_copied_action()}</span>
				{:else}
					<Copy class="h-4 w-4 shrink-0" />
					<span>{m.user_mcp_config_copy_action()}</span>
				{/if}
			</button>
		</div>

		<pre class="max-h-[520px] overflow-auto bg-zinc-950 p-4 text-xs leading-6 text-zinc-100"><code>{activeConfig}</code></pre>
	</section>
</div>
