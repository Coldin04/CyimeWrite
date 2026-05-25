import type { JSONContent } from '@tiptap/core';
import { apiFetch } from '$lib/api';
import {
	copyToClipboard,
	downloadTextFile,
	exportBBCode,
	exportHtmlDocument,
	exportMarkdown
} from '$lib/export/documentExport';
import type { ExportAction } from '$lib/export/exportActions';
import {
	cloneContentJson,
	collectManagedImages,
	replaceManagedImagesWithPublicURLs
} from '$lib/export/managedImages';

export class ExportCopyError extends Error {
	content: string;

	constructor(message: string, content: string) {
		super(message);
		this.name = 'ExportCopyError';
		this.content = content;
	}
}

export function buildExportFilename(title: string, extension: 'html' | 'md'): string {
	const rawTitle = title.trim();
	const safeTitle = rawTitle.replace(/[\\/:*?"<>|]+/g, ' ').replace(/\s+/g, ' ').trim();
	return `${safeTitle || 'cyimewrite-export'}.${extension}`;
}

export async function runExportAction(action: ExportAction, options: {
	title: string;
	contentJson: JSONContent;
}): Promise<'copied' | 'downloaded'> {
	if (action === 'download-pdf') {
		throw new Error('download_pdf_requires_print_html');
	}

	if (action === 'copy-bbcode') {
		const bbcode = exportBBCode(options.contentJson);
		const copied = await copyToClipboard(bbcode);
		if (!copied) {
			throw new ExportCopyError('copy_bbcode_failed', bbcode);
		}
		return 'copied';
	}

	if (action === 'download-html') {
		const html = await exportHtmlDocument({
			title: options.title.trim() || 'Cyime Export',
			contentJson: options.contentJson
		});
		downloadTextFile(buildExportFilename(options.title, 'html'), html, 'text/html;charset=utf-8');
		return 'downloaded';
	}

	const markdown = exportMarkdown(options.contentJson);
	if (action === 'copy-markdown') {
		const copied = await copyToClipboard(markdown);
		if (!copied) {
			throw new ExportCopyError('copy_markdown_failed', markdown);
		}
		return 'copied';
	}

	downloadTextFile(buildExportFilename(options.title, 'md'), markdown, 'text/markdown;charset=utf-8');
	return 'downloaded';
}

export async function inlineManagedImagesAsDataURLs(
	content: JSONContent,
	resolveAssetContentURL: (assetId: string) => string
): Promise<JSONContent> {
	const managedImages = collectManagedImages(content);
	if (managedImages.length === 0) {
		return cloneContentJson(content);
	}

	const dataURLByAssetID = new Map<string, string>();
	for (const item of managedImages) {
		const response = await apiFetch(resolveAssetContentURL(item.assetId));
		if (!response.ok) {
			throw new Error(`Failed to fetch private image ${item.assetId}`);
		}

		const blob = await response.blob();
		const dataURL = await new Promise<string>((resolve, reject) => {
			const reader = new FileReader();
			reader.onload = () => {
				if (typeof reader.result === 'string') {
					resolve(reader.result);
					return;
				}
				reject(new Error('Failed to encode image as data URL'));
			};
			reader.onerror = () => reject(reader.error ?? new Error('Failed to read image blob'));
			reader.readAsDataURL(blob);
		});
		dataURLByAssetID.set(item.assetId, dataURL);
	}

	return replaceManagedImagesWithPublicURLs(content, dataURLByAssetID);
}

export async function exportPdfDocument(options: {
	title: string;
	html: string;
}): Promise<void> {
	const iframe = document.createElement('iframe');
	iframe.style.position = 'fixed';
	iframe.style.right = '0';
	iframe.style.bottom = '0';
	iframe.style.width = '0';
	iframe.style.height = '0';
	iframe.style.border = '0';
	iframe.setAttribute('aria-hidden', 'true');
	document.body.appendChild(iframe);

	const cleanup = () => {
		window.setTimeout(() => {
			iframe.remove();
		}, 1000);
	};

	// Temporary browser-side PDF export: render a print-friendly HTML snapshot
	// and rely on the browser's native Print to PDF flow. This avoids adding
	// Chromium or a heavy server-side PDF stack for now.
	iframe.onload = () => {
		iframe.contentWindow?.focus();
		iframe.contentWindow?.print();
		cleanup();
	};
	iframe.srcdoc = options.html;
}

export { exportHtmlDocument };
