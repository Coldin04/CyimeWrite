<script lang="ts">
	import { tick } from 'svelte';
	import { fade } from 'svelte/transition';
	import { clickOutside } from '$lib/actions/clickOutside';
	import * as m from '$paraglide/messages';
	import Code from '~icons/ph/code';
	import CaretDown from '~icons/ph/caret-down';
	import Check from '~icons/ph/check';
	import { codeBlockLanguageOptions, normalizeCodeBlockLanguage } from '$lib/components/editor/codeHighlight';

	interface Props {
		currentValue: string;
		onSelect: (value: string) => void;
	}

	let { currentValue, onSelect }: Props = $props();

	let menuElement: HTMLDivElement | null = null;
	let triggerElement: HTMLButtonElement | null = null;
	let panelElement = $state<HTMLDivElement | null>(null);
	let customInputElement = $state<HTMLInputElement | null>(null);
	let open = $state(false);
	let panelStyle = $state('');
	let customValue = $state('');
	const viewportMargin = 12;

	const customLanguageInputId = 'cw-code-language-custom';
	const presetValues: Set<string> = new Set(codeBlockLanguageOptions.map((option) => option.value));
	const currentLabel = $derived(resolveCurrentLabel(currentValue));

	function resolveCurrentLabel(value: string) {
		const preset = codeBlockLanguageOptions.find((option) => option.value === value);
		if (preset) return preset.label;
		const normalized = normalizeCodeBlockLanguage(value);
		return normalized || 'Auto';
	}

	function handleSelect(value: string) {
		open = false;
		onSelect(value);
	}

	function closeMenu() {
		open = false;
	}

	function updatePanelPosition() {
		if (!triggerElement) return;
		const rect = triggerElement.getBoundingClientRect();
		const panelWidth = panelElement?.offsetWidth ?? 224;
		const left = Math.max(
			viewportMargin,
			Math.min(rect.left, window.innerWidth - panelWidth - viewportMargin)
		);
		panelStyle = `position: fixed; left: ${Math.round(left)}px; top: ${Math.round(rect.bottom + 8)}px;`;
	}

	async function toggleMenu() {
		open = !open;
		if (!open) return;
		customValue = presetValues.has(currentValue) ? '' : currentValue;
		await tick();
		updatePanelPosition();
	}

	function applyCustomLanguage() {
		const normalized = normalizeCodeBlockLanguage(customValue);
		if (!normalized) return;
		handleSelect(normalized);
	}
</script>

<div
	bind:this={menuElement}
	class="shrink-0"
	use:clickOutside={{
		enabled: open,
		handler: closeMenu
	}}
>
	<button
		bind:this={triggerElement}
		type="button"
		title={m.editor_toolbar_code_language()}
		aria-label={m.editor_toolbar_code_language()}
		aria-haspopup="menu"
		aria-expanded={open}
		class="flex h-8 shrink-0 items-center gap-1.5 rounded-md border border-zinc-900 bg-zinc-900 px-2 text-xs text-white transition-colors hover:bg-zinc-800 dark:border-zinc-100 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200"
		onclick={toggleMenu}
	>
		<Code class="h-4 w-4 shrink-0" />
		<span class="inline-flex min-w-8 items-center justify-center text-[11px] font-semibold uppercase">
			{currentLabel}
		</span>
		<CaretDown class={`h-3.5 w-3.5 transition-transform ${open ? 'rotate-180' : ''}`} />
	</button>

	{#if open}
		<div
			bind:this={panelElement}
			in:fade={{ duration: 120 }}
			out:fade={{ duration: 100 }}
			role="menu"
			style={panelStyle}
			class="z-40 min-w-[14rem] rounded-xl border border-zinc-200 bg-white p-1.5 shadow-xl shadow-zinc-900/10 dark:border-zinc-700 dark:bg-zinc-900 dark:shadow-black/30"
		>
			{#each codeBlockLanguageOptions as option, index}
				<button
					type="button"
					role="menuitem"
					class={`flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition-colors ${
						index > 0 ? 'mt-1' : ''
					} ${
						currentValue === option.value
							? 'bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900'
							: 'text-zinc-700 hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-800'
					}`}
					onclick={() => handleSelect(option.value)}
				>
					<span class="inline-flex h-4 min-w-8 items-center justify-center text-[11px] font-semibold uppercase">
						{option.value || 'Auto'}
					</span>
					<span>{option.label}</span>
					{#if currentValue === option.value}
						<Check class="ml-auto h-4 w-4 shrink-0" />
					{/if}
				</button>
			{/each}

			<div class="mt-1.5 border-t border-zinc-200 pt-1.5 dark:border-zinc-700">
				<label
					for={customLanguageInputId}
					class="block px-2.5 pb-1 text-[11px] font-medium text-zinc-500 dark:text-zinc-400"
				>
					{m.editor_toolbar_code_language_custom()}
				</label>
				<div class="flex items-center gap-1.5 px-1">
					<input
						id={customLanguageInputId}
						bind:this={customInputElement}
						bind:value={customValue}
						type="text"
						inputmode="text"
						spellcheck="false"
						autocomplete="off"
						autocorrect="off"
						autocapitalize="off"
						placeholder="rust"
						class="h-8 min-w-0 flex-1 rounded-md border border-zinc-200 bg-white px-2 text-xs text-zinc-800 outline-none transition-colors placeholder:text-zinc-400 focus:border-zinc-500 focus:ring-2 focus:ring-zinc-200 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100 dark:focus:border-zinc-500 dark:focus:ring-zinc-800"
						onkeydown={(event) => {
							if (event.key === 'Enter') {
								event.preventDefault();
								applyCustomLanguage();
							}
							if (event.key === 'Escape') {
								event.preventDefault();
								closeMenu();
							}
						}}
					/>
					<button
						type="button"
						class="inline-flex h-8 shrink-0 items-center justify-center rounded-md bg-zinc-900 px-2 text-xs font-medium text-white transition-colors hover:bg-zinc-800 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200"
						disabled={!normalizeCodeBlockLanguage(customValue)}
						onclick={applyCustomLanguage}
					>
						{m.common_save()}
					</button>
				</div>
			</div>
		</div>
	{/if}
</div>
