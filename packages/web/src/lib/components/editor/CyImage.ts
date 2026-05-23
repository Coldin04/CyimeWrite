import type {
	JSONContent,
	MarkdownParseHelpers,
	MarkdownRendererHelpers,
	MarkdownToken,
	RenderContext
} from '@tiptap/core';
import Image from '@tiptap/extension-image';

export const cyImageWidths = ['auto', '40%', '60%', '80%', '100%'] as const;
export const cyImageAlignments = ['content', 'full'] as const;
export const cyImageAssetMarkdownScheme = 'cyime-asset:';

function trimmedString(value: unknown): string {
	return typeof value === 'string' ? value.trim() : '';
}

export function buildCyImageAssetMarkdownURL(assetId: string): string {
	return `${cyImageAssetMarkdownScheme}${encodeURIComponent(assetId.trim())}`;
}

export function parseCyImageAssetMarkdownURL(value: unknown): string | null {
	const raw = trimmedString(value);
	if (!raw.toLowerCase().startsWith(cyImageAssetMarkdownScheme)) {
		return null;
	}

	const encoded = raw.slice(cyImageAssetMarkdownScheme.length).trim();
	if (!encoded) {
		return null;
	}

	try {
		return decodeURIComponent(encoded).trim() || null;
	} catch {
		return encoded;
	}
}

function escapeMarkdownImageText(value: string): string {
	return value.replaceAll('\\', '\\\\').replaceAll('[', '\\[').replaceAll(']', '\\]');
}

function escapeMarkdownImageTitle(value: string): string {
	return value.replaceAll('\\', '\\\\').replaceAll('"', '\\"');
}

function parseImageMarkdown(token: MarkdownToken, helpers: MarkdownParseHelpers): JSONContent {
	const assetId = parseCyImageAssetMarkdownURL(token.href);
	const title = trimmedString(token.title) || null;
	const alt = trimmedString(token.text) || null;

	if (assetId) {
		return helpers.createNode('image', {
			src: null,
			assetId,
			title,
			alt
		});
	}

	return helpers.createNode('image', {
		src: token.href,
		title,
		alt
	});
}

function renderImageMarkdown(
	node: JSONContent,
	_helpers: MarkdownRendererHelpers,
	_context: RenderContext
): string {
	const attrs = node.attrs ?? {};
	const assetId = trimmedString(attrs.assetId);
	const src = assetId ? buildCyImageAssetMarkdownURL(assetId) : trimmedString(attrs.src);
	const title = trimmedString(attrs.title);
	const alt = escapeMarkdownImageText(trimmedString(attrs.alt));

	return title ? `![${alt}](${src} "${escapeMarkdownImageTitle(title)}")` : `![${alt}](${src})`;
}

export const CyImage = Image.extend({
	addAttributes() {
		return {
			...this.parent?.(),
			assetId: {
				default: null,
				parseHTML: (element) => element.getAttribute('data-asset-id'),
				renderHTML: (attributes) => {
					const assetId =
						typeof attributes.assetId === 'string' && attributes.assetId.trim() !== ''
							? attributes.assetId.trim()
							: null;

					if (!assetId) {
						return {};
					}

					return {
						'data-asset-id': assetId
					};
				}
			},
			alt: {
				default: null,
				parseHTML: (element) => element.getAttribute('alt'),
				renderHTML: (attributes) => {
					const alt = typeof attributes.alt === 'string' ? attributes.alt.trim() : '';
					if (alt !== '') {
						return { alt };
					}

					const title = typeof attributes.title === 'string' ? attributes.title.trim() : '';
					if (title !== '') {
						return { alt: title };
					}

					return {};
				}
			},
			width: {
				default: null,
				parseHTML: (element) =>
					element.getAttribute('data-display-width') ||
					element.getAttribute('width') ||
					(element instanceof HTMLElement ? element.style.width || null : null),
				renderHTML: (attributes) => {
					const width =
						typeof attributes.width === 'string' && attributes.width.trim() !== ''
							? attributes.width.trim()
							: null;

					if (!width || width === 'auto') {
						return {};
					}

					return {
						'data-display-width': width,
						style: `width: ${width};`
					};
				}
			},
			align: {
				default: 'content',
				parseHTML: (element) => element.getAttribute('data-display-align') || 'content',
				renderHTML: (attributes) => {
					const align =
						typeof attributes.align === 'string' && attributes.align.trim() !== ''
							? attributes.align.trim()
							: 'content';

					if (align === 'content') {
						return {};
					}

					return {
						'data-display-align': align
					};
				}
			}
		};
	},
	parseMarkdown: parseImageMarkdown,
	renderMarkdown: renderImageMarkdown
});
