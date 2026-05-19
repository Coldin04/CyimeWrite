<script lang="ts">
	import * as m from '$paraglide/messages';
	import { clickOutside } from '$lib/actions/clickOutside';

	let {
		open = false,
		title = '',
		message,
		confirmText,
		secondaryText,
		cancelText,
		confirmVariant = 'danger',
		loading = false,
		disabled = false,
		onConfirm,
		onSecondary,
		onCancel
	}: {
		open?: boolean;
		title?: string;
		message: string;
		confirmText?: string;
		secondaryText?: string;
		cancelText?: string;
		confirmVariant?: 'danger' | 'primary';
		loading?: boolean;
		disabled?: boolean;
		onConfirm?: () => void | Promise<void>;
		onSecondary?: () => void | Promise<void>;
		onCancel?: () => void;
	} = $props();

	const confirmClass = $derived(
		confirmVariant === 'primary'
			? 'bg-sky-500 hover:bg-sky-600 text-white shadow-sm dark:bg-sky-500 dark:hover:bg-sky-400'
			: 'bg-red-600 hover:bg-red-700 text-white'
	);

	function handleKeydown(event: KeyboardEvent) {
		if (!open) {
			return;
		}
		if (event.key === 'Escape') {
			event.preventDefault();
			onCancel?.();
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
	<div
		class="fixed inset-0 z-[100] flex items-center justify-center bg-black/55 p-4 backdrop-blur-[1px]"
	>
		<div
			role="dialog"
			aria-modal="true"
			aria-label={title}
			tabindex="-1"
			class="w-full max-w-md rounded-xl border border-zinc-200 bg-white p-5 shadow-2xl dark:border-zinc-700 dark:bg-zinc-900"
			use:clickOutside={{
				enabled: open,
				handler: () => onCancel?.()
			}}
		>
			{#if title}
				<h3 class="text-base font-semibold text-zinc-900 dark:text-zinc-100">{title}</h3>
			{/if}
			<p class="mt-2 text-sm leading-6 text-zinc-600 dark:text-zinc-300">{message}</p>

			<div class="mt-5 flex justify-end gap-2">
				<button
					type="button"
					class="rounded-md px-4 py-2 text-sm text-zinc-700 transition-colors hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-800"
					disabled={loading}
					onclick={() => onCancel?.()}
				>
					{cancelText ?? m.common_cancel()}
				</button>
				{#if secondaryText}
					<button
						type="button"
						class="rounded-md px-4 py-2 text-sm text-zinc-700 transition-colors hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-800"
						disabled={loading}
						onclick={() => onSecondary?.()}
					>
						{secondaryText}
					</button>
				{/if}
				<button
					type="button"
					class={`rounded-md px-4 py-2 text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-60 ${confirmClass}`}
					disabled={disabled || loading}
					onclick={() => onConfirm?.()}
				>
					{loading ? m.common_loading() : (confirmText ?? m.common_delete())}
				</button>
			</div>
		</div>
	</div>
{/if}
