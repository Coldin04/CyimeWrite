<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import type { JSONContent } from '@tiptap/core';
	import { browser } from '$app/environment';
	import { beforeNavigate, goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { get } from 'svelte/store';
	import type { ExportAction } from '$lib/export/exportActions';
	import Editor from '$lib/components/editor/Editor.svelte';
	import EditorTopBar from '$lib/components/editor/EditorTopBar.svelte';
	import ExportPrivateImagesDialog from '$lib/components/editor/ExportPrivateImagesDialog.svelte';
	import ConfirmDialog from '$lib/components/common/ConfirmDialog.svelte';
	import ModalDialog from '$lib/components/common/ModalDialog.svelte';
	import {
		defaultAutoSaveEnabled,
		defaultAutoSaveIntervalSeconds,
		readAutoSaveEnabled,
		readAutoSaveIntervalSeconds
	} from '$lib/components/editor/autoSave';
	import { auth } from '$lib/stores/auth';
	import { apiFetch } from '$lib/api';
	import { resolveApiUrl } from '$lib/config/api';
	import { realtimeConfig } from '$lib/stores/realtime';
	import { yjsProvider, type ProviderInstance } from '$lib/utils/yjsProvider';
	import {
		getDocumentContent,
		pasteDocumentImage,
		resolveAssetReadURLs,
		updateDocumentContent
	} from '$lib/api/editor';
	import {
		createDocument,
		getDocumentDetails,
		listDocumentMembers,
		updateDocumentImageTarget,
		type ShareDocumentMember
	} from '$lib/api/workspace';
	import { getImageBedConfigs, type ImageBedConfig } from '$lib/api/user';
	import {
		getDocumentImageTargetLabel,
		getDocumentImageTargetOptions
	} from '$lib/components/editor/documentImageTargets';
	import {
		buildExportAssetFilename,
		collectImageNodes,
		collectManagedImages,
		cloneContentJson,
		getManagedAssetId,
		inferExportAssetMimeType,
		normalizeManagedImagesForSave,
		replaceManagedImagesWithPublicURLs
	} from '$lib/export/managedImages';
	import {
		ExportCopyError,
		exportHtmlDocument,
		exportPdfDocument,
		inlineManagedImagesAsDataURLs,
		runExportAction
	} from '$lib/export/exportPrivateImages';
	import { exportActionRequiresPublicImageURLs } from '$lib/export/exportActions';
	import { toast } from 'svelte-sonner';
	import * as m from '$paraglide/messages';

	let title = $state('');
	let manualExcerpt = $state('');
	let myRole = $state<'owner' | 'collaborator' | 'editor' | 'viewer' | string>('owner');
	let publicAccess = $state<'private' | 'authenticated' | 'public' | string>('private');
	let publicUrl = $state('');
	let folderId = $state<string | null>(null);
	const EMPTY_DOC: JSONContent = {
		type: 'doc',
		content: [{ type: 'paragraph' }]
	};

	let content = $state<JSONContent>(EMPTY_DOC);
	let documentType = $state<'rich_text' | 'table' | string>('rich_text');
	let preferredImageTargetId = $state('managed-r2');
	let imageBedConfigs = $state<ImageBedConfig[]>([]);
	let isUpdatingImageTarget = $state(false);
	let isSaving = $state(false);
	let lastSaved = $state<Date | null>(null);
	let hasUnsavedChanges = $state(false);
	let isLoading = $state(true);
	let collaboration = $state<ProviderInstance | null>(null);
	let collaborationDocumentId = $state<string | null>(null);
	let documentLoadSequence = 0;
	let collaborationError = $state<string | null>(null);
	let collaborationIndicator = $state<
		{ kind: 'single' | 'single-offline' | 'multi-pending' | 'multi'; label: string } | null
	>(null);
	let detachCollaborationListeners: (() => void) | null = null;
	let presenceCount = $state(0);
	let presenceConnected = $state(false);
	let hasAttemptedPresence = $state(false);
	let documentMembers = $state<ShareDocumentMember[]>([]);
	let onlineMembers = $state<ShareDocumentMember[]>([]);
	let presenceSessionId = $state('');
	let presenceHeartbeatTimer: number | null = null;
	let isInitializingCollaboration = $state(false);
	let lastCollaborationAttemptAt = $state(0);
	let isYjsConnected = $state(false);
	let isLeaveConfirmOpen = $state(false);
	let pendingNavigationUrl = $state<string | null>(null);
	let bypassLeaveGuard = $state(false);
	let isExportPrivateImagesDialogOpen = $state(false);
	let isPreparingExport = $state(false);
	let pendingExportAction = $state<ExportAction | null>(null);
	let exportTargetId = $state('');
	let manualCopyContent = $state('');
	let manualCopyTitle = $state('');
	let autoSaveEnabled = $state(defaultAutoSaveEnabled);
	let autoSaveIntervalSeconds = $state(defaultAutoSaveIntervalSeconds);
	let hasPendingCollaborationSave = $state(false);
	let hasPendingImmediateCollaborationPersist = $state(false);
	let hasManualSaveRequestInFlight = $state(false);
	let localCollaborationChangeSeq = $state(0);
	let flushedCollaborationChangeSeq = $state(0);
	let persistedCollaborationChangeSeq = $state(0);
	let editorContentOverride = $state<{ token: number; content: JSONContent } | null>(null);
	let collaborationContentSnapshotTimer: number | null = null;
	type SaveDriver = 'none' | 'collaboration' | 'local';
	type SaveReason = 'manual' | 'auto' | 'leave' | 'export';
	type SaveWaiter = {
		resolve: (value: boolean) => void;
		timer: number;
	};
	type EditorOverrideWaiter = {
		expectedSerializedContent: string;
		resolve: (value: boolean) => void;
		timer: number;
	};
	let pageSignal = $state(get(page));
	const unsubscribePage = page.subscribe((p) => (pageSignal = p));
	let authSignal = $state(get(auth));
	const unsubscribeAuth = auth.subscribe((state) => (authSignal = state));
	let realtimeConfigSignal = $state(get(realtimeConfig));
	const unsubscribeRealtimeConfig = realtimeConfig.subscribe((state) => (realtimeConfigSignal = state));
	const documentId = $derived(pageSignal.params?.id);
	const collaborationEnabled = $derived(realtimeConfigSignal.config?.collaborationEnabled ?? false);
	const activeCollaboration = $derived(
		collaborationDocumentId === documentId ? collaboration : null
	);
	let collaborationSaveWaiters: SaveWaiter[] = [];
	let editorContentOverrideWaiter: EditorOverrideWaiter | null = null;
	let nextEditorContentOverrideToken = 0;
	const saveDriver = $derived.by<SaveDriver>(() => {
		if (!documentId || isLoading) {
			return 'none';
		}
		return collaborationEnabled && activeCollaboration?.provider && isYjsConnected
			? 'collaboration'
			: 'local';
	});
	const availableImageTargets = $derived(getDocumentImageTargetOptions(imageBedConfigs));
	const exportImageTargetOptions = $derived(
		availableImageTargets.filter((option) => option.id !== 'managed-r2')
	);
	const currentImageTargetLabel = $derived(
		getDocumentImageTargetLabel(preferredImageTargetId, availableImageTargets)
	);

	type ImageNodeRecord = Record<string, unknown> & {
		attrs?: Record<string, unknown>;
	};

	async function refreshSignedImageSources(input: JSONContent): Promise<JSONContent> {
		const cloned = cloneContentJson(input);
		const imageNodes: ImageNodeRecord[] = [];
		collectImageNodes(cloned, imageNodes);
		if (imageNodes.length === 0) {
			return cloned;
		}

		const assetIds = Array.from(
			new Set(
				imageNodes
					.map((node) => getManagedAssetId((node.attrs ?? {}) as Record<string, unknown>))
					.filter((value): value is string => value !== null)
			)
		);
		if (assetIds.length === 0) {
			return cloned;
		}

		let resolved: Awaited<ReturnType<typeof resolveAssetReadURLs>> | null = null;
		try {
			resolved = await resolveAssetReadURLs(assetIds);
		} catch (error) {
			console.error('[Load] Failed to resolve image URLs:', error);
			return cloned;
		}
		if (!resolved) {
			return cloned;
		}
		const resolvedMap = new Map(
			resolved.items
				.filter((item) => item.assetId && item.url)
				.map((item) => [item.assetId, item.url as string])
		);

		for (const node of imageNodes) {
			const attrs = (node.attrs ?? {}) as Record<string, unknown>;
			const assetId = getManagedAssetId(attrs);
			if (!assetId) continue;
			const resolvedURL = resolvedMap.get(assetId);
			if (!resolvedURL) {
				console.error('[Load] Failed to resolve image URL for asset:', assetId);
				continue;
			}
			attrs.src = resolvedURL;
			node.attrs = attrs;
		}

		return cloned;
	}

	function serializeComparableContent(input: JSONContent): string {
		return JSON.stringify(normalizeManagedImagesForSave(input));
	}

	function clearCollaborationContentSnapshotTimer() {
		if (collaborationContentSnapshotTimer !== null) {
			window.clearTimeout(collaborationContentSnapshotTimer);
			collaborationContentSnapshotTimer = null;
		}
	}

	function sendCollaborationContentSnapshot(snapshotContent: JSONContent): void {
		const provider = collaboration?.provider;
		if (!collaborationEnabled || !provider || !isYjsConnected || !documentId) {
			return;
		}

		try {
			provider.sendStateless(
				JSON.stringify({
					type: 'document-content-snapshot',
					documentId,
					contentJson: normalizeManagedImagesForSave(snapshotContent)
				})
			);
		} catch (error) {
			console.warn('[Collaboration] Failed to send canonical content snapshot:', error);
		}
	}

	async function requestImmediateCollaborationPersist(): Promise<boolean> {
		if (!collaborationEnabled || !isYjsConnected || !documentId) {
			return false;
		}

		try {
			const wsUrl = await resolveRealtimeWsUrl();
			const url = new URL(wsUrl);
			url.protocol = url.protocol === 'wss:' ? 'https:' : 'http:';
			url.pathname = '/api/v1/realtime/persist-now';
			url.search = '';
			url.searchParams.set('documentId', documentId);

			const response = await apiFetch(url.toString(), {
				method: 'POST'
			});
			if (!response.ok) {
				const errorText = await response.text();
				throw new Error(
					`Immediate collaboration persist failed: ${response.status}${errorText ? ` ${errorText}` : ''}`
				);
			}
			return true;
		} catch (error) {
			console.warn('[Collaboration] Failed to request immediate persist:', error);
			return false;
		}
	}

	function scheduleCollaborationContentSnapshot(snapshotContent: JSONContent): void {
		if (!browser || !collaborationEnabled) {
			return;
		}

		clearCollaborationContentSnapshotTimer();
		collaborationContentSnapshotTimer = window.setTimeout(() => {
			collaborationContentSnapshotTimer = null;
			sendCollaborationContentSnapshot(snapshotContent);
		}, 300);
	}

	function logSaveDebug(event: string, details: Record<string, unknown> = {}) {
		console.debug('[SaveDebug]', event, {
			at: new Date().toISOString(),
			documentId,
			saveDriver,
			hasUnsavedChanges,
			hasPendingCollaborationSave,
			isSaving,
			isYjsConnected,
			...details
		});
	}

	function createExportCopyTitle(value: string): string {
		const trimmed = value.trim();
		return trimmed === '' ? 'Untitled Export' : `${trimmed} (Export)`;
	}

	async function performExport(action: ExportAction, exportContent: JSONContent) {
		try {
			if (action === 'download-pdf') {
				const printContent = await inlineManagedImagesAsDataURLs(exportContent, (assetId) =>
					resolveApiUrl(`/api/v1/media/assets/${assetId}/content`)
				);
				const html = await exportHtmlDocument({
					title: title.trim() || 'Cyime Export',
					contentJson: printContent,
					colorMode: 'light'
				});
				await exportPdfDocument({
					title: title.trim() || 'Cyime Export',
					html
				});
				return;
			}

			const result = await runExportAction(action, {
				title,
				contentJson: exportContent
			});
			if (action === 'copy-markdown' && result === 'copied') {
				toast.success(m.editor_export_markdown_copied());
				return;
			}
			if (action === 'copy-bbcode' && result === 'copied') {
				toast.success(m.editor_export_bbcode_copied());
			}
		} catch (error) {
			console.error('[Export] Failed to export document:', error);
			if (error instanceof ExportCopyError && error.message === 'copy_markdown_failed') {
				manualCopyTitle = m.editor_export_copy_markdown();
				manualCopyContent = error.content;
				toast.error(m.editor_export_markdown_copy_failed());
				return;
			}
			if (error instanceof ExportCopyError && error.message === 'copy_bbcode_failed') {
				manualCopyTitle = m.editor_export_copy_bbcode();
				manualCopyContent = error.content;
				toast.error(m.editor_export_bbcode_copy_failed());
				return;
			}
			if (error instanceof Error && error.message === 'copy_markdown_failed') {
				toast.error(m.editor_export_markdown_copy_failed());
				return;
			}
			if (error instanceof Error && error.message === 'copy_bbcode_failed') {
				toast.error(m.editor_export_bbcode_copy_failed());
				return;
			}
			toast.error(m.editor_export_failed());
		}
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

	async function prepareExportContentWithPublicImages(targetId: string): Promise<JSONContent> {
		if (!documentId) {
			throw new Error('Missing document id');
		}

		const contentSnapshot = cloneContentJson(content);
		const managedImages = collectManagedImages(contentSnapshot);
		if (managedImages.length === 0) {
			return contentSnapshot;
		}

		toast.loading(m.editor_export_prepare_images({ current: 0, total: managedImages.length }), {
			id: 'export-private-images',
			duration: Infinity
		});

		try {
			const publicURLByAssetID = new Map<string, string>();

			for (let index = 0; index < managedImages.length; index += 1) {
				const item = managedImages[index];
				toast.loading(m.editor_export_prepare_images({ current: index + 1, total: managedImages.length }), {
					id: 'export-private-images',
					duration: Infinity
				});

				const response = await apiFetch(`/api/v1/media/assets/${item.assetId}/content`);
				if (!response.ok) {
					throw new Error(`Failed to fetch private image ${item.assetId}`);
				}

				const blob = await response.blob();
				const mimeType = inferExportAssetMimeType(
					blob.type || response.headers.get('content-type') || '',
					item.title ?? item.alt,
					item.src
				);
				const file = new File(
					[blob],
					buildExportAssetFilename(item.assetId, mimeType, item.title ?? item.alt),
					{ type: mimeType }
				);
				const uploaded = await pasteDocumentImage(documentId, file, { targetId });
				publicURLByAssetID.set(item.assetId, uploaded.url);
			}

			return replaceManagedImagesWithPublicURLs(contentSnapshot, publicURLByAssetID);
		} finally {
			toast.dismiss('export-private-images');
		}
	}

	function resolveExportErrorMessage(error: unknown): string {
		const apiError = error as { code?: string; status?: number; message?: string } | undefined;
		const message = error instanceof Error ? error.message : (apiError?.message ?? '');
		if (
			apiError?.code === 'DOCUMENT_IMAGE_PROVIDER_UPLOAD_FAILED' &&
			/timeout|awaiting response headers|deadline exceeded/i.test(message)
		) {
			return m.editor_export_image_bed_timeout();
		}
		if (apiError?.code === 'DOCUMENT_IMAGE_PROVIDER_UPLOAD_FAILED') {
			return m.editor_export_image_bed_upload_failed();
		}
		return message.trim() !== '' ? message : m.editor_export_failed();
	}

	function closeExportPrivateImagesDialog() {
		if (isPreparingExport) {
			return;
		}
		isExportPrivateImagesDialogOpen = false;
		pendingExportAction = null;
		exportTargetId = '';
	}

	async function finalizeExportWithProcessedContent(exportContent: JSONContent) {
		if (!pendingExportAction) {
			return;
		}
		const action = pendingExportAction;
		isExportPrivateImagesDialogOpen = false;
		pendingExportAction = null;
		exportTargetId = '';
		await performExport(action, exportContent);
	}

	async function handleExportWithSaveAs() {
		if (!pendingExportAction || !documentId || !exportTargetId || isPreparingExport) {
			return;
		}

		isPreparingExport = true;
		try {
			const exportContent = await prepareExportContentWithPublicImages(exportTargetId);
			await createDocument({
				title: createExportCopyTitle(title),
				contentJson: normalizeManagedImagesForSave(exportContent) as { [key: string]: unknown },
				folderId,
				documentType: documentType === 'table' ? 'table' : 'rich_text',
				preferredImageTargetId: exportTargetId
			});
			await finalizeExportWithProcessedContent(exportContent);
		} catch (error) {
			console.error('[Export] Failed to create export copy:', error);
			toast.error(resolveExportErrorMessage(error));
		} finally {
			isPreparingExport = false;
		}
	}

	async function handleExportWithReplace() {
		if (!pendingExportAction || !documentId || !exportTargetId || isPreparingExport) {
			return;
		}

		isPreparingExport = true;
		try {
			const exportContent = await prepareExportContentWithPublicImages(exportTargetId);
			const applied = await applyEditorContentOverride(exportContent);
			if (!applied) {
				throw new Error('Failed to apply export content into the editor');
			}

			const saved = await requestDocumentSave('export');
			if (!saved) {
				throw new Error('Failed to persist export content');
			}

			const targetResult = await updateDocumentImageTarget(documentId, exportTargetId);
			preferredImageTargetId = targetResult.preferredImageTargetId;
			await finalizeExportWithProcessedContent(exportContent);
		} catch (error) {
			console.error('[Export] Failed to replace private images for export:', error);
			toast.error(resolveExportErrorMessage(error));
		} finally {
			isPreparingExport = false;
		}
	}

	async function handleExportAction(action: ExportAction) {
		if (hasUnsavedChanges || isSaving || hasPendingCollaborationSave) {
			const saved = await requestDocumentSave('export');
			if (!saved) {
				toast.error(m.editor_save_failed());
				return;
			}
		}

		const managedImages = collectManagedImages(content);
		if (!exportActionRequiresPublicImageURLs(action) || managedImages.length === 0) {
			await performExport(action, content);
			return;
		}

		if (exportImageTargetOptions.length === 0) {
			toast.error(m.editor_export_private_images_config_required());
			return;
		}

		const preferredTarget = exportImageTargetOptions.find((option) => option.id === preferredImageTargetId);
		exportTargetId = preferredTarget?.id ?? exportImageTargetOptions[0].id;
		pendingExportAction = action;
		isExportPrivateImagesDialogOpen = true;
	}

	beforeNavigate((navigation) => {
		if (documentId && navigation.to?.url) {
			void unregisterPresenceSession(documentId, { keepalive: true });
		}

		if (!browser || !hasUnsavedChanges || bypassLeaveGuard) {
			return;
		}
		if (!navigation.to?.url) return;

		navigation.cancel();
		pendingNavigationUrl = `${navigation.to.url.pathname}${navigation.to.url.search}${navigation.to.url.hash}`;
		isLeaveConfirmOpen = true;
	});

	function handleCancelLeave() {
		isLeaveConfirmOpen = false;
		pendingNavigationUrl = null;
	}

	async function handleConfirmLeave() {
		if (!pendingNavigationUrl) {
			handleCancelLeave();
			return;
		}

		const target = pendingNavigationUrl;
		isLeaveConfirmOpen = false;
		pendingNavigationUrl = null;
		bypassLeaveGuard = true;
		if (documentId) {
			await unregisterPresenceSession(documentId, { keepalive: true });
		}
		await goto(target);
		bypassLeaveGuard = false;
	}

	async function handleLeaveWithoutSave() {
		await handleConfirmLeave();
	}

	function handleContentChange(
		newContent: JSONContent,
		meta: { viaCollaboration: boolean; isLocalChange: boolean } = {
			viaCollaboration: false,
			isLocalChange: true
		}
	) {
		const isActiveCollaborationChange = meta.viaCollaboration && Boolean(activeCollaboration) && isYjsConnected;
		if (isLoading) return;
		if (serializeComparableContent(content) === serializeComparableContent(newContent)) {
			return;
		}
		content = newContent;
		if (
			editorContentOverrideWaiter &&
			serializeComparableContent(newContent) === editorContentOverrideWaiter.expectedSerializedContent
		) {
			window.clearTimeout(editorContentOverrideWaiter.timer);
			editorContentOverrideWaiter.resolve(true);
			editorContentOverrideWaiter = null;
		}
		if (!(isActiveCollaborationChange && !meta.isLocalChange)) {
			hasUnsavedChanges = true;
			if (isActiveCollaborationChange) {
				localCollaborationChangeSeq += 1;
				hasPendingCollaborationSave = true;
				// A fresh local edit supersedes any previous "waiting for persist"
				// state. While the user is actively typing, show the document as
				// dirty rather than stuck in "saving".
				isSaving = false;
				logSaveDebug('collaboration-local-change', {
					localCollaborationChangeSeq,
					meta
				});
			}
		}
		if (isActiveCollaborationChange) {
			scheduleCollaborationContentSnapshot(newContent);
		}
		if (documentId) {
			void connectPresenceSocket(documentId);
			if (!collaboration) {
				void startCollaboration(documentId, 'editing');
			}
		}
	}

	function handleTitleChange(newTitle: string) {
		title = newTitle;
	}

	function handleExcerptChange(newExcerpt: string) {
		manualExcerpt = newExcerpt;
	}

	function handlePublicAccessChange(nextPublicAccess: string, nextPublicURL: string) {
		publicAccess = nextPublicAccess;
		publicUrl = nextPublicURL;
	}

	function settleCollaborationSaveWaiters(value: boolean) {
		for (const waiter of collaborationSaveWaiters) {
			window.clearTimeout(waiter.timer);
			waiter.resolve(value);
		}
		collaborationSaveWaiters = [];
		hasPendingImmediateCollaborationPersist = false;
		hasManualSaveRequestInFlight = false;
	}

	function settleEditorContentOverrideWaiter(value: boolean) {
		if (!editorContentOverrideWaiter) {
			return;
		}

		window.clearTimeout(editorContentOverrideWaiter.timer);
		editorContentOverrideWaiter.resolve(value);
		editorContentOverrideWaiter = null;
	}

	async function applyEditorContentOverride(nextContent: JSONContent): Promise<boolean> {
		if (serializeComparableContent(content) === serializeComparableContent(nextContent)) {
			return true;
		}

		settleEditorContentOverrideWaiter(false);

		const result = new Promise<boolean>((resolve) => {
			const timer = window.setTimeout(() => {
				editorContentOverrideWaiter = null;
				resolve(false);
			}, 5000);
			editorContentOverrideWaiter = {
				expectedSerializedContent: serializeComparableContent(nextContent),
				resolve,
				timer
			};
		});

		const token = ++nextEditorContentOverrideToken;
		editorContentOverride = { token, content: nextContent };
		await tick();
		window.setTimeout(() => {
			if (
				editorContentOverrideWaiter &&
				editorContentOverrideWaiter.expectedSerializedContent === serializeComparableContent(nextContent)
			) {
				content = nextContent;
				hasUnsavedChanges = true;
				if (saveDriver === 'collaboration') {
					localCollaborationChangeSeq += 1;
					hasPendingCollaborationSave = true;
				}
				settleEditorContentOverrideWaiter(true);
			}
		}, 0);
		return result;
	}

	function queueCollaborationSaveWaiter(): Promise<boolean> {
		return new Promise<boolean>((resolve) => {
			const timer = window.setTimeout(() => {
				collaborationSaveWaiters = collaborationSaveWaiters.filter((entry) => entry.timer !== timer);
				resolve(false);
			}, 20000);
			collaborationSaveWaiters = [...collaborationSaveWaiters, { resolve, timer }];
		});
	}

	function getCurrentUserMember(): ShareDocumentMember | null {
		const user = authSignal.user;
		if (!user?.id) {
			return null;
		}

		return {
			userId: user.id,
			role: myRole,
			displayName:
				(typeof user.displayName === 'string' && user.displayName.trim() !== ''
					? user.displayName.trim()
					: null) ??
				(typeof user.email === 'string' && user.email.trim() !== '' ? user.email.trim() : null),
			email: typeof user.email === 'string' && user.email.trim() !== '' ? user.email.trim() : null
		};
	}

	function setOnlineMembersFromUserIds(userIds: string[]) {
		const seen = new Set<string>();
		const nextMembers: ShareDocumentMember[] = [];
		const membersById = new Map(documentMembers.map((member) => [member.userId, member]));

		for (const userId of userIds) {
			if (!userId || seen.has(userId)) {
				continue;
			}
			seen.add(userId);
			const matchedMember = membersById.get(userId);
			if (matchedMember) {
				nextMembers.push(matchedMember);
				continue;
			}

			if (userId === authSignal.user?.id) {
				const currentUserMember = getCurrentUserMember();
				if (currentUserMember) {
					nextMembers.push(currentUserMember);
					continue;
				}
			}

			nextMembers.push({
				userId,
				role: 'editor',
				displayName: userId,
				email: null
			});
		}

		onlineMembers = nextMembers;
	}

	function resetOnlineMembers() {
		const currentUserMember = getCurrentUserMember();
		onlineMembers = currentUserMember ? [currentUserMember] : [];
	}

	function updateCollaborationIndicator() {
		if (!collaborationEnabled) {
			collaborationIndicator = null;
			return;
		}

		if (presenceCount > 1) {
			if (isYjsConnected) {
				collaborationIndicator = {
					kind: 'multi',
					label: m.editor_collaboration_connected_multi({ count: String(presenceCount) })
				};
				return;
			}

			collaborationIndicator = {
				kind: 'multi-pending',
				label: m.editor_collaboration_pending_multi({ count: String(presenceCount) })
			};
			return;
		}

		if (collaborationError || (hasAttemptedPresence && !presenceConnected && !collaboration)) {
			collaborationIndicator = {
				kind: 'single-offline',
				label: m.editor_collaboration_disconnected_single_mode()
			};
			return;
		}

		collaborationIndicator = { kind: 'single', label: m.editor_collaboration_single_online() };
	}

	function ensurePresenceSessionId() {
		if (presenceSessionId !== '') {
			return presenceSessionId;
		}
		presenceSessionId = crypto.randomUUID();
		return presenceSessionId;
	}

	async function resolveRealtimeWsUrl(): Promise<string> {
		if (!auth.getAccessToken()) {
			throw new Error('Missing access token');
		}

		let wsUrl = get(realtimeConfig).config?.realtimeWsUrl ?? '';
		if (!wsUrl) {
			await realtimeConfig.reload();
			wsUrl = get(realtimeConfig).config?.realtimeWsUrl ?? '';
		}
		if (!wsUrl) {
			throw new Error('Realtime WebSocket URL is not configured');
		}

		return wsUrl;
	}

	function buildRealtimePresenceURL(nextDocumentId: string): string {
		const url = new URL(resolveApiUrl('/api/v1/workspace/documents/_/presence'), window.location.origin);
		url.pathname = url.pathname.replace('/_', `/${nextDocumentId}`);
		url.search = '';
		return url.toString();
	}

	async function fetchCollaborationPresence(nextDocumentId: string): Promise<number> {
		if (!collaborationEnabled) {
			return 0;
		}

		const response = await apiFetch(buildRealtimePresenceURL(nextDocumentId));
		if (!response.ok) {
			throw new Error(`Failed to fetch collaboration presence: ${response.status}`);
		}

		const payload = (await response.json()) as { connectedCount?: number };
		return typeof payload.connectedCount === 'number' ? payload.connectedCount : 0;
	}

	function clearPresenceSocket() {
		if (presenceHeartbeatTimer !== null) {
			window.clearInterval(presenceHeartbeatTimer);
			presenceHeartbeatTimer = null;
		}
	}

	async function unregisterPresenceSession(
		nextDocumentId: string,
		options: { keepalive?: boolean } = {}
	): Promise<void> {
		const token = auth.getAccessToken();
		if (!collaborationEnabled || !browser || !nextDocumentId || !presenceSessionId || !token) {
			return;
		}

		try {
			await fetch(buildRealtimePresenceURL(nextDocumentId), {
				method: 'DELETE',
				headers: {
					Authorization: `Bearer ${token}`,
					'X-Presence-Session-Id': presenceSessionId
				},
				credentials: 'include',
				keepalive: options.keepalive ?? false
			});
		} catch (error) {
			console.warn(`[Presence] Failed to unregister session for ${nextDocumentId}:`, error);
		}
	}

	async function connectPresenceSocket(nextDocumentId: string) {
		if (!collaborationEnabled || !browser || presenceHeartbeatTimer !== null) {
			return;
		}

		if (!auth.getAccessToken()) {
			return;
		}

		hasAttemptedPresence = true;
		const sessionId = ensurePresenceSessionId();
		const heartbeat = async () => {
			if (!auth.getAccessToken()) {
				return;
			}

			try {
				const response = await apiFetch(buildRealtimePresenceURL(nextDocumentId), {
					method: 'PUT',
					headers: {
						'Content-Type': 'application/json',
						'X-Presence-Session-Id': sessionId
					},
					body: JSON.stringify({ sessionId })
				});
				if (response.status === 429) {
					const payload = (await response.json()) as { maxSessions?: number };
					throw new Error(
						typeof payload.maxSessions === 'number'
							? m.editor_collaboration_session_limit_reached_with_count({
									count: String(payload.maxSessions)
								})
							: m.editor_collaboration_session_limit_reached()
					);
				}
				if (!response.ok) {
					throw new Error(`Presence heartbeat failed: ${response.status}`);
				}
				const payload = (await response.json()) as { connectedCount?: number };
				presenceCount = typeof payload.connectedCount === 'number' ? payload.connectedCount : 0;
				presenceConnected = true;
				collaborationError = isYjsConnected ? null : collaborationError;
				updateCollaborationIndicator();
			} catch (error) {
				presenceConnected = false;
				if (
					error instanceof Error &&
					(error.message.includes(m.editor_collaboration_session_limit_reached()) ||
						error.message.includes('online session limit'))
				) {
					collaborationError = error.message;
					toast.error(error.message);
				}
				console.warn(`[Presence] Heartbeat failed for ${nextDocumentId}:`, error);
				updateCollaborationIndicator();
			}
		};

		await heartbeat();
		presenceHeartbeatTimer = window.setInterval(() => {
			void heartbeat();
		}, 20000);
	}

	async function initializeCollaboration(nextDocumentId: string): Promise<ProviderInstance> {
		if (!collaborationEnabled) {
			throw new Error('collaboration-disabled');
		}

		const wsUrl = await resolveRealtimeWsUrl();
		const token = auth.getAccessToken();
		if (!token) {
			throw new Error('Missing access token');
		}

		const instance = await yjsProvider.createProvider({
			wsUrl,
			documentId: nextDocumentId,
			userId: authSignal.user?.id ?? 'unknown',
			token
		});

		if (instance.error) {
			console.warn('[Collaboration] Falling back to local Y.js mode:', instance.error);
		}

		return instance;
	}

	async function startCollaboration(nextDocumentId: string, reason: 'presence' | 'editing') {
		if (!collaborationEnabled) {
			return;
		}

		const now = Date.now();
		if (
			isInitializingCollaboration ||
			(collaboration && collaborationDocumentId === nextDocumentId && !collaborationError) ||
			(now - lastCollaborationAttemptAt < 10000 && reason !== 'presence')
		) {
			return;
		}

		if (collaboration && collaborationDocumentId !== nextDocumentId) {
			resetCollaborationForDocumentChange(nextDocumentId);
		} else if (collaborationError && collaboration) {
			yjsProvider.destroyProvider(nextDocumentId);
			collaboration = null;
			collaborationDocumentId = null;
			clearCollaborationListeners();
		}

		isInitializingCollaboration = true;
		lastCollaborationAttemptAt = now;
		try {
			const collaborationInstance = await initializeCollaboration(nextDocumentId);
			if (documentId !== nextDocumentId) {
				yjsProvider.destroyProvider(nextDocumentId);
				return;
			}

			if (collaborationInstance.error || !collaborationInstance.provider) {
				collaboration = null;
				collaborationError = collaborationInstance.error || m.editor_collaboration_connect_failed();
				isYjsConnected = false;
				updateCollaborationIndicator();
				yjsProvider.destroyProvider(nextDocumentId);
				return;
			}

			collaboration = collaborationInstance;
			collaborationDocumentId = nextDocumentId;
			collaborationError = null;
			attachCollaborationListeners(collaborationInstance);
		} catch (collaborationInitError) {
			console.error('[Collaboration] Failed to initialize realtime collaboration:', collaborationInitError);
			collaboration = null;
			collaborationDocumentId = null;
			collaborationError =
				collaborationInitError instanceof Error
					? collaborationInitError.message
					: m.editor_collaboration_unknown_error();
			isYjsConnected = false;
			updateCollaborationIndicator();
			clearCollaborationListeners();
		} finally {
			isInitializingCollaboration = false;
		}
	}

	function clearCollaborationListeners() {
		clearCollaborationContentSnapshotTimer();
		detachCollaborationListeners?.();
		detachCollaborationListeners = null;
	}

	function resetCollaborationForDocumentChange(nextDocumentId: string) {
		const previousDocumentId = collaborationDocumentId;

		clearCollaborationListeners();
		if (previousDocumentId && previousDocumentId !== nextDocumentId) {
			yjsProvider.destroyProvider(previousDocumentId);
		}

		collaboration = null;
		collaborationDocumentId = null;
		collaborationError = null;
		isYjsConnected = false;
	}

	function attachCollaborationListeners(instance: ProviderInstance) {
		clearCollaborationListeners();

		if (!instance.provider) {
			collaborationError = m.editor_collaboration_disconnected();
			isYjsConnected = false;
			updateCollaborationIndicator();
			return;
		}

		const syncPresence = (states?: Array<{ clientId: number }>) => {
			const awarenessStates = Array.from(instance.provider?.awareness?.getStates().values?.() ?? []);
			const peerCount = states?.length ?? awarenessStates.length;
			const onlineUserIds = awarenessStates
				.map((state) => {
					const userState = (state as { user?: { id?: string } }).user;
					return typeof userState?.id === 'string' ? userState.id : '';
				})
				.filter((value) => value !== '');

			presenceCount = Math.max(peerCount, 1);
			setOnlineMembersFromUserIds(
				onlineUserIds.length > 0
					? onlineUserIds
					: authSignal.user?.id
						? [authSignal.user.id]
						: []
			);
			updateCollaborationIndicator();
		};

		const handleStatus = ({ status }: { status: string }) => {
			if (status === 'connected') {
				collaborationError = null;
				isYjsConnected = true;
				syncPresence();
				return;
			}

			if (status === 'disconnected') {
				collaborationError = m.editor_collaboration_disconnected();
				isYjsConnected = false;
				isSaving = false;
				hasManualSaveRequestInFlight = false;
				settleCollaborationSaveWaiters(false);
				resetOnlineMembers();
				console.warn(`[Yjs] Disconnected for ${documentId}`);
				updateCollaborationIndicator();
			}
		};

		const handleAuthenticationFailed = ({ reason }: { reason: string }) => {
			collaborationError = reason || m.editor_collaboration_auth_failed();
			isYjsConnected = false;
			isSaving = false;
			hasManualSaveRequestInFlight = false;
			settleCollaborationSaveWaiters(false);
			resetOnlineMembers();
			console.warn(`[Yjs] Authentication failed for ${documentId}: ${collaborationError}`);
			updateCollaborationIndicator();
			if (documentId) {
				yjsProvider.stopReconnects(documentId);
			}
		};

		const handleAwarenessChange = ({ states }: { states: Array<{ clientId: number }> }) => {
			syncPresence(states);
		};

		const handleUnsyncedChanges = ({ number }: { number: number }) => {
			if (number > 0) {
				hasPendingCollaborationSave = localCollaborationChangeSeq > persistedCollaborationChangeSeq;
				isSaving = hasManualSaveRequestInFlight;
				logSaveDebug('realtime-unsynced', {
					unsyncedChanges: number
				});
				return;
			}

			if (localCollaborationChangeSeq > persistedCollaborationChangeSeq) {
				flushedCollaborationChangeSeq = localCollaborationChangeSeq;
				hasPendingCollaborationSave = true;
				isSaving = hasManualSaveRequestInFlight;
				if (hasPendingImmediateCollaborationPersist) {
					hasPendingImmediateCollaborationPersist = false;
					void (async () => {
						const requested = await requestImmediateCollaborationPersist();
						if (!requested) {
							hasManualSaveRequestInFlight = false;
							hasPendingCollaborationSave = localCollaborationChangeSeq > persistedCollaborationChangeSeq;
							isSaving = false;
							settleCollaborationSaveWaiters(false);
							toast.error(m.editor_save_failed());
							return;
						}
						logSaveDebug('manual-save-flush-complete-requested-immediate-persist', {
							flushedCollaborationChangeSeq,
							persistedCollaborationChangeSeq
						});
					})();
				}
				logSaveDebug('realtime-awaiting-persist-ack', {
					flushedCollaborationChangeSeq,
					persistedCollaborationChangeSeq
				});
				return;
			}

			hasPendingCollaborationSave = false;
			isSaving = false;
			logSaveDebug('realtime-clean');
		};

		const handleStateless = ({ payload }: { payload: string }) => {
			let message:
				| {
						type?: string;
						documentId?: string;
						savedAt?: string;
						startedAt?: string;
						acceptedAt?: string;
				  }
				| null = null;
			try {
				message = JSON.parse(payload) as {
					type?: string;
					documentId?: string;
					savedAt?: string;
					startedAt?: string;
					acceptedAt?: string;
				};
			} catch {
				return;
			}

			if (message?.documentId !== documentId) {
				return;
			}

			if (message.type === 'document-save-request-accepted') {
				if (hasManualSaveRequestInFlight) {
					isSaving = true;
					logSaveDebug('realtime-save-request-accepted', {
						acceptedAt: message.acceptedAt
					});
				}
				return;
			}

			if (message.type === 'document-persisting') {
				if (localCollaborationChangeSeq > persistedCollaborationChangeSeq) {
					isSaving = true;
					logSaveDebug('realtime-persisting', {
						flushedCollaborationChangeSeq,
						persistedCollaborationChangeSeq,
						startedAt: message.startedAt
					});
				}
				return;
			}

			if (message?.type !== 'document-persisted') {
				return;
			}

			if (flushedCollaborationChangeSeq <= persistedCollaborationChangeSeq) {
				return;
			}

			persistedCollaborationChangeSeq = flushedCollaborationChangeSeq;
			if (localCollaborationChangeSeq !== persistedCollaborationChangeSeq) {
				return;
			}

			if (instance.provider?.hasUnsyncedChanges) {
				return;
			}

			hasPendingCollaborationSave = false;
			hasUnsavedChanges = false;
			isSaving = false;
			hasManualSaveRequestInFlight = false;
			lastSaved =
				typeof message.savedAt === 'string' && message.savedAt.trim() !== ''
					? new Date(message.savedAt)
					: new Date();
			logSaveDebug('realtime-persisted', {
				flushedCollaborationChangeSeq,
				persistedCollaborationChangeSeq,
				savedAt: message.savedAt
			});
			settleCollaborationSaveWaiters(true);
		};

		instance.provider.on('status', handleStatus);
		instance.provider.on('authenticationFailed', handleAuthenticationFailed);
		instance.provider.on('awarenessChange', handleAwarenessChange);
		instance.provider.on('unsyncedChanges', handleUnsyncedChanges);
		instance.provider.on('stateless', handleStateless);
		detachCollaborationListeners = () => {
			instance.provider?.off('status', handleStatus);
			instance.provider?.off('authenticationFailed', handleAuthenticationFailed);
			instance.provider?.off('awarenessChange', handleAwarenessChange);
			instance.provider?.off('unsyncedChanges', handleUnsyncedChanges);
			instance.provider?.off('stateless', handleStateless);
		};

		if (instance.error) {
			collaborationError = instance.error;
			isYjsConnected = false;
			resetOnlineMembers();
			updateCollaborationIndicator();
		} else if (instance.isConnected) {
			isYjsConnected = true;
			syncPresence();
			handleUnsyncedChanges({
				number: instance.provider.hasUnsyncedChanges ? instance.provider.unsyncedChanges : 0
			});
		} else {
			isYjsConnected = false;
			resetOnlineMembers();
			updateCollaborationIndicator();
		}
	}

	async function saveLocalContent(reason: SaveReason = 'manual'): Promise<boolean> {
		if (!documentId || isLoading || isSaving || !hasUnsavedChanges) {
			return !hasUnsavedChanges;
		}

		isSaving = true;
		logSaveDebug('local-save-start', { reason });
		try {
			await updateDocumentContent(documentId, normalizeManagedImagesForSave(content));
			lastSaved = new Date();
			hasUnsavedChanges = false;
			logSaveDebug('local-save-success', { reason });
			return true;
		} catch (error) {
			console.error('[Save] Failed to save content:', error);
			logSaveDebug('local-save-failed', { reason, error: error instanceof Error ? error.message : String(error) });
			if (reason === 'manual') {
				toast.error(m.editor_save_failed());
			}
			return false;
		} finally {
			isSaving = false;
		}
	}

	function shouldUseHttpAutoSave(): boolean {
		// When realtime collaboration is actually connected, the Hocuspocus/Yjs
		// pipeline already debounces persistence on the collaboration server.
		// Keeping the legacy HTTP autosave interval enabled at the same time
		// causes redundant writes and visible save-state jitter in the editor UI.
		//
		// We only suppress the interval while collaboration is healthy. If
		// realtime is unavailable or disconnects, the page falls back to the
		// classic HTTP autosave path so single-user editing still has a safety net.
		return saveDriver === 'local';
	}

	async function requestCollaborationSave(_reason: SaveReason): Promise<boolean> {
		const provider = collaboration?.provider;
		if (!provider || !isYjsConnected) {
			logSaveDebug('collaboration-save-skipped-disconnected');
			return false;
		}

		if (!hasUnsavedChanges && !hasPendingCollaborationSave && !provider.hasUnsyncedChanges) {
			logSaveDebug('collaboration-save-noop');
			return true;
		}

		isSaving = false;
		if (_reason === 'manual') {
			logSaveDebug('manual-save-clicked');
			hasManualSaveRequestInFlight = true;
			isSaving = true;
			hasPendingImmediateCollaborationPersist = true;
		}
		logSaveDebug('collaboration-save-requested', {
			reason: _reason,
			unsyncedChanges: provider.unsyncedChanges,
			hasProviderUnsyncedChanges: provider.hasUnsyncedChanges
		});
		sendCollaborationContentSnapshot(content);
		if (_reason === 'manual') {
			// If Yjs already flushed this revision to the collaboration doc, skip
			// the extra forceSync round-trip and persist immediately.
			if (!provider.hasUnsyncedChanges) {
				flushedCollaborationChangeSeq = localCollaborationChangeSeq;
				hasPendingCollaborationSave = localCollaborationChangeSeq > persistedCollaborationChangeSeq;
				hasPendingImmediateCollaborationPersist = false;
				if (await requestImmediateCollaborationPersist()) {
					logSaveDebug('manual-save-requested-immediate-persist', {
						flushedCollaborationChangeSeq,
						persistedCollaborationChangeSeq
					});
					return queueCollaborationSaveWaiter();
				}
				hasPendingImmediateCollaborationPersist = false;
				hasManualSaveRequestInFlight = false;
				isSaving = false;
				toast.error(m.editor_save_failed());
				return false;
			}
			provider.forceSync();
			return queueCollaborationSaveWaiter();
		}

		if (hasUnsavedChanges || provider.hasUnsyncedChanges) {
			provider.forceSync();
		}

		return queueCollaborationSaveWaiter();
	}

	async function requestDocumentSave(reason: SaveReason = 'manual'): Promise<boolean> {
		if (saveDriver === 'collaboration') {
			return requestCollaborationSave(reason);
		}
		if (saveDriver === 'local') {
			return saveLocalContent(reason);
		}
		return false;
	}

	async function handleSaveAndLeave() {
		const saved = await requestDocumentSave('leave');
		if (!saved) {
			return;
		}
		await handleConfirmLeave();
	}

	async function handleImageTargetChange(nextTargetId: string) {
		if (!documentId || isUpdatingImageTarget || nextTargetId === preferredImageTargetId) {
			return;
		}

		isUpdatingImageTarget = true;
		try {
			const updated = await updateDocumentImageTarget(documentId, nextTargetId);
			preferredImageTargetId = updated.preferredImageTargetId;
			toast.success(m.editor_image_target_updated());
		} catch (error) {
			console.error('[Document] Failed to update image target:', error);
			toast.error(
				error instanceof Error && error.message.trim() !== ''
					? error.message
					: m.editor_image_target_update_failed()
			);
		} finally {
			isUpdatingImageTarget = false;
		}
	}

	// Load document content when ID becomes available
	$effect(() => {
		if (documentId && !authSignal.loading && !realtimeConfigSignal.loading) {
			const targetDocumentId = documentId;
			const loadSequence = ++documentLoadSequence;
			isLoading = true;
			settleCollaborationSaveWaiters(false);
			resetOnlineMembers();
			clearPresenceSocket();
			resetCollaborationForDocumentChange(targetDocumentId);

			const isCurrentLoad = () => loadSequence === documentLoadSequence && documentId === targetDocumentId;

			const loadContent = async () => {
				try {
					console.log('[Load] Loading document for ID:', targetDocumentId);
					// Load document details (for title) and content in parallel
					const [details, data, configs, memberResponse] = await Promise.all([
						getDocumentDetails(targetDocumentId),
						getDocumentContent(targetDocumentId),
						getImageBedConfigs().catch((error) => {
							console.error('[Load] Failed to load image bed configs:', error);
							return [] as ImageBedConfig[];
						}),
						listDocumentMembers(targetDocumentId).catch(() => ({
							documentId: targetDocumentId,
							members: [] as ShareDocumentMember[]
						}))
					]);

					if (!isCurrentLoad()) {
						return;
					}

					if (details.myRole === 'viewer') {
						await goto(`/view/documents/${targetDocumentId}`);
						return;
					}

					if (!collaborationEnabled && details.myRole !== 'owner') {
						await goto(`/view/documents/${targetDocumentId}`);
						return;
					}

					const loadedContent = data.contentJson ?? EMPTY_DOC;
					const hydratedContent = await refreshSignedImageSources(loadedContent);
					if (!isCurrentLoad()) {
						return;
					}

					imageBedConfigs = configs;
					content = hydratedContent;
					// Use the title from the API
					title = details.title ?? '';
					manualExcerpt = details.manualExcerpt ?? '';
					folderId = details.folderId ?? null;
					myRole = details.myRole ?? 'owner';
					documentMembers = memberResponse.members;
					publicAccess = details.publicAccess ?? 'private';
					publicUrl = details.publicUrl ?? `/view/documents/${targetDocumentId}`;
					documentType = details.documentType ?? 'rich_text';
					preferredImageTargetId = details.preferredImageTargetId ?? 'managed-r2';
					hasUnsavedChanges = false;
					lastSaved = null;
					presenceCount = 0;
					presenceConnected = false;
					hasAttemptedPresence = false;
					collaborationError = null;
					isYjsConnected = false;
					hasPendingCollaborationSave = false;
					localCollaborationChangeSeq = 0;
					flushedCollaborationChangeSeq = 0;
					persistedCollaborationChangeSeq = 0;
					hasManualSaveRequestInFlight = false;
					isSaving = false;
					updateCollaborationIndicator();
					console.log('[Load] Title loaded:', title);
					isLoading = false;

					if (collaborationEnabled) {
						void (async () => {
							try {
								presenceCount = await fetchCollaborationPresence(targetDocumentId);
								if (!isCurrentLoad()) {
									return;
								}
								updateCollaborationIndicator();
								await connectPresenceSocket(targetDocumentId);
								if (!isCurrentLoad()) {
									return;
								}
								// Collaboration bootstrap stays in the background on purpose:
								// local content must render first so a down realtime service
								// degrades to single-user editing instead of blanking the editor.
								await startCollaboration(targetDocumentId, 'presence');
							} catch (presenceError) {
								if (!isCurrentLoad()) {
									return;
								}
								console.error('[Collaboration] Failed to fetch presence:', presenceError);
								presenceCount = 0;
								presenceConnected = false;
								hasAttemptedPresence = true;
								collaborationError = 'presence-disconnected';
								isYjsConnected = false;
								updateCollaborationIndicator();
							}
						})();
					}
				} catch (error) {
					if (!isCurrentLoad()) {
						return;
					}
					console.error('[Load] Failed to load document:', error);
					collaboration = null;
					collaborationDocumentId = null;
					collaborationIndicator = null;
					isYjsConnected = false;
					clearPresenceSocket();
					clearCollaborationListeners();
					toast.error(
						error instanceof Error && error.message.trim() !== ''
							? error.message
							: '加载文档失败'
					);
					goto('/workspace');
				} finally {
					if (isCurrentLoad() && isLoading) {
						isLoading = false;
					}
				}
			};
			loadContent();
		}
	});

	onDestroy(() => {
		unsubscribePage();
		unsubscribeAuth();
		unsubscribeRealtimeConfig();
		if (documentId) {
			void unregisterPresenceSession(documentId, { keepalive: true });
		}
		clearCollaborationContentSnapshotTimer();
		settleEditorContentOverrideWaiter(false);
		settleCollaborationSaveWaiters(false);
		clearPresenceSocket();
		clearCollaborationListeners();
		if (collaborationDocumentId) {
			yjsProvider.destroyProvider(collaborationDocumentId);
		}
		if (documentId && documentId !== collaborationDocumentId) {
			yjsProvider.destroyProvider(documentId);
		}
	});

	$effect(() => {
		if (!browser) {
			return;
		}

		// 当前先从本地偏好读取自动保存策略，后续可以直接换成个人中心设置源。
		autoSaveEnabled = readAutoSaveEnabled();
		autoSaveIntervalSeconds = readAutoSaveIntervalSeconds();
	});

	$effect(() => {
		if (!browser || !documentId || isLoading || !autoSaveEnabled || !shouldUseHttpAutoSave()) {
			return;
		}

		// 自动保存只负责非 realtime 模式下的兜底落盘，不额外维护独立状态指示。
			const timer = window.setInterval(() => {
				if (!hasUnsavedChanges || isSaving) {
					return;
				}

				void requestDocumentSave('auto');
			}, autoSaveIntervalSeconds * 1000);

		return () => {
			window.clearInterval(timer);
		};
	});


	onMount(() => {
		const handleKeydown = (event: KeyboardEvent) => {
			const isSaveKey = (event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's';
			if (!isSaveKey) return;
			event.preventDefault();
			void requestDocumentSave('manual');
		};

		const handleBeforeUnload = (event: BeforeUnloadEvent) => {
			if (documentId) {
				void unregisterPresenceSession(documentId, { keepalive: true });
			}

			if (!hasUnsavedChanges) {
				return;
			}

			event.preventDefault();
			event.returnValue = '';
		};

		window.addEventListener('keydown', handleKeydown);
		window.addEventListener('beforeunload', handleBeforeUnload);
		return () => {
			window.removeEventListener('keydown', handleKeydown);
			window.removeEventListener('beforeunload', handleBeforeUnload);
		};
	});
</script>

<svelte:head>
  <title>{m.page_title_edit_document({ title })}</title>
</svelte:head>

<div class="flex h-screen flex-col bg-white dark:bg-zinc-900">
	{#if documentId}
			<EditorTopBar
				{documentId}
				initialTitle={title}
				initialExcerpt={manualExcerpt}
				{documentType}
				{preferredImageTargetId}
				{availableImageTargets}
				{myRole}
				{publicAccess}
				{publicUrl}
				{collaborationEnabled}
				{collaborationIndicator}
				{onlineMembers}
				readOnly={false}
				showEditShortcut={false}
				{isUpdatingImageTarget}
				{isSaving}
				{lastSaved}
				{hasUnsavedChanges}
				onTitleChange={handleTitleChange}
				onManualExcerptChange={handleExcerptChange}
				onImageTargetChange={handleImageTargetChange}
				onPublicAccessChange={(nextPublicAccess, nextPublicURL) =>
					handlePublicAccessChange(nextPublicAccess, nextPublicURL)}
			/>
	{/if}

	<!-- Editor -->
	<main class="flex-1 overflow-hidden">
		<div class="h-full w-full">
			{#if browser && !isLoading}
				{#if documentType === 'table'}
					<div class="prose dark:prose-invert p-6">
						<p>{m.edit_document_editor_under_construction()}</p>
					</div>
				{:else}
					<Editor
						documentId={documentId!}
						{content}
						externalContentOverride={editorContentOverride}
						currentImageTargetId={preferredImageTargetId}
						currentImageTargetLabel={currentImageTargetLabel}
						imageTargetOptions={availableImageTargets}
						collaboration={activeCollaboration}
						{isUpdatingImageTarget}
						{isSaving}
						{hasUnsavedChanges}
						onImageTargetChange={handleImageTargetChange}
						hydrateManagedContent={refreshSignedImageSources}
						onSave={() => requestDocumentSave('manual')}
						onExportAction={handleExportAction}
						onContentChange={handleContentChange}
					/>
				{/if}
			{:else}
				<div class="prose dark:prose-invert">
					<p>{m.workspace_loading()}</p>
				</div>
			{/if}
		</div>
	</main>
</div>

<ConfirmDialog
	open={isLeaveConfirmOpen}
	title={m.common_unsaved_changes()}
	message={m.editor_unsaved_confirm_leave()}
	confirmText={m.common_save()}
	secondaryText={m.common_dont_save()}
	confirmVariant="primary"
	onCancel={handleCancelLeave}
	onSecondary={handleLeaveWithoutSave}
	onConfirm={handleSaveAndLeave}
/>

<ExportPrivateImagesDialog
	open={isExportPrivateImagesDialogOpen}
	imageCount={collectManagedImages(content).length}
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
