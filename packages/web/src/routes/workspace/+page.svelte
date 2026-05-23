<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import GreetingHeader from '$lib/components/workspace/GreetingHeader.svelte';
	import Toolbar from '$lib/components/workspace/Toolbar.svelte';
	import ListHeader from '$lib/components/workspace/ListHeader.svelte';
	import FolderListItem from '$lib/components/workspace/FolderListItem.svelte';
	import DocumentListItem from '$lib/components/workspace/DocumentListItem.svelte';
	import TableDocumentListItem from '$lib/components/workspace/TableDocumentListItem.svelte';
	import FolderListItemSkeleton from '$lib/components/workspace/FolderListItemSkeleton.svelte';
	import DocumentListItemSkeleton from '$lib/components/workspace/DocumentListItemSkeleton.svelte';
	import NewFolderItem from '$lib/components/workspace/NewFolderItem.svelte';
	import MoveDialog from '$lib/components/workspace/MoveDialog.svelte';
	import CopyDialog from '$lib/components/workspace/CopyDialog.svelte';
	import SharedFolderListItem from '$lib/components/workspace/SharedFolderListItem.svelte';
	import {
		getFiles,
		getFolderAncestors,
		getSharedDocumentSummary,
		batchDeleteFiles,
		type FileItem
	} from '$lib/api/workspace';
	import { realtimeConfig } from '$lib/stores/realtime';
	import { breadcrumbItems, workspaceContext } from '$lib/stores/workspace';
	import * as m from '$paraglide/messages';
	import { toast } from 'svelte-sonner';

	let items = $state<FileItem[]>([]);
	let hasMore = $state(false);
	let hasSharedEntry = $state(false);
	let sortBy = $state('updated_at');
	let order = $state('desc');
	let filterType = $state<'all' | 'folders' | 'documents'>('all');
	let isLoading = $state(true);
	let refreshTrigger = $state(0);
	let isMoveDialogOpen = $state(false);
	let isCopyDialogOpen = $state(false);
	let realtimeConfigSignal = $state(get(realtimeConfig));
	realtimeConfig.subscribe((state) => (realtimeConfigSignal = state));
	const collaborationEnabled = $derived(realtimeConfigSignal.config?.collaborationEnabled ?? false);

	// Use local state for selected items to avoid store overhead during rapid selection
	let bulkMode = $state(false);
	let selectedItems = $state<{ [key: string]: boolean }>({});
	const selectableItems = $derived(items);

	// Sync bulk mode with store and reset local selection when bulk mode changes
	$effect(() => {
		if ($workspaceContext.bulkMode !== bulkMode) {
			bulkMode = $workspaceContext.bulkMode;
			if (!bulkMode) {
				selectedItems = {};
			}
		}
	});

	// Use local state for derived values (much faster than store-derived)
	const selectedItemsCount = $derived(Object.keys(selectedItems).length);
	const allSelected = $derived(selectableItems.length > 0 && selectedItemsCount === selectableItems.length);
	const someSelected = $derived(selectedItemsCount > 0 && !allSelected);

	// Skeleton delay state - only show skeleton if loading takes more than 200ms
	let showSkeleton = $state(false);
	let skeletonTimer: ReturnType<typeof setTimeout> | null = null;

	onMount(() => {
		const handleSharedDocumentsChanged = () => {
			refreshTrigger++;
		};

		window.addEventListener('workspace:shared-documents-changed', handleSharedDocumentsChanged);
		return () => {
			window.removeEventListener('workspace:shared-documents-changed', handleSharedDocumentsChanged);
		};
	});

	$effect(() => {
		const trigger = refreshTrigger;
		let aborted = false;

		// Start loading and skeleton timer
		isLoading = true;
		showSkeleton = false;

		// Only show skeleton if loading takes more than 200ms
		skeletonTimer = setTimeout(() => {
			if (!aborted && isLoading) {
				showSkeleton = true;
			}
		}, 200);

		(async () => {
			try {
				const shouldProbeSharedFolder =
					collaborationEnabled &&
					$workspaceContext.currentFolderId === null &&
					(filterType === 'all' || filterType === 'folders');

				const filesPromise = getFiles({
					parent_id: $workspaceContext.currentFolderId,
					limit: 50,
					offset: 0,
					sort_by: sortBy,
					order: order,
					type: filterType
				});

				const ancestorsPromise = $workspaceContext.currentFolderId
					? getFolderAncestors($workspaceContext.currentFolderId)
					: Promise.resolve([]);
				const sharedSummaryPromise = shouldProbeSharedFolder
					? getSharedDocumentSummary().catch(() => ({ hasSharedDocuments: false }))
					: Promise.resolve({ hasSharedDocuments: false });

				const [fileData, ancestorData, sharedSummary] = await Promise.all([
					filesPromise,
					ancestorsPromise,
					sharedSummaryPromise
				]);

				if (aborted) return;

				hasSharedEntry = !!sharedSummary?.hasSharedDocuments;
				items = fileData.items || [];
				hasMore = fileData.hasMore;
				breadcrumbItems.set(ancestorData);
			} catch (error) {
				if (aborted) return;
				console.error('Failed to load workspace data:', error);
				items = [];
				hasMore = false;
				hasSharedEntry = false;
				breadcrumbItems.set([]);
			} finally {
				if (aborted) return;
				isLoading = false;
				showSkeleton = false;
				if (skeletonTimer) {
					clearTimeout(skeletonTimer);
					skeletonTimer = null;
				}
			}
		})();

		return () => {
			aborted = true;
			showSkeleton = false;
			if (skeletonTimer) {
				clearTimeout(skeletonTimer);
				skeletonTimer = null;
			}
		};
	});

	function handleFolderCreated() {
		workspaceContext.update((ctx) => ({ ...ctx, isCreatingFolder: false }));
		refreshTrigger++;
	}

	function toggleBulkMode() {
		bulkMode = !bulkMode;
		selectedItems = {};
		workspaceContext.update((ctx) => ({ ...ctx, bulkMode }));
	}

	function toggleSelectAll() {
		if (allSelected) {
			selectedItems = {};
		} else {
			const newSelected: { [key: string]: boolean } = {};
			for (const item of selectableItems) {
				newSelected[item.id] = true;
			}
			selectedItems = newSelected;
			if (!bulkMode) {
				bulkMode = true;
				workspaceContext.update((ctx) => ({ ...ctx, bulkMode: true }));
			}
		}
	}

	async function handleBulkDelete() {
		const itemsToDelete = getSelectedItemsDetails();
		if (itemsToDelete.length === 0) return;

		try {
			const result = await batchDeleteFiles(itemsToDelete);

			if (result.success) {
				toast.success(m.workspace_bulk_delete_success({ count: itemsToDelete.length }));
			} else {
				const failedCount = result.failedItems?.length || 0;
				const successCount = itemsToDelete.length - failedCount;
				toast.warning(
					m.workspace_bulk_delete_partial_success({
						success: successCount,
						failed: failedCount
					})
				);
			}
		} catch (error) {
			console.error('Failed to delete items:', error);
			toast.error(
				m.workspace_bulk_delete_failed({
					error: error instanceof Error ? error.message : '未知错误'
				})
			);
		} finally {
			resetBulkMode();
			refreshTrigger++;
		}
	}

	function toggleItem(id: string) {
		const wasSelected = !!selectedItems[id];
		const newSelected = { ...selectedItems };
		if (newSelected[id]) {
			delete newSelected[id];
		} else {
			newSelected[id] = true;
		}
		selectedItems = newSelected;

		// Keep item-level checkbox behavior aligned with "select all":
		// selecting an item while not in bulk mode should enter bulk mode.
		if (!wasSelected && !bulkMode) {
			bulkMode = true;
			workspaceContext.update((ctx) => ({ ...ctx, bulkMode: true }));
		}
	}

	function getSelectedItemsDetails() {
		return Object.keys(selectedItems)
			.map((id) => {
				const fileItem = items.find((i) => i.id === id);
				return fileItem ? { id: fileItem.id, type: fileItem.type } : null;
			})
			.filter((item): item is { id: string; type: 'folder' | 'document' } => item !== null);
	}

	function handleBulkMove() {
		if (selectedItemsCount > 0) {
			isMoveDialogOpen = true;
		}
	}

	function handleBulkCopy() {
		if (selectedItemsCount > 0) {
			isCopyDialogOpen = true;
		}
	}

	function handleMoveDialogClose() {
		isMoveDialogOpen = false;
		resetBulkMode();
		refreshTrigger++;
	}

	function handleCopyDialogClose() {
		isCopyDialogOpen = false;
		resetBulkMode();
		refreshTrigger++;
	}

	function resetBulkMode() {
		selectedItems = {};
		bulkMode = false;
		workspaceContext.update((ctx) => ({ ...ctx, bulkMode: false }));
	}
</script>

<svelte:head>
	<title>{m.page_title_workspace()}</title>
</svelte:head>

<div>
	<GreetingHeader />

	<Toolbar
		{bulkMode}
		{selectedItemsCount}
		onToggleBulk={resetBulkMode}
		onBulkDelete={handleBulkDelete}
		onBulkMove={handleBulkMove}
		onBulkCopy={handleBulkCopy}
		onNavigate={(id) => {
			workspaceContext.update((ctx) => ({ ...ctx, currentFolderId: id, bulkMode: false }));
			resetBulkMode();
		}}
	/>

	<div class="my-6 border-t border-zinc-200 dark:border-zinc-700">
		<ListHeader
			{allSelected}
			{someSelected}
			{bulkMode}
			{selectedItemsCount}
			on:toggleAll={toggleSelectAll}
			on:bulkdelete={handleBulkDelete}
		/>

		<!-- 新建文件夹组件 -->
		{#if $workspaceContext.isCreatingFolder}
			<NewFolderItem
				parentId={$workspaceContext.currentFolderId}
				on:create={handleFolderCreated}
				on:cancel={() => {
					workspaceContext.update((ctx) => ({ ...ctx, isCreatingFolder: false }));
				}}
			/>
		{/if}

		<!-- 文件列表 -->
	{#if showSkeleton}
		<FolderListItemSkeleton />
		<DocumentListItemSkeleton />
	{:else if items.length === 0 && !hasSharedEntry}
		<div class="flex flex-col items-center justify-center py-12 text-center">
				<svg
					xmlns="http://www.w3.org/2000/svg"
					width="48"
					height="48"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="1.5"
					stroke-linecap="round"
					stroke-linejoin="round"
					class="mb-4 text-zinc-400 dark:text-zinc-500"
				>
					<path
						d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13c0 1.1.9 2 2 2Z"
					/>
				</svg>
				<h3 class="text-lg font-semibold text-zinc-800 dark:text-zinc-200">
					{m.workspace_empty_title()}
				</h3>
				<p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">
					{m.workspace_empty_description()}
				</p>
			</div>
	{:else}
		{#if collaborationEnabled && hasSharedEntry && $workspaceContext.currentFolderId === null && (filterType === 'all' || filterType === 'folders')}
			<SharedFolderListItem />
		{/if}

		{#each items as item (item.id)}
			{#if item.type === 'folder'}
				<FolderListItem
					{item}
					{selectedItems}
					{bulkMode}
					onToggle={toggleItem}
					onNavigate={(id) => {
						workspaceContext.update((ctx) => ({ ...ctx, currentFolderId: id, bulkMode: false }));
						resetBulkMode();
					}}
					onRefresh={() => refreshTrigger++}
				/>
			{:else if item.type === 'document'}
				{#if item.documentType === 'table'}
					<TableDocumentListItem
						{item}
							{selectedItems}
							{bulkMode}
							{collaborationEnabled}
							onToggle={toggleItem}
							onRefresh={() => refreshTrigger++}
						/>
					{:else}
						<DocumentListItem
							{item}
							{selectedItems}
							{bulkMode}
							{collaborationEnabled}
							onToggle={toggleItem}
							onRefresh={() => refreshTrigger++}
						/>
					{/if}
				{/if}
			{/each}
		{/if}
	</div>
</div>

{#if isMoveDialogOpen}
	<MoveDialog
		items={getSelectedItemsDetails()}
		on:cancel={() => (isMoveDialogOpen = false)}
		on:move={handleMoveDialogClose}
	/>
{/if}

{#if isCopyDialogOpen}
	<CopyDialog
		items={getSelectedItemsDetails()}
		on:cancel={() => (isCopyDialogOpen = false)}
		on:copy={handleCopyDialogClose}
	/>
{/if}
