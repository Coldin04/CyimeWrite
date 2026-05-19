<script lang="ts">
	import { clickOutside } from '$lib/actions/clickOutside';

	let {
		open = false,
		title = '',
		maxWidthClass = 'max-w-2xl',
		onClose,
		children
	}: {
		open?: boolean;
		title?: string;
		maxWidthClass?: string;
		onClose?: () => void;
		children?: import('svelte').Snippet;
	} = $props();

	function handleKeydown(event: KeyboardEvent) {
		if (!open) return;
		if (event.key === 'Escape') {
			event.preventDefault();
			onClose?.();
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
	<div class="fixed inset-0 z-[120] flex items-center justify-center bg-black/55 p-4 backdrop-blur-[1px]">
		<div
			role="dialog"
			aria-modal="true"
			aria-label={title}
			tabindex="-1"
			class={`w-full ${maxWidthClass} rounded-xl border border-zinc-200 bg-white p-5 shadow-2xl dark:border-zinc-800 dark:bg-zinc-900`}
			use:clickOutside={{
				enabled: open,
				handler: () => onClose?.()
			}}
		>
			{@render children?.()}
		</div>
	</div>
{/if}
