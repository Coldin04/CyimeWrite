import type { JSONContent } from '@tiptap/core';
import { generateHTML } from '@tiptap/core';
import Link from '@tiptap/extension-link';
import Mathematics from '@tiptap/extension-mathematics';
import StarterKit from '@tiptap/starter-kit';
import { Table } from '@tiptap/extension-table';
import { TableCell } from '@tiptap/extension-table-cell';
import { TableHeader } from '@tiptap/extension-table-header';
import { TableRow } from '@tiptap/extension-table-row';
import katex from 'katex';
import { CyImage } from '$lib/components/editor/CyImage';

type ExportNode = {
	type?: string;
	text?: string;
	attrs?: Record<string, unknown>;
	content?: ExportNode[];
	marks?: Array<{ type?: string; attrs?: Record<string, unknown> }>;
};

function exportExtensions() {
	return [
		StarterKit.configure({
			link: false
		}),
		CyImage.configure({
			inline: false,
			allowBase64: true
		}),
		Link.configure({
			openOnClick: false,
			autolink: true,
			defaultProtocol: 'https'
		}),
		Mathematics.configure({
			katexOptions: {
				throwOnError: false,
				strict: 'ignore'
			}
		}),
		Table.configure({
			resizable: false
		}),
		TableRow,
		TableHeader,
		TableCell
	];
}

function escapeHtml(value: string): string {
	return value
		.replaceAll('&', '&amp;')
		.replaceAll('<', '&lt;')
		.replaceAll('>', '&gt;')
		.replaceAll('"', '&quot;')
		.replaceAll("'", '&#39;');
}

function escapeMarkdown(value: string): string {
	return value.replaceAll('[', '\\[').replaceAll(']', '\\]');
}

function normalizeCodeBlockLanguageInfo(value: unknown): string {
	if (typeof value !== 'string') {
		return '';
	}
	return value
		.trim()
		.replace(/[\s`]+/g, '')
		.slice(0, 32);
}

function getText(node: ExportNode | undefined): string {
	if (!node) return '';
	if (node.type === 'text') return node.text ?? '';
	return (node.content ?? []).map((child) => getText(child)).join('');
}

function renderKatexInHtml(html: string): string {
	if (typeof DOMParser === 'undefined') return html;

	const doc = new DOMParser().parseFromString(html, 'text/html');

	for (const node of Array.from(doc.querySelectorAll('span[data-type="inline-math"][data-latex]'))) {
		const latex = node.getAttribute('data-latex') ?? '';
		try {
			node.innerHTML = katex.renderToString(latex, {
				throwOnError: false,
				strict: 'ignore'
			});
		} catch {
			node.textContent = latex;
		}
	}

	for (const node of Array.from(doc.querySelectorAll('div[data-type="block-math"][data-latex]'))) {
		const latex = node.getAttribute('data-latex') ?? '';
		try {
			node.innerHTML = katex.renderToString(latex, {
				throwOnError: false,
				strict: 'ignore',
				displayMode: true
			});
		} catch {
			node.textContent = latex;
		}
	}

	return doc.body.innerHTML;
}

export function exportHtmlDocument(options: {
	title: string;
	contentJson: JSONContent;
	includeKatexCssLink?: boolean;
}): string {
	const { title, contentJson, includeKatexCssLink = true } = options;
	const renderedBody = renderKatexInHtml(generateHTML(contentJson, exportExtensions()));
	const safeTitle = title.trim() || 'Cyime Export';
	const katexLink = includeKatexCssLink
		? '<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/katex@0.16.44/dist/katex.min.css" />'
		: '';

	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>${escapeHtml(safeTitle)}</title>
  ${katexLink}
  <style>
    :root { color-scheme: light; }
    body { margin: 0; padding: 48px 20px; font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color: #18181b; background: #ffffff; }
    main { max-width: 920px; margin: 0 auto; }
    img { max-width: 100%; height: auto; }
    pre { overflow-x: auto; padding: 16px; border-radius: 12px; background: #18181b; color: #fafafa; }
    code { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
    blockquote { margin: 20px 0; padding: 12px 16px; border-left: 3px solid #14b8a6; background: #f4f4f5; }
    table { width: 100%; border-collapse: collapse; }
    th, td { border: 1px solid #a1a1aa; padding: 8px 10px; }
  </style>
</head>
<body>
  <main>${renderedBody}</main>
</body>
</html>`;
}

export function exportMarkdown(contentJson: JSONContent): string {
	const lines: string[] = [];

	function renderInline(nodes: ExportNode[] = []): string {
		let output = '';

		for (const node of nodes) {
			switch (node.type) {
				case 'text': {
					let text = node.text ?? '';
					const marks = node.marks ?? [];
					const isCode = marks.some((mark) => mark.type === 'code');
					const isBold = marks.some((mark) => mark.type === 'bold');
					const isItalic = marks.some((mark) => mark.type === 'italic');
					const linkMark = marks.find((mark) => mark.type === 'link');

					if (isCode) {
						text = `\`${text.replaceAll('`', '\\`')}\``;
					} else {
						if (isBold) text = `**${text}**`;
						if (isItalic) text = `*${text}*`;
					}

					if (linkMark?.attrs?.href && typeof linkMark.attrs.href === 'string') {
						text = `[${text}](${linkMark.attrs.href})`;
					}

					output += text;
					break;
				}
				case 'hardBreak':
					output += '  \n';
					break;
				case 'inlineMath':
					output += `$${typeof node.attrs?.latex === 'string' ? node.attrs.latex : ''}$`;
					break;
				case 'image': {
					const src = typeof node.attrs?.src === 'string' ? node.attrs.src : '';
					const title = typeof node.attrs?.title === 'string' ? node.attrs.title : '';
					const alt = typeof node.attrs?.alt === 'string' ? node.attrs.alt : title;
					output += `![${escapeMarkdown(alt)}](${src})`;
					break;
				}
				default:
					output += renderInline(node.content ?? []);
			}
		}

		return output;
	}

	function renderNode(node: ExportNode | undefined, depth = 0, orderedIndex = 1) {
		if (!node) return;

		switch (node.type) {
			case 'doc':
				for (const child of node.content ?? []) renderNode(child, depth);
				return;
			case 'paragraph':
				lines.push(renderInline(node.content ?? []));
				lines.push('');
				return;
			case 'heading': {
				const level = typeof node.attrs?.level === 'number' ? node.attrs.level : 1;
				lines.push(`${'#'.repeat(Math.min(6, Math.max(1, level)))} ${renderInline(node.content ?? [])}`);
				lines.push('');
				return;
			}
			case 'bulletList':
				for (const child of node.content ?? []) renderNode(child, depth + 1);
				lines.push('');
				return;
			case 'orderedList': {
				let nextIndex = 1;
				for (const child of node.content ?? []) {
					renderNode(child, depth + 1, nextIndex);
					nextIndex += 1;
				}
				lines.push('');
				return;
			}
			case 'listItem': {
				const firstParagraph = (node.content ?? []).find((child) => child.type === 'paragraph');
				const fallbackContent = firstParagraph?.content ?? node.content ?? [];
				const marker = orderedIndex > 0 ? `${orderedIndex}.` : '-';
				lines.push(`${'  '.repeat(Math.max(0, depth - 1))}${marker} ${renderInline(fallbackContent)}`);
				for (const child of node.content ?? []) {
					if (child.type === 'paragraph') continue;
					renderNode(child, depth);
				}
				return;
			}
			case 'blockquote': {
				const nestedLines: string[] = [];
				const originalLines = lines.splice(0, lines.length);
				for (const child of node.content ?? []) renderNode(child, depth);
				nestedLines.push(...lines);
				lines.splice(0, lines.length, ...originalLines);
				for (const line of nestedLines.filter((entry) => entry !== '')) {
					lines.push(`> ${line}`);
				}
				lines.push('');
				return;
			}
			case 'codeBlock':
				lines.push(`\`\`\`${normalizeCodeBlockLanguageInfo(node.attrs?.language)}`);
				lines.push(getText(node));
				lines.push('```');
				lines.push('');
				return;
			case 'horizontalRule':
				lines.push('---');
				lines.push('');
				return;
			case 'blockMath':
				lines.push('$$');
				lines.push(typeof node.attrs?.latex === 'string' ? node.attrs.latex : '');
				lines.push('$$');
				lines.push('');
				return;
			case 'image': {
				const src = typeof node.attrs?.src === 'string' ? node.attrs.src : '';
				if (src) {
					const title = typeof node.attrs?.title === 'string' ? node.attrs.title : '';
					const alt = typeof node.attrs?.alt === 'string' ? node.attrs.alt : title;
					lines.push(`![${escapeMarkdown(alt)}](${src})`);
					lines.push('');
				}
				return;
			}
			default: {
				const inline = renderInline(node.content ?? []);
				if (inline.trim() !== '') {
					lines.push(inline);
					lines.push('');
				}
			}
		}
	}

	renderNode(contentJson as ExportNode);

	while (lines.length > 0 && lines[lines.length - 1].trim() === '') {
		lines.pop();
	}

	return lines.join('\n');
}

function escapeBBCodeText(value: string): string {
	return value.replaceAll('[', '&#91;').replaceAll(']', '&#93;');
}

export function exportBBCode(contentJson: JSONContent): string {
	const lines: string[] = [];

	function renderInline(nodes: ExportNode[] = []): string {
		let output = '';

		for (const node of nodes) {
			switch (node.type) {
				case 'text': {
					let text = escapeBBCodeText(node.text ?? '');
					const marks = node.marks ?? [];
					const isCode = marks.some((mark) => mark.type === 'code');
					const isBold = marks.some((mark) => mark.type === 'bold');
					const isItalic = marks.some((mark) => mark.type === 'italic');
					const linkMark = marks.find((mark) => mark.type === 'link');

					if (isCode) {
						text = `[icode]${text}[/icode]`;
					} else {
						if (isBold) text = `[b]${text}[/b]`;
						if (isItalic) text = `[i]${text}[/i]`;
					}

					if (linkMark?.attrs?.href && typeof linkMark.attrs.href === 'string') {
						text = `[url=${linkMark.attrs.href}]${text || linkMark.attrs.href}[/url]`;
					}

					output += text;
					break;
				}
				case 'hardBreak':
					output += '\n';
					break;
				case 'inlineMath':
					output += `[icode]$${typeof node.attrs?.latex === 'string' ? node.attrs.latex : ''}$[/icode]`;
					break;
				case 'image': {
					const src = typeof node.attrs?.src === 'string' ? node.attrs.src : '';
					if (src) {
						output += `[img]${src}[/img]`;
					}
					break;
				}
				default:
					output += renderInline(node.content ?? []);
			}
		}

		return output;
	}

	function renderTable(node: ExportNode) {
		const rows = (node.content ?? []).filter((child) => child.type === 'tableRow');
		if (rows.length === 0) {
			return;
		}

		lines.push('[code]');
		for (const row of rows) {
			const cells = (row.content ?? []).map((cell) =>
				renderInline(cell.content ?? []).replaceAll('\n', ' ').trim()
			);
			lines.push(cells.join(' | '));
		}
		lines.push('[/code]');
		lines.push('');
	}

	function renderNode(node: ExportNode | undefined, depth = 0, orderedIndex = 1) {
		if (!node) return;

		switch (node.type) {
			case 'doc':
				for (const child of node.content ?? []) renderNode(child, depth);
				return;
			case 'paragraph': {
				const text = renderInline(node.content ?? []);
				lines.push(text);
				lines.push('');
				return;
			}
			case 'heading': {
				const text = renderInline(node.content ?? []);
				lines.push(`[b][size=5]${text}[/size][/b]`);
				lines.push('');
				return;
			}
			case 'bulletList':
				lines.push('[list]');
				for (const child of node.content ?? []) renderNode(child, depth + 1);
				lines.push('[/list]');
				lines.push('');
				return;
			case 'orderedList': {
				let nextIndex = 1;
				lines.push('[list=1]');
				for (const child of node.content ?? []) {
					renderNode(child, depth + 1, nextIndex);
					nextIndex += 1;
				}
				lines.push('[/list]');
				lines.push('');
				return;
			}
			case 'listItem': {
				const firstParagraph = (node.content ?? []).find((child) => child.type === 'paragraph');
				const fallbackContent = firstParagraph?.content ?? node.content ?? [];
				lines.push(`[*]${renderInline(fallbackContent)}`);
				for (const child of node.content ?? []) {
					if (child.type === 'paragraph') continue;
					renderNode(child, depth, orderedIndex);
				}
				return;
			}
			case 'blockquote':
				lines.push('[quote]');
				for (const child of node.content ?? []) renderNode(child, depth);
				lines.push('[/quote]');
				lines.push('');
				return;
			case 'codeBlock':
				lines.push('[code]');
				lines.push(getText(node));
				lines.push('[/code]');
				lines.push('');
				return;
			case 'horizontalRule':
				lines.push('[hr]');
				lines.push('');
				return;
			case 'blockMath':
				lines.push('[code]');
				lines.push(typeof node.attrs?.latex === 'string' ? node.attrs.latex : '');
				lines.push('[/code]');
				lines.push('');
				return;
			case 'table':
				renderTable(node);
				return;
			case 'image': {
				const src = typeof node.attrs?.src === 'string' ? node.attrs.src : '';
				if (src) {
					lines.push(`[img]${src}[/img]`);
					lines.push('');
				}
				return;
			}
			default: {
				const inline = renderInline(node.content ?? []);
				if (inline.trim() !== '') {
					lines.push(inline);
					lines.push('');
				}
			}
		}
	}

	renderNode(contentJson as ExportNode);

	while (lines.length > 0 && lines[lines.length - 1].trim() === '') {
		lines.pop();
	}

	return lines.join('\n');
}

export function downloadTextFile(filename: string, content: string, mimeType: string) {
	const blob = new Blob([content], { type: mimeType });
	const url = URL.createObjectURL(blob);
	const anchor = document.createElement('a');
	anchor.href = url;
	anchor.download = filename;
	document.body.appendChild(anchor);
	anchor.click();
	anchor.remove();
	URL.revokeObjectURL(url);
}

export async function copyToClipboard(text: string): Promise<boolean> {
	if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
		try {
			await navigator.clipboard.writeText(text);
			return true;
		} catch {
			// fallback below
		}
	}

	try {
		const textarea = document.createElement('textarea');
		textarea.value = text;
		textarea.style.position = 'fixed';
		textarea.style.left = '-9999px';
		document.body.appendChild(textarea);
		textarea.focus();
		textarea.select();
		const copied = document.execCommand('copy');
		textarea.remove();
		return copied;
	} catch {
		return false;
	}
}
