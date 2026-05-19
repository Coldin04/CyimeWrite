<script lang="ts">
	import { tick } from 'svelte';
	import { clickOutside } from '$lib/actions/clickOutside';
	import * as m from '$paraglide/messages';
	import LinkSimple from '~icons/ph/link-simple';
	import LinkBreak from '~icons/ph/link-break';

	interface Props {
		href: string;
		onSave: (href: string) => void;
		onRemove: () => void;
	}

	let { href, onSave, onRemove }: Props = $props();

	let panelElement: HTMLDivElement | null = null;
	let triggerElement: HTMLButtonElement | null = null;
	let panelContentElement = $state<HTMLDivElement | null>(null);
	let draft = $state('');
	let open = $state(false);
	let panelStyle = $state('');
	const viewportMargin = 12;

	$effect(() => {
		draft = href;
	});

	function closeWithoutSaving() {
		draft = href;
		open = false;
	}

	function handleSave() {
		onSave(draft.trim());
		open = false;
	}

	function updatePanelPosition() {
		if (!triggerElement) return;
		const rect = triggerElement.getBoundingClientRect();
		const panelWidth = panelContentElement?.offsetWidth ?? 288;
		const left = Math.max(
			viewportMargin,
			Math.min(rect.left, window.innerWidth - panelWidth - viewportMargin)
		);
		panelStyle = `position: fixed; left: ${Math.round(left)}px; top: ${Math.round(rect.bottom + 8)}px;`;
	}

	async function togglePanel() {
		open = !open;
		if (!open) return;
		await tick();
		updatePanelPosition();
	}
</script>

<div
	bind:this={panelElement}
	class="flex shrink-0 items-center gap-1"
	use:clickOutside={{
		enabled: open,
		handler: closeWithoutSaving
	}}
>
	<button
		bind:this={triggerElement}
		type="button"
		title={m.editor_link_label()}
		aria-label={m.editor_link_label()}
		class={`inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md transition-colors ${
			href.trim() !== ''
				? 'bg-zinc-900 text-white hover:bg-zinc-800 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200'
				: 'text-zinc-700 hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-800'
		}`}
		onclick={togglePanel}
	>
		<LinkSimple class="h-4 w-4" />
	</button>

	{#if href.trim() !== ''}
		<button
			type="button"
			title={m.editor_link_remove()}
			aria-label={m.editor_link_remove()}
			class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-zinc-700 transition-colors hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-800"
			onclick={onRemove}
		>
			<LinkBreak class="h-4 w-4" />
		</button>
	{/if}

	{#if open}
		<div
			bind:this={panelContentElement}
			style={panelStyle}
			class="z-40 rounded-xl border border-zinc-200 bg-white p-2 shadow-xl shadow-zinc-900/10 dark:border-zinc-700 dark:bg-zinc-900 dark:shadow-black/30"
		>
			<div class="flex min-w-[18rem] items-center gap-2 rounded-lg border border-zinc-200 bg-zinc-50 px-2 py-2 text-xs text-zinc-700 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-200">
				<label class="flex min-w-0 flex-1 items-center gap-2">
					<span class="shrink-0 text-zinc-500 dark:text-zinc-400">
						<LinkSimple class="h-3.5 w-3.5" />
					</span>
					<input
						type="url"
						class="min-w-0 flex-1 bg-transparent text-xs outline-none placeholder:text-zinc-400 dark:placeholder:text-zinc-500"
						placeholder={m.editor_link_placeholder()}
						bind:value={draft}
					/>
				</label>
				<button
					type="button"
					class="inline-flex h-8 shrink-0 items-center rounded-md bg-zinc-900 px-2 text-xs font-medium text-white transition-colors hover:bg-zinc-800 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200"
					onclick={handleSave}
				>
					{m.common_save()}
				</button>
			</div>
		</div>
	{/if}
</div>
