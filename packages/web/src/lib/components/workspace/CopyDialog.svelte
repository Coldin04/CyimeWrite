<script lang="ts">
	import { batchCopyFiles, getAllFolders, type FileItem } from '$lib/api/workspace';
	import { createEventDispatcher } from 'svelte';
	import Folder from '~icons/ph/folder';
	import { toast } from 'svelte-sonner';
	import * as m from '$paraglide/messages';

	type ItemToCopy = {
		id: string;
		type: 'folder' | 'document';
	};

	let {
		items = []
	}: {
		items: ItemToCopy[];
	} = $props();

	const dispatch = createEventDispatcher();

	let isCopying = $state(false);
	let selectedFolderId = $state<string | null>(null);
	let allFolders = $state<FileItem[]>([]);
	let isLoadingFolders = $state(true);

	$effect(() => {
		(async () => {
			try {
				isLoadingFolders = true;
				const folders = await getAllFolders({});
				const foldersToCopy = items.filter((i) => i.type === 'folder');

				if (foldersToCopy.length === 0) {
					allFolders = folders;
					return;
				}

				const foldersToExclude = new Set<string>();
				const queue: string[] = [];

				for (const folder of foldersToCopy) {
					foldersToExclude.add(folder.id);
					queue.push(folder.id);
				}

				const parentToChildrenMap = new Map<string, string[]>();
				for (const folder of folders) {
					if (!folder.parentId) continue;
					if (!parentToChildrenMap.has(folder.parentId)) {
						parentToChildrenMap.set(folder.parentId, []);
					}
					parentToChildrenMap.get(folder.parentId)!.push(folder.id);
				}

				let head = 0;
				while (head < queue.length) {
					const currentId = queue[head++];
					const children = parentToChildrenMap.get(currentId) || [];
					for (const childId of children) {
						if (foldersToExclude.has(childId)) continue;
						foldersToExclude.add(childId);
						queue.push(childId);
					}
				}

				allFolders = folders.filter((folder) => !foldersToExclude.has(folder.id));
			} catch (error) {
				console.error('Failed to load folders:', error);
				toast.error(m.copy_dialog_load_failed());
			} finally {
				isLoadingFolders = false;
			}
		})();
	});

	type FolderTreeNode = {
		id: string;
		name: string;
		children: FolderTreeNode[];
		level: number;
	};

	function buildFolderTree(folders: FileItem[]): FolderTreeNode[] {
		const folderMap = new Map<string, FolderTreeNode>();
		const roots: FolderTreeNode[] = [];

		folders.forEach((folder) => {
			folderMap.set(folder.id, {
				id: folder.id,
				name: folder.name,
				children: [],
				level: 0
			});
		});

		folders.forEach((folder) => {
			const node = folderMap.get(folder.id)!;
			if (folder.parentId) {
				const parent = folderMap.get(folder.parentId);
				if (parent) {
					parent.children.push(node);
				} else {
					roots.push(node);
				}
			} else {
				roots.push(node);
			}
		});

		function setLevels(nodes: FolderTreeNode[], level: number) {
			nodes.forEach((node) => {
				node.level = level;
				setLevels(node.children, level + 1);
			});
		}
		setLevels(roots, 0);

		return roots;
	}

	const folderTree = $derived(buildFolderTree(allFolders));

	function flattenTree(nodes: FolderTreeNode[], result: FolderTreeNode[] = []): FolderTreeNode[] {
		nodes.forEach((node) => {
			result.push(node);
			flattenTree(node.children, result);
		});
		return result;
	}

	const flatFolders = $derived(flattenTree(folderTree));

	function handleCancel() {
		dispatch('cancel');
	}

	async function handleCopy() {
		if (isCopying || items.length === 0) return;

		isCopying = true;
		try {
			const result = await batchCopyFiles(items, selectedFolderId);
			if (result.success) {
				toast.success(m.copy_dialog_success());
			} else {
				toast.warning(
					m.copy_dialog_partial_success({
						success: result.copiedCount,
						failed: result.failedItems?.length ?? 0
					})
				);
			}
			dispatch('copy', { targetId: selectedFolderId });
		} catch (error: any) {
			console.error('Failed to copy:', error);
			toast.error(error.message || m.copy_dialog_failed());
		} finally {
			isCopying = false;
		}
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			handleCancel();
		}
	}
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div
	role="button"
	tabindex="0"
	class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
	onclick={handleCancel}
	onkeydown={(event) => {
		if (event.key === 'Enter' || event.key === ' ') {
			event.preventDefault();
			handleCancel();
		}
		handleKeydown(event);
	}}
>
	<div
		role="dialog"
		aria-modal="true"
		aria-labelledby="copy-dialog-title"
		tabindex="-1"
		class="w-full max-w-md rounded-lg bg-white p-6 shadow-xl dark:bg-zinc-800"
		onclick={(event) => event.stopPropagation()}
		onkeydown={(event) => event.stopPropagation()}
	>
		<h3 id="copy-dialog-title" class="mb-4 text-lg font-medium text-zinc-900 dark:text-zinc-100">
			{m.copy_dialog_title()}
		</h3>

		{#if isLoadingFolders}
			<div class="mb-4 flex items-center justify-center py-8">
				<div class="h-6 w-6 animate-spin rounded-full border-2 border-zinc-300 border-t-blue-500"></div>
			</div>
		{:else}
			<div class="mb-4 max-h-64 overflow-y-auto rounded-md border border-zinc-200 dark:border-zinc-700">
				<button
					type="button"
					class="flex w-full items-center gap-2 border-b border-zinc-100 px-4 py-2 text-left text-sm text-zinc-800 transition-colors hover:bg-zinc-50 dark:border-zinc-700 dark:text-zinc-200 dark:hover:bg-zinc-700 {selectedFolderId ===
					null
						? 'bg-blue-50 dark:bg-blue-900/30'
						: ''}"
					onclick={() => (selectedFolderId = null)}
					onkeydown={(event) => {
						if (event.key === 'Enter' || event.key === ' ') {
							event.preventDefault();
							selectedFolderId = null;
						}
					}}
				>
					<Folder class="h-4 w-4 flex-shrink-0 text-zinc-500" />
					<span class="truncate">{m.move_dialog_root_folder()}</span>
				</button>

				{#each flatFolders as folder (folder.id)}
					<button
						type="button"
						class="flex w-full items-center gap-2 border-b border-zinc-100 px-4 py-2 text-left text-sm text-zinc-800 transition-colors hover:bg-zinc-50 dark:border-zinc-700 dark:text-zinc-200 dark:hover:bg-zinc-700 {selectedFolderId ===
						folder.id
							? 'bg-blue-50 dark:bg-blue-900/30'
							: ''}"
						onclick={() => (selectedFolderId = folder.id)}
						onkeydown={(event) => {
							if (event.key === 'Enter' || event.key === ' ') {
								event.preventDefault();
								selectedFolderId = folder.id;
							}
						}}
						style="padding-left: {1 + folder.level * 1.5}rem"
					>
						<Folder class="h-4 w-4 flex-shrink-0 text-sky-500" />
						<span class="truncate">{folder.name}</span>
					</button>
				{/each}
			</div>
		{/if}

		<div class="flex justify-end gap-2">
			<button
				type="button"
				onclick={handleCancel}
				class="rounded-md px-4 py-2 text-sm text-zinc-700 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-700"
			>
				{m.common_cancel()}
			</button>
			<button
				type="button"
				onclick={handleCopy}
				disabled={isCopying || isLoadingFolders}
				class="rounded-md bg-sky-500 px-4 py-2 text-sm text-white shadow-sm hover:bg-sky-600 disabled:cursor-not-allowed disabled:opacity-50"
			>
				{isCopying ? m.copy_dialog_copying() : m.common_copy()}
			</button>
		</div>
	</div>
</div>
