import type { JSONContent } from '@tiptap/core';
import { apiFetch } from '$lib/api';
import { pasteDocumentImage } from '$lib/api/editor';
import { resolveApiUrl } from '$lib/config/api';
import type { ExportAction } from '$lib/export/exportActions';
import {
	buildExportAssetFilename,
	cloneContentJson,
	collectManagedImages,
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
import { toast } from 'svelte-sonner';
import * as m from '$paraglide/messages';

export { normalizeManagedImagesForSave };

export function createExportCopyTitle(value: string): string {
	const trimmed = value.trim();
	const exportLabel = `(${m.common_export()})`;
	return trimmed === '' ? `${m.workspace_notification_untitled_document()} ${exportLabel}` : `${trimmed} ${exportLabel}`;
}

export async function performDocumentExport(options: {
	action: ExportAction;
	title: string;
	contentJson: JSONContent;
	onManualCopy: (title: string, content: string) => void;
}) {
	const { action, title, contentJson, onManualCopy } = options;

	try {
		if (action === 'download-pdf') {
			const printContent = await inlineManagedImagesAsDataURLs(contentJson, (assetId) =>
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
			contentJson
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
			onManualCopy(m.editor_export_copy_markdown(), error.content);
			toast.error(m.editor_export_markdown_copy_failed());
			return;
		}
		if (error instanceof ExportCopyError && error.message === 'copy_bbcode_failed') {
			onManualCopy(m.editor_export_copy_bbcode(), error.content);
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

export async function prepareExportContentWithPublicImages(options: {
	documentId: string;
	contentJson: JSONContent;
	targetId: string;
	toastId: string;
}): Promise<JSONContent> {
	const contentSnapshot = cloneContentJson(options.contentJson);
	const managedImages = collectManagedImages(contentSnapshot);
	if (managedImages.length === 0) {
		return contentSnapshot;
	}

	toast.loading(m.editor_export_prepare_images({ current: 0, total: managedImages.length }), {
		id: options.toastId,
		duration: Infinity
	});

	try {
		const publicURLByAssetID = new Map<string, string>();

		for (let index = 0; index < managedImages.length; index += 1) {
			const image = managedImages[index];
			toast.loading(m.editor_export_prepare_images({ current: index + 1, total: managedImages.length }), {
				id: options.toastId,
				duration: Infinity
			});

			const response = await apiFetch(`/api/v1/media/assets/${image.assetId}/content`);
			if (!response.ok) {
				throw new Error(`Failed to fetch private image ${image.assetId}`);
			}

			const blob = await response.blob();
			const mimeType = inferExportAssetMimeType(
				blob.type || response.headers.get('content-type') || '',
				image.title ?? image.alt,
				image.src
			);
			const file = new File(
				[blob],
				buildExportAssetFilename(image.assetId, mimeType, image.title ?? image.alt),
				{ type: mimeType }
			);
			const uploaded = await pasteDocumentImage(options.documentId, file, { targetId: options.targetId });
			publicURLByAssetID.set(image.assetId, uploaded.url);
		}

		return replaceManagedImagesWithPublicURLs(contentSnapshot, publicURLByAssetID);
	} finally {
		toast.dismiss(options.toastId);
	}
}

export function resolveExportErrorMessage(error: unknown): string {
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
