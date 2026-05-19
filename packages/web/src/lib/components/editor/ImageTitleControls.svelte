<script lang="ts">
	import { tick } from 'svelte';
	import { clickOutside } from '$lib/actions/clickOutside';
	import { fade } from 'svelte/transition';
	import * as m from '$paraglide/messages';

	interface Props {
		titleValue: string;
		descriptionValue: string;
		onSave: (payload: { title: string; description: string }) => void;
	}

	let { titleValue, descriptionValue, onSave }: Props = $props();

	let panelElement: HTMLDivElement | null = null;
	let triggerElement: HTMLButtonElement | null = null;
	let panelContentElement = $state<HTMLDivElement | null>(null);
	let draftTitle = $state('');
	let draftDescription = $state('');
	let open = $state(false);
	let panelStyle = $state('');
	const viewportMargin = 12;
	const defaultPanelWidth = 352;

	$effect(() => {
		draftTitle = titleValue;
		draftDescription = descriptionValue;
	});

	function closeWithoutSaving() {
		draftTitle = titleValue;
		draftDescription = descriptionValue;
		open = false;
	}

	function handleSave() {
		onSave({
			title: draftTitle,
			description: draftDescription
		});
		open = false;
	}

	function updatePanelPosition() {
		if (!triggerElement) return;
		const rect = triggerElement.getBoundingClientRect();
		const panelWidth = panelContentElement?.offsetWidth ?? defaultPanelWidth;
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
		// Run once more on the next frame to stabilize position after layout/transition.
		requestAnimationFrame(updatePanelPosition);
	}
</script>

<div
	bind:this={panelElement}
	class="flex shrink-0 items-center"
	use:clickOutside={{
		enabled: open,
		handler: closeWithoutSaving
	}}
>
	<button
		bind:this={triggerElement}
		type="button"
		title={m.editor_image_title_label()}
		aria-label={m.editor_image_title_label()}
		class="inline-flex h-8 shrink-0 items-center justify-center rounded-md px-2 text-xs text-zinc-700 transition-colors hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-800"
		onclick={togglePanel}
	>
		<span class="text-[11px] font-semibold tracking-[0.02em]">{m.editor_image_title_label()}</span>
	</button>

	{#if open}
		<div
			bind:this={panelContentElement}
			in:fade={{ duration: 120 }}
			out:fade={{ duration: 100 }}
			style={panelStyle}
			class="z-40 w-[22rem] max-w-[calc(100vw-1.5rem)] rounded-xl border border-zinc-200 bg-white p-2 shadow-xl shadow-zinc-900/10 dark:border-zinc-700 dark:bg-zinc-900 dark:shadow-black/30"
		>
			<div class="w-full space-y-2 rounded-lg border border-zinc-200 bg-zinc-50 p-2 text-xs text-zinc-700 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-200">
				<label class="flex items-center gap-2">
					<span class="w-14 shrink-0 text-zinc-500 dark:text-zinc-400">{m.editor_image_title_label()}</span>
					<input
						type="text"
						class="min-w-0 flex-1 rounded border border-zinc-200 bg-white px-2 py-1 text-xs outline-none placeholder:text-zinc-400 dark:border-zinc-700 dark:bg-zinc-900 dark:placeholder:text-zinc-500"
						placeholder={m.editor_image_title_placeholder()}
						bind:value={draftTitle}
					/>
				</label>
				<label class="flex items-start gap-2">
					<span class="w-14 shrink-0 pt-1 text-zinc-500 dark:text-zinc-400">描述</span>
					<textarea
						rows="2"
						class="min-h-12 min-w-0 flex-1 resize-none rounded border border-zinc-200 bg-white px-2 py-1 text-xs outline-none placeholder:text-zinc-400 dark:border-zinc-700 dark:bg-zinc-900 dark:placeholder:text-zinc-500"
						placeholder="输入图片描述（用于朗读）"
						bind:value={draftDescription}
					></textarea>
				</label>
				<div class="flex justify-end">
					<button
						type="button"
						class="inline-flex h-8 shrink-0 items-center rounded-md bg-zinc-900 px-2 text-xs font-medium text-white transition-colors hover:bg-zinc-800 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200"
						onclick={handleSave}
					>
						{m.common_save()}
					</button>
				</div>
			</div>
		</div>
	{/if}
</div>
