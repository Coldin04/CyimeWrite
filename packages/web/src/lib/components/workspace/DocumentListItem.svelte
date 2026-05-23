<script lang="ts">
	import type { JSONContent } from '@tiptap/core';
	import FileText from '~icons/ph/file-text';
	import Table from '~icons/ph/table';
	import {
		createDocument,
		deleteFile,
		getDocumentDetails,
		updateDocumentImageTarget,
		updateFileName,
		type FileItem
	} from '$lib/api/workspace';
	import DotsThreeVertical from '~icons/ph/dots-three-vertical';
	import Pencil from '~icons/ph/pencil';
	import Trash from '~icons/ph/trash';
	import FolderOpen from '~icons/ph/folder-open';
	import CopySimple from '~icons/ph/copy-simple';
	import SlidersHorizontal from '~icons/ph/sliders-horizontal';
	import { toast } from 'svelte-sonner';
	import { goto } from '$app/navigation';
	import MoveDialog from '$lib/components/workspace/MoveDialog.svelte';
	import CopyDialog from '$lib/components/workspace/CopyDialog.svelte';
	import ConfirmDialog from '$lib/components/common/ConfirmDialog.svelte';
	import ModalDialog from '$lib/components/common/ModalDialog.svelte';
	import EditorDocumentSettingsDialog from '$lib/components/editor/EditorDocumentSettingsDialog.svelte';
	import ExportControls from '$lib/components/editor/ExportControls.svelte';
	import ExportPrivateImagesDialog from '$lib/components/editor/ExportPrivateImagesDialog.svelte';
	import {
		getDocumentImageTargetOptions,
		type DocumentImageTargetOption
	} from '$lib/components/editor/documentImageTargets';
	import { getImageBedConfigs, type ImageBedConfig } from '$lib/api/user';
	import {
		getDocumentContent,
		updateDocumentContent
	} from '$lib/api/editor';
	import type { ExportAction } from '$lib/export/exportActions';
	import { exportActionRequiresPublicImageURLs } from '$lib/export/exportActions';
	import { collectManagedImages } from '$lib/export/exportPrivateImages';
	import {
		createExportCopyTitle,
		normalizeManagedImagesForSave,
		performDocumentExport,
		prepareExportContentWithPublicImages,
		resolveExportErrorMessage
	} from '$lib/export/documentExportWorkflow';
	import { clickOutside } from '$lib/actions/clickOutside';
	import * as m from '$paraglide/messages';
	import { getLocale } from '$paraglide/runtime';

	let {
		item,
		selectedItems,
		bulkMode = false,
		onToggle,
		onRefresh,
		iconKind = 'document',
		collaborationEnabled = false
	}: {
		item: FileItem;
		selectedItems: { [key:string]: boolean };
		bulkMode?: boolean;
		onToggle: (id: string) => void;
		onRefresh?: () => void;
		iconKind?: 'document' | 'table';
		collaborationEnabled?: boolean;
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
	let documentTitle = $state('');
	let documentExcerpt = $state('');
	let documentManualExcerpt = $state('');
	let documentType = $state('rich_text');
	let documentPreferredImageTargetId = $state('managed-r2');
	let documentPublicAccess = $state('private');
	let documentPublicUrl = $state('');
	let documentRole = $state('owner');
	let imageBedConfigs = $state<ImageBedConfig[]>([]);
	let isDocumentSettingsOpen = $state(false);
	let isLoadingDocumentSettings = $state(false);
	let isUpdatingImageTarget = $state(false);
	let isExporting = $state(false);
	let isExportPrivateImagesDialogOpen = $state(false);
	let isPreparingExport = $state(false);
	let pendingExportAction = $state<ExportAction | null>(null);
	let pendingExportContent = $state<JSONContent | null>(null);
	let exportTargetId = $state('');
	let manualCopyContent = $state('');
	let manualCopyTitle = $state('');
	const isMovingItem = $derived(isMoving && item.type === 'document');
	const isCopyingItem = $derived(isCopying && item.type === 'document');
	const canEditDocumentMeta = $derived(documentRole === 'owner' || documentRole === 'collaborator');
	const canManageDocumentMembers = $derived(
		collaborationEnabled && (documentRole === 'owner' || documentRole === 'collaborator')
	);
	const availableImageTargets = $derived<DocumentImageTargetOption[]>(
		getDocumentImageTargetOptions(imageBedConfigs)
	);
	const exportImageTargetOptions = $derived(
		availableImageTargets.filter((option) => option.id !== 'managed-r2')
	);
	const menuClass = $derived(
		menuPlacement === 'context'
			? 'fixed z-40 w-44 origin-top-left rounded-md bg-white py-1 shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none dark:bg-zinc-800 dark:ring-zinc-700'
			: 'absolute top-full right-0 z-20 mt-1 w-44 origin-top-right rounded-md bg-white py-1 shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none dark:bg-zinc-800 dark:ring-zinc-700'
	);
	const menuStyle = $derived(
		menuPlacement === 'context' ? `left: ${contextMenuX}px; top: ${contextMenuY}px;` : ''
	);

	$effect(() => {
		documentTitle = item.title || '';
		documentExcerpt = item.excerpt || '';
		documentManualExcerpt = item.manualExcerpt || '';
		documentType = item.documentType || 'rich_text';
		documentPreferredImageTargetId = item.preferredImageTargetId || 'managed-r2';
		documentPublicAccess = item.publicAccess || 'private';
		documentPublicUrl = item.publicUrl || '';
		documentRole = item.myRole || 'owner';
	});

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
			goto(`/edit/documents/${item.id}`);
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
		const menuHeight = 280;
		contextMenuX = Math.max(margin, Math.min(event.clientX, window.innerWidth - menuWidth - margin));
		contextMenuY = Math.max(margin, Math.min(event.clientY, window.innerHeight - menuHeight - margin));
		menuPlacement = 'context';
		showMenu = true;
	}

	function startEditing() {
		editingName = documentTitle || '';
		isEditing = true;
		showMenu = false;
	}

	async function saveEditing() {
		if (!editingName.trim() || editingName === documentTitle) {
			isEditing = false;
			return;
		}

		try {
			await updateFileName(item.id, 'document', editingName.trim());
			documentTitle = editingName.trim();
			toast.success(m.document_rename_success());
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
			await deleteFile(item.id, 'document');
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

	async function ensureImageBedConfigs() {
		if (imageBedConfigs.length > 0) return;
		imageBedConfigs = await getImageBedConfigs();
	}

	async function loadDocumentSettings() {
		if (isLoadingDocumentSettings) return;
		isLoadingDocumentSettings = true;
		try {
			const [details, configs] = await Promise.all([getDocumentDetails(item.id), getImageBedConfigs()]);
			documentTitle = details.title || '';
			documentExcerpt = details.excerpt || '';
			documentManualExcerpt = details.manualExcerpt || '';
			documentType = details.documentType || 'rich_text';
			documentPreferredImageTargetId = details.preferredImageTargetId || 'managed-r2';
			documentPublicAccess = details.publicAccess || 'private';
			documentPublicUrl = details.publicUrl || '';
			documentRole = details.myRole || documentRole;
			imageBedConfigs = configs;
		} catch (error) {
			console.error('Failed to load document settings:', error);
			toast.error(error instanceof Error && error.message.trim() !== '' ? error.message : m.common_unknown_error());
		} finally {
			isLoadingDocumentSettings = false;
		}
	}

	function startDocumentSettings() {
		showMenu = false;
		isDocumentSettingsOpen = true;
		void loadDocumentSettings();
	}

	async function handleDocumentImageTargetChange(nextTargetId: string) {
		if (isUpdatingImageTarget || nextTargetId === documentPreferredImageTargetId) {
			return;
		}

		isUpdatingImageTarget = true;
		try {
			const updated = await updateDocumentImageTarget(item.id, nextTargetId);
			documentPreferredImageTargetId = updated.preferredImageTargetId;
			toast.success(m.editor_image_target_updated());
			onRefresh?.();
		} catch (error) {
			console.error('Failed to update document image target:', error);
			toast.error(
				error instanceof Error && error.message.trim() !== ''
					? error.message
					: m.editor_image_target_update_failed()
			);
		} finally {
			isUpdatingImageTarget = false;
		}
	}

	function updateDocumentTitleFromSettings(nextTitle: string) {
		documentTitle = nextTitle;
		onRefresh?.();
	}

	function updateDocumentExcerptFromSettings(nextExcerpt: string) {
		documentManualExcerpt = nextExcerpt;
		documentExcerpt = nextExcerpt || documentExcerpt;
		onRefresh?.();
	}

	function updateDocumentPublicAccessFromSettings(nextPublicAccess: string, nextPublicUrl: string) {
		documentPublicAccess = nextPublicAccess;
		documentPublicUrl = nextPublicUrl;
		onRefresh?.();
	}

	function closeManualCopyDialog() {
		manualCopyContent = '';
		manualCopyTitle = '';
	}

	async function retryManualCopy() {
		try {
			await navigator.clipboard.writeText(manualCopyContent);
			toast.success(m.editor_export_manual_copy_success());
			closeManualCopyDialog();
		} catch {
			toast.error(m.editor_export_manual_copy_failed());
		}
	}

	function setManualCopy(title: string, content: string) {
		manualCopyTitle = title;
		manualCopyContent = content;
	}

	function closeExportPrivateImagesDialog() {
		if (isPreparingExport) return;
		isExportPrivateImagesDialogOpen = false;
		pendingExportAction = null;
		pendingExportContent = null;
		exportTargetId = '';
	}

	async function finalizeExportWithProcessedContent(exportContent: JSONContent) {
		if (!pendingExportAction) return;
		const action = pendingExportAction;
		isExportPrivateImagesDialogOpen = false;
		pendingExportAction = null;
		pendingExportContent = null;
		exportTargetId = '';
		await performDocumentExport({
			action,
			title: documentTitle,
			contentJson: exportContent,
			onManualCopy: setManualCopy
		});
	}

	async function handleExportWithSaveAs() {
		if (!pendingExportAction || !exportTargetId || isPreparingExport) return;

		isPreparingExport = true;
		try {
			if (!pendingExportContent) {
				throw new Error('Missing export content');
			}
			const exportContent = await prepareExportContentWithPublicImages({
				documentId: item.id,
				contentJson: pendingExportContent,
				targetId: exportTargetId,
				toastId: 'workspace-export-private-images'
			});
			await createDocument({
				title: createExportCopyTitle(documentTitle),
				contentJson: normalizeManagedImagesForSave(exportContent) as { [key: string]: unknown },
				folderId: item.folderId ?? null,
				documentType: documentType === 'table' ? 'table' : 'rich_text',
				preferredImageTargetId: exportTargetId
			});
			onRefresh?.();
			await finalizeExportWithProcessedContent(exportContent);
		} catch (error) {
			console.error('[Workspace Export] Failed to create export copy:', error);
			toast.error(resolveExportErrorMessage(error));
		} finally {
			isPreparingExport = false;
		}
	}

	async function handleExportWithReplace() {
		if (!pendingExportAction || !exportTargetId || isPreparingExport) return;

		isPreparingExport = true;
		try {
			if (!pendingExportContent) {
				throw new Error('Missing export content');
			}
			const exportContent = await prepareExportContentWithPublicImages({
				documentId: item.id,
				contentJson: pendingExportContent,
				targetId: exportTargetId,
				toastId: 'workspace-export-private-images'
			});
			await updateDocumentContent(item.id, normalizeManagedImagesForSave(exportContent));
			const targetResult = await updateDocumentImageTarget(item.id, exportTargetId);
			documentPreferredImageTargetId = targetResult.preferredImageTargetId;
			onRefresh?.();
			await finalizeExportWithProcessedContent(exportContent);
		} catch (error) {
			console.error('[Workspace Export] Failed to replace private images for export:', error);
			toast.error(resolveExportErrorMessage(error));
		} finally {
			isPreparingExport = false;
		}
	}

	async function handleExportAction(action: ExportAction) {
		if (isExporting || isPreparingExport) return;

		showMenu = false;
		isExporting = true;
		try {
			const contentResponse = await getDocumentContent(item.id);
			const exportContent = contentResponse.contentJson;
			const managedImages = collectManagedImages(exportContent);

			if (!exportActionRequiresPublicImageURLs(action) || managedImages.length === 0) {
				await performDocumentExport({
					action,
					title: documentTitle,
					contentJson: exportContent,
					onManualCopy: setManualCopy
				});
				return;
			}

			await ensureImageBedConfigs();
			if (exportImageTargetOptions.length === 0) {
				toast.error(m.editor_export_private_images_config_required());
				return;
			}

			const preferredTarget = exportImageTargetOptions.find(
				(option) => option.id === documentPreferredImageTargetId
			);
			exportTargetId = preferredTarget?.id ?? exportImageTargetOptions[0].id;
			pendingExportAction = action;
			pendingExportContent = exportContent;
			isExportPrivateImagesDialogOpen = true;
		} catch (error) {
			console.error('[Workspace Export] Failed to prepare export:', error);
			toast.error(error instanceof Error && error.message.trim() !== '' ? error.message : m.editor_export_failed());
		} finally {
			isExporting = false;
		}
	}
</script>

<div
	role="button"
	tabindex="0"
	class="group flex cursor-pointer items-center justify-between border-b border-zinc-200 px-4 py-3 transition-colors hover:bg-gradient-to-r hover:from-blue-50/50 hover:to-transparent dark:border-zinc-700 dark:hover:bg-none dark:hover:bg-zinc-800/60 {isSelected
		? 'bg-blue-50 dark:bg-blue-900/30'
		: ''}"
	onclick={handleClick}
	onkeydown={handleKeyDown}
	oncontextmenu={openContextMenu}
>
	<!-- Left Side: Name -->
	<div class="flex min-w-0 items-start gap-3 pr-4">
		<input
			type="checkbox"
			class={checkboxClasses}
			checked={isSelected}
			onclick={(e) => e.stopPropagation()}
			onchange={() => onToggle(item.id)}
		/>
		{#if iconKind === 'table'}
			<Table class="mt-0.5 h-5 w-5 flex-shrink-0 text-blue-500 dark:text-blue-400" />
		{:else}
			<FileText class="mt-0.5 h-5 w-5 flex-shrink-0 text-blue-500 dark:text-blue-400" />
		{/if}
		<div class="min-w-0">
			<span class="block truncate font-normal text-zinc-800 dark:text-zinc-200">{documentTitle}</span>
			{#if documentExcerpt}
				<p class="mt-0.5 truncate text-xs text-zinc-500 dark:text-zinc-400">{documentExcerpt}</p>
			{/if}
		</div>
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
					<ExportControls
						variant="menuitem"
						onAction={(action) => {
							void handleExportAction(action);
						}}
					/>
					<button
						onclick={startDocumentSettings}
						class="flex w-full items-center gap-2 px-4 py-2 text-sm text-zinc-700 hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-700"
						role="menuitem"
					>
						<SlidersHorizontal class="h-4 w-4" />
						<span>{m.editor_image_target_menu_title()}</span>
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

{#if isDocumentSettingsOpen}
	<EditorDocumentSettingsDialog
		documentId={item.id}
		documentTitle={documentTitle}
		documentManualExcerpt={documentManualExcerpt}
		{documentType}
		currentTargetId={documentPreferredImageTargetId}
		options={availableImageTargets}
		canEditBasic={canEditDocumentMeta}
		canManageMembers={canManageDocumentMembers}
		canEditImageSettings={canEditDocumentMeta}
		canManagePublic={documentRole === 'owner'}
		publicAccess={documentPublicAccess}
		publicUrl={documentPublicUrl}
		isUpdating={isUpdatingImageTarget || isLoadingDocumentSettings}
		trigger="none"
		initialOpen={true}
		onSelect={handleDocumentImageTargetChange}
		onTitleChange={updateDocumentTitleFromSettings}
		onManualExcerptChange={updateDocumentExcerptFromSettings}
		onPublicAccessChange={updateDocumentPublicAccessFromSettings}
		onOpenChange={(open) => {
			if (!open) {
				isDocumentSettingsOpen = false;
			}
		}}
	/>
{/if}

<ExportPrivateImagesDialog
	open={isExportPrivateImagesDialogOpen}
	imageCount={pendingExportContent ? collectManagedImages(pendingExportContent).length : 0}
	targetOptions={exportImageTargetOptions}
	selectedTargetId={exportTargetId}
	busy={isPreparingExport}
	onTargetChange={(nextTargetId) => {
		exportTargetId = nextTargetId;
	}}
	onCancel={closeExportPrivateImagesDialog}
	onSaveAs={handleExportWithSaveAs}
	onReplace={handleExportWithReplace}
/>

<ModalDialog
	open={manualCopyContent.trim() !== ''}
	title={manualCopyTitle || m.editor_export_manual_copy_title()}
	maxWidthClass="max-w-3xl"
	onClose={closeManualCopyDialog}
>
	<div class="space-y-4">
		<div>
			<h2 class="text-lg font-semibold text-zinc-900 dark:text-zinc-100">
				{m.editor_export_manual_copy_title()}
			</h2>
			<p class="mt-1 text-sm leading-6 text-zinc-600 dark:text-zinc-300">
				{m.editor_export_manual_copy_description()}
			</p>
		</div>
		<textarea
			readonly
			spellcheck="false"
			value={manualCopyContent}
			class="h-72 w-full resize-none rounded-md border border-zinc-200 bg-zinc-50 p-3 font-mono text-xs leading-5 text-zinc-800 outline-none dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100"
			onfocus={(event) => event.currentTarget.select()}
		></textarea>
		<div class="flex justify-end gap-2">
			<button
				type="button"
				class="inline-flex h-8 items-center rounded-md px-3 text-sm text-zinc-700 transition-colors hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-800"
				onclick={closeManualCopyDialog}
			>
				{m.common_cancel()}
			</button>
			<button
				type="button"
				class="inline-flex h-8 items-center rounded-md bg-zinc-900 px-3 text-sm font-medium text-white transition-colors hover:bg-zinc-800 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200"
				onclick={retryManualCopy}
			>
				{m.editor_export_manual_copy_action()}
			</button>
		</div>
	</div>
</ModalDialog>

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
			<h3 class="mb-4 text-lg font-medium text-zinc-900 dark:text-zinc-100">{m.document_rename_title()}</h3>
			<input
				type="text"
				value={editingName}
				oninput={(e) => editingName = e.currentTarget.value}
				onkeydown={handleEditingKeydown}
				class="mb-4 w-full rounded-md border border-zinc-300 px-3 py-2 text-base text-zinc-900 focus:border-blue-500 focus:outline-none dark:border-zinc-600 dark:bg-zinc-700 dark:text-zinc-100"
				placeholder={m.document_name_placeholder()}
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

<ConfirmDialog
	open={isDeleteConfirmOpen}
	title={m.common_delete()}
	message={m.document_delete_confirm()}
	confirmText={m.common_delete()}
	onCancel={closeDeleteConfirm}
	onConfirm={confirmDelete}
/>
