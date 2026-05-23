import type { JSONContent } from '@tiptap/core';

type ImageNodeRecord = Record<string, unknown> & {
	attrs?: Record<string, unknown>;
	content?: unknown[];
};

export type ManagedImageUsage = {
	assetId: string;
	src: string | null;
	title: string | null;
	alt: string | null;
};

export function cloneContentJson(value: JSONContent): JSONContent {
	return JSON.parse(JSON.stringify(value)) as JSONContent;
}

export function collectImageNodes(value: unknown, nodes: ImageNodeRecord[]) {
	if (!value || typeof value !== 'object') {
		return;
	}

	const node = value as ImageNodeRecord;
	if (node.type === 'image') {
		nodes.push(node);
	}

	const children = node.content;
	if (Array.isArray(children)) {
		for (const child of children) {
			collectImageNodes(child, nodes);
		}
	}
}

export function getManagedAssetId(attrs: Record<string, unknown>): string | null {
	const raw = attrs.assetId;
	return typeof raw === 'string' && raw.trim() !== '' ? raw.trim() : null;
}

export function collectManagedImages(content: JSONContent): ManagedImageUsage[] {
	const imageNodes: ImageNodeRecord[] = [];
	collectImageNodes(content, imageNodes);

	const usages: ManagedImageUsage[] = [];
	const seen = new Set<string>();
	for (const node of imageNodes) {
		const attrs = (node.attrs ?? {}) as Record<string, unknown>;
		const assetId = getManagedAssetId(attrs);
		if (!assetId || seen.has(assetId)) {
			continue;
		}

		seen.add(assetId);
		usages.push({
			assetId,
			src: typeof attrs.src === 'string' && attrs.src.trim() !== '' ? attrs.src.trim() : null,
			title: typeof attrs.title === 'string' && attrs.title.trim() !== '' ? attrs.title.trim() : null,
			alt: typeof attrs.alt === 'string' && attrs.alt.trim() !== '' ? attrs.alt.trim() : null
		});
	}

	return usages;
}

export function replaceManagedImagesWithPublicURLs(
	content: JSONContent,
	publicURLByAssetID: Map<string, string>
): JSONContent {
	const cloned = cloneContentJson(content);
	const imageNodes: ImageNodeRecord[] = [];
	collectImageNodes(cloned, imageNodes);

	for (const node of imageNodes) {
		const attrs = (node.attrs ?? {}) as Record<string, unknown>;
		const assetId = getManagedAssetId(attrs);
		if (!assetId) {
			continue;
		}

		const publicURL = publicURLByAssetID.get(assetId);
		if (!publicURL) {
			continue;
		}

		attrs.src = publicURL;
		delete attrs.assetId;
		node.attrs = attrs;
	}

	return cloned;
}

export function normalizeManagedImagesForSave(input: JSONContent): JSONContent {
	const cloned = cloneContentJson(input);
	const imageNodes: ImageNodeRecord[] = [];
	collectImageNodes(cloned, imageNodes);

	for (const node of imageNodes) {
		const attrs = (node.attrs ?? {}) as Record<string, unknown>;
		const assetId = getManagedAssetId(attrs);
		if (!assetId) continue;
		delete attrs.src;
		node.attrs = attrs;
	}

	return cloned;
}

export function buildExportAssetFilename(
	assetId: string,
	mimeType: string,
	fallbackLabel?: string | null
): string {
	const rawLabel = (fallbackLabel ?? '').trim() || `asset-${assetId}`;
	const safeLabel = rawLabel.replace(/[\\/:*?"<>|]+/g, ' ').replace(/\s+/g, ' ').trim();
	const extension = resolveExportAssetExtension(mimeType, safeLabel);
	const basename = stripExportAssetExtension(safeLabel || `asset-${assetId}`);
	return `${basename || `asset-${assetId}`}.${extension}`;
}

export function inferExportAssetMimeType(
	mimeType: string,
	fallbackLabel?: string | null,
	src?: string | null
): string {
	const normalized = normalizeMimeType(mimeType);
	if (isSupportedExportImageMimeType(normalized)) {
		return normalized;
	}

	const extension = getImageExtensionFromLabel(fallbackLabel) ?? getImageExtensionFromURL(src);
	switch (extension) {
		case 'png':
			return 'image/png';
		case 'jpg':
		case 'jpeg':
			return 'image/jpeg';
		case 'webp':
			return 'image/webp';
		case 'gif':
			return 'image/gif';
		default:
			return normalized || 'application/octet-stream';
	}
}

function resolveExportAssetExtension(mimeType: string, fallbackLabel?: string | null): string {
	const normalized = normalizeMimeType(mimeType);
	switch (normalized) {
		case 'image/png':
			return 'png';
		case 'image/jpeg':
			return 'jpg';
		case 'image/webp':
			return 'webp';
		case 'image/gif':
			return 'gif';
		default:
			return getImageExtensionFromLabel(fallbackLabel) ?? 'bin';
	}
}

function stripExportAssetExtension(value: string): string {
	return value.replace(/\.(?:png|jpe?g|webp|gif|bin)$/i, '').trim();
}

function normalizeMimeType(value: string): string {
	return value.split(';', 1)[0]?.trim().toLowerCase() ?? '';
}

function isSupportedExportImageMimeType(value: string): boolean {
	return value === 'image/png' || value === 'image/jpeg' || value === 'image/webp' || value === 'image/gif';
}

function getImageExtensionFromLabel(value?: string | null): string | null {
	const match = value?.trim().toLowerCase().match(/\.([a-z0-9]+)$/);
	const extension = match?.[1] ?? '';
	return extension === 'png' || extension === 'jpg' || extension === 'jpeg' || extension === 'webp' || extension === 'gif'
		? extension
		: null;
}

function getImageExtensionFromURL(value?: string | null): string | null {
	if (!value) {
		return null;
	}
	try {
		const parsed = new URL(value);
		return getImageExtensionFromLabel(parsed.pathname);
	} catch {
		return getImageExtensionFromLabel(value);
	}
}
