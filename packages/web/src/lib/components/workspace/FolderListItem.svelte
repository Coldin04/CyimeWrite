<script lang="ts">
	import Folder from '~icons/ph/folder';
	import type { FileItem } from '$lib/api/workspace';
	import DotsThreeVertical from '~icons/ph/dots-three-vertical';
	import Pencil from '~icons/ph/pencil';
	import Trash from '~icons/ph/trash';
	import FolderOpen from '~icons/ph/folder-open';
	import CopySimple from '~icons/ph/copy-simple';
	import { deleteFile, updateFileName } from '$lib/api/workspace';
	import { toast } from 'svelte-sonner';
	import MoveDialog from '$lib/components/workspace/MoveDialog.svelte';
	import CopyDialog from '$lib/components/workspace/CopyDialog.svelte';
	import ConfirmDialog from '$lib/components/common/ConfirmDialog.svelte';
	import { clickOutside } from '$lib/actions/clickOutside';
	import * as m from '$paraglide/messages';
	import { getLocale } from '$paraglide/runtime';

	const {
		item,
		selectedItems,
		bulkMode = false,
		onToggle,
		onNavigate,
		onRefresh
	}: {
		item: FileItem;
		selectedItems: { [key: string]: boolean };
		bulkMode?: boolean;
		onToggle: (id: string) => void;
		onNavigate?: (id: string) => void;
		onRefresh?: () => void;
	} = $props();

	const isSelected = $derived(!!selectedItems[item.id]);
	const checkboxClasses = $derived(
		`h-4 w-4 rounded border-zinc-400 transition-opacity dark:border-zinc-600 ${
			bulkMode || isSelected ? 'opacity-100' : 'opacity-0'
		}`
	);

	let showMenu = $state(false);
	let isEditing = $state(false);
	let isDeleteConfirmOpen = $state(false);
	let editingName = $state('');
	let isMoving = $state(false);
	let isCopying = $state(false);
	let menuPlacement = $state<'button' | 'context'>('button');
	let contextMenuX = $state(0);
	let contextMenuY = $state(0);
	const isMovingItem = $derived(isMoving && item.type === 'folder');
	const isCopyingItem = $derived(isCopying && item.type === 'folder');
	const menuClass = $derived(
		menuPlacement === 'context'
			? 'fixed z-40 w-44 origin-top-left rounded-md bg-white py-1 shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none dark:bg-zinc-800 dark:ring-zinc-700'
			: 'absolute top-full right-0 z-20 mt-1 w-44 origin-top-right rounded-md bg-white py-1 shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none dark:bg-zinc-800 dark:ring-zinc-700'
	);
	const menuStyle = $derived(
		menuPlacement === 'context' ? `left: ${contextMenuX}px; top: ${contextMenuY}px;` : ''
	);

	function formatRelativeTime(dateString: string): string {
		const date = new Date(dateString);
		const now = new Date();
		const diffInSeconds = Math.floor((now.getTime() - date.getTime()) / 1000);

		if (diffInSeconds < 60) {
			return m.time_just_now();
		} else if (diffInSeconds < 3600) {
			const minutes = Math.floor(diffInSeconds / 60);
			return m.time_minutes_ago({ minutes });
		} else if (diffInSeconds < 86400) {
			const hours = Math.floor(diffInSeconds / 3600);
			return m.time_hours_ago({ hours });
		} else if (diffInSeconds < 604800) {
			const days = Math.floor(diffInSeconds / 86400);
			return m.time_days_ago({ days });
		} else {
			return date.toLocaleDateString(getLocale(), {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		}
	}

	function handleClick() {
		if (bulkMode) {
			onToggle(item.id);
		} else {
			onNavigate?.(item.id);
		}
	}

	function handleKeyDown(event: KeyboardEvent) {
		if (event.key === ' ' || event.key === 'Enter') {
			event.preventDefault();
			handleClick();
		}
	}

	function toggleMenu() {
		menuPlacement = 'button';
		showMenu = !showMenu;
	}

	function closeMenu() {
		showMenu = false;
	}

	function openContextMenu(event: MouseEvent) {
		event.preventDefault();
		event.stopPropagation();
		const margin = 8;
		const menuWidth = 176;
		const menuHeight = 176;
		contextMenuX = Math.max(margin, Math.min(event.clientX, window.innerWidth - menuWidth - margin));
		contextMenuY = Math.max(margin, Math.min(event.clientY, window.innerHeight - menuHeight - margin));
		menuPlacement = 'context';
		showMenu = true;
	}

	function startEditing() {
		editingName = item.name || '';
		isEditing = true;
		showMenu = false;
	}

	async function saveEditing() {
		if (!editingName.trim() || editingName === item.name) {
			isEditing = false;
			return;
		}

		try {
			await updateFileName(item.id, 'folder', editingName.trim());
			toast.success(m.folder_rename_success());
			onRefresh?.();
		} catch (error) {
			console.error('Failed to rename:', error);
			toast.error(m.folder_rename_failed());
		} finally {
			isEditing = false;
		}
	}

	function cancelEditing() {
		isEditing = false;
	}

	function handleEditingKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			saveEditing();
		} else if (e.key === 'Escape') {
			cancelEditing();
		}
	}

	function openDeleteConfirm() {
		isDeleteConfirmOpen = true;
		showMenu = false;
	}

	function closeDeleteConfirm() {
		isDeleteConfirmOpen = false;
	}

	async function confirmDelete() {
		try {
			await deleteFile(item.id, 'folder');
			toast.success(m.folder_delete_success());
			onRefresh?.();
		} catch (error) {
			console.error('Failed to delete:', error);
			toast.error(m.folder_delete_failed());
		} finally {
			isDeleteConfirmOpen = false;
		}
	}

	function startMoving() {
		isMoving = true;
		showMenu = false;
	}

	function startCopying() {
		isCopying = true;
		showMenu = false;
	}

	function handleMoveComplete() {
		isMoving = false;
		onRefresh?.();
	}

	function handleMoveCancel() {
		isMoving = false;
	}

	function handleCopyComplete() {
		isCopying = false;
		onRefresh?.();
	}

	function handleCopyCancel() {
		isCopying = false;
	}
</script>

<div
	role="button"
	tabindex="0"
	class="group flex cursor-pointer items-center justify-between border-b border-zinc-200 px-4 py-3 transition-colors hover:bg-gradient-to-r hover:from-sky-50/50 hover:to-transparent dark:border-zinc-700 dark:hover:bg-none dark:hover:bg-zinc-800/60 {isSelected
		? 'bg-sky-50 dark:bg-sky-900/30'
		: ''}"
	onclick={handleClick}
	onkeydown={handleKeyDown}
	oncontextmenu={openContextMenu}
>
	<!-- Left Side: Name -->
	<div class="flex min-w-0 items-center gap-3 pr-4">
		<input
			type="checkbox"
			class={checkboxClasses}
			checked={isSelected}
			onclick={(e) => e.stopPropagation()}
			onchange={() => onToggle(item.id)}
		/>
		<Folder class="h-5 w-5 flex-shrink-0 text-sky-500 dark:text-sky-400" />
		<span class="truncate font-normal text-zinc-800 dark:text-zinc-200">{item.name}</span>
	</div>

	<!-- Right Side: Metadata -->
	<div class="flex flex-shrink-0 items-center justify-end gap-x-4 sm:gap-x-6">
		<div class="hidden w-28 text-right text-sm text-zinc-600 dark:text-zinc-400 sm:block">
			{formatRelativeTime(item.updatedAt)}
		</div>
		<div class="hidden w-24 text-right text-sm text-zinc-600 dark:text-zinc-400 md:block pr-0.5">
			{item.creator.displayName || m.common_you()}
		</div>
		<div
			class="relative w-10 flex justify-center"
			use:clickOutside={{
				enabled: showMenu,
				handler: closeMenu
			}}
		>
			<button
				class="rounded-full p-2 text-zinc-500 transition-colors hover:bg-zinc-200 dark:text-zinc-400 dark:hover:bg-zinc-700"
				onclick={(e) => {
					e.stopPropagation();
					toggleMenu();
				}}
			>
				<DotsThreeVertical class="h-5 w-5" />
			</button>
			
			{#if showMenu}
				<div
					role="menu"
					class={menuClass}
					style={menuStyle}
					onclick={(e) => e.stopPropagation()}
					onkeydown={(e) => {
						if (e.key === 'Escape') {
							closeMenu();
						}
					}}
					tabindex="-1"
				>
					<button
						onclick={startEditing}
						class="flex w-full items-center gap-2 px-4 py-2 text-sm text-zinc-700 hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-700"
						role="menuitem"
					>
						<Pencil class="h-4 w-4" />
						<span>{m.common_rename()}</span>
					</button>
					<button
						onclick={startMoving}
						class="flex w-full items-center gap-2 px-4 py-2 text-sm text-zinc-700 hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-700"
						role="menuitem"
					>
						<FolderOpen class="h-4 w-4" />
						<span>{m.common_move_to()}</span>
					</button>
					<button
						onclick={startCopying}
						class="flex w-full items-center gap-2 px-4 py-2 text-sm text-zinc-700 hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-700"
						role="menuitem"
					>
						<CopySimple class="h-4 w-4" />
						<span>{m.common_copy_to()}</span>
					</button>
					<button
						onclick={openDeleteConfirm}
						class="flex w-full items-center gap-2 px-4 py-2 text-sm text-red-600 hover:bg-zinc-100 dark:text-red-400 dark:hover:bg-zinc-700"
						role="menuitem"
					>
						<Trash class="h-4 w-4" />
						<span>{m.common_delete()}</span>
					</button>
				</div>
			{/if}
		</div>
	</div>
</div>

{#if isMoving && isMovingItem}
	<MoveDialog
		items={[{ id: item.id, type: item.type }]}
		on:cancel={handleMoveCancel}
		on:move={handleMoveComplete}
	/>
{/if}

{#if isCopying && isCopyingItem}
	<CopyDialog
		items={[{ id: item.id, type: item.type }]}
		on:cancel={handleCopyCancel}
		on:copy={handleCopyComplete}
	/>
{/if}

<ConfirmDialog
	open={isDeleteConfirmOpen}
	title={m.common_delete()}
	message={m.folder_delete_confirm()}
	confirmText={m.common_delete()}
	onCancel={closeDeleteConfirm}
	onConfirm={confirmDelete}
/>

{#if isEditing}
	<div
		role="button"
		tabindex="0"
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		onclick={cancelEditing}
		onkeydown={(e) => {
			if (e.key === 'Escape' || e.key === 'Enter') {
				cancelEditing();
			}
		}}
	>
		<div
			role="presentation"
			class="w-full max-w-md rounded-lg bg-white p-6 shadow-xl dark:bg-zinc-800"
			onclick={(e) => e.stopPropagation()}
		>
			<h3 class="mb-4 text-lg font-medium text-zinc-900 dark:text-zinc-100">{m.folder_rename_title()}</h3>
			<input
				type="text"
				value={editingName}
				oninput={(e) => editingName = e.currentTarget.value}
				onkeydown={handleEditingKeydown}
				class="mb-4 w-full rounded-md border border-zinc-300 px-3 py-2 text-base text-zinc-900 focus:border-blue-500 focus:outline-none dark:border-zinc-600 dark:bg-zinc-700 dark:text-zinc-100"
				placeholder={m.folder_name_placeholder()}
			/>
			<div class="flex justify-end gap-2">
				<button
					onclick={cancelEditing}
					class="rounded-md px-4 py-2 text-sm text-zinc-700 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-700"
				>
					{m.common_cancel()}
				</button>
				<button
					onclick={saveEditing}
					class="rounded-md bg-sky-500 px-4 py-2 text-sm text-white shadow-sm hover:bg-sky-600"
				>
					{m.common_save()}
				</button>
			</div>
		</div>
	</div>
{/if}
