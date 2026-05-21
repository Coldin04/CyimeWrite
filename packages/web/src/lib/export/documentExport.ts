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
import { common, createLowlight } from 'lowlight';
import { CyImage } from '$lib/components/editor/CyImage';

type ExportNode = {
	type?: string;
	text?: string;
	attrs?: Record<string, unknown>;
	content?: ExportNode[];
	marks?: Array<{ type?: string; attrs?: Record<string, unknown> }>;
};

type LowlightNode = {
	type?: string;
	value?: string;
	properties?: {
		className?: string[];
	};
	children?: LowlightNode[];
};

const lowlight = createLowlight(common);

lowlight.registerAlias({
	bash: ['sh', 'shell'],
	c: ['h'],
	cpp: ['cc', 'cxx', 'hpp']
});

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

function renderLowlightNodes(nodes: LowlightNode[] = []): string {
	return nodes
		.map((node) => {
			if (node.type === 'text') {
				return escapeHtml(node.value ?? '');
			}

			const children = renderLowlightNodes(node.children ?? []);
			const className = node.properties?.className?.filter(Boolean).join(' ') ?? '';
			if (!className) {
				return children;
			}

			return `<span class="${escapeHtml(className)}">${children}</span>`;
		})
		.join('');
}

function highlightCodeForExport(source: string, language: string): string {
	if (isMermaidLanguage(language)) {
		return escapeHtml(source);
	}

	try {
		if (language) {
			try {
				const result = lowlight.highlight(language, source);
				return renderLowlightNodes((result.children ?? []) as LowlightNode[]);
			} catch {
				// fall back to auto-detection below
			}
		}
		const result = lowlight.highlightAuto(source);
		return renderLowlightNodes((result.children ?? []) as LowlightNode[]);
	} catch {
		return escapeHtml(source);
	}
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

function isMermaidLanguage(value: string | null | undefined): boolean {
	return value?.trim().toLowerCase() === 'mermaid';
}

function isDarkMode(): boolean {
	if (typeof document !== 'undefined' && document.documentElement.classList.contains('dark')) {
		return true;
	}
	return (
		typeof window !== 'undefined' &&
		window.matchMedia?.('(prefers-color-scheme: dark)').matches === true
	);
}

function hashString(value: string): string {
	let hash = 5381;
	for (let index = 0; index < value.length; index += 1) {
		hash = (hash * 33) ^ value.charCodeAt(index);
	}
	return (hash >>> 0).toString(36);
}

function getHexColorLuminance(hexColor: string): number | null {
	const normalized = hexColor.trim().replace(/^#/, '');
	const expanded =
		normalized.length === 3
			? normalized
					.split('')
					.map((value) => value + value)
					.join('')
			: normalized;

	if (!/^[0-9a-f]{6}$/i.test(expanded)) {
		return null;
	}

	const channels = [0, 2, 4].map((offset) => {
		const channel = Number.parseInt(expanded.slice(offset, offset + 2), 16) / 255;
		return channel <= 0.03928 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
	});

	return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}

function chooseTextColorForFill(fillColor: string): string | null {
	const luminance = getHexColorLuminance(fillColor);
	if (luminance === null) {
		return null;
	}

	return luminance > 0.45 ? '#111827' : '#ffffff';
}

function applyReadableMermaidStyleTextColors(source: string): string {
	return source
		.split('\n')
		.map((line) => {
			const styleMatch = line.match(/^(\s*style\s+\S+\s+)(.+)$/i);
			if (!styleMatch) {
				return line;
			}

			const [, prefix, declarations] = styleMatch;
			if (/(^|,)color\s*:/i.test(declarations)) {
				return line;
			}

			const fillMatch = declarations.match(/(?:^|,)fill\s*:\s*(#[0-9a-f]{3}(?:[0-9a-f]{3})?)(?=,|$)/i);
			const textColor = fillMatch ? chooseTextColorForFill(fillMatch[1]) : null;
			if (!textColor) {
				return line;
			}

			return `${prefix}${declarations},color:${textColor}`;
		})
		.join('\n');
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

function enhanceExportHtml(html: string): string {
	if (typeof DOMParser === 'undefined') return html;

	const doc = new DOMParser().parseFromString(html, 'text/html');

	for (const code of Array.from(doc.querySelectorAll('pre > code'))) {
		const pre = code.parentElement;
		if (!pre) continue;

		const language = Array.from(code.classList)
			.find((className) => className.startsWith('language-'))
			?.slice('language-'.length);
		if (language) {
			pre.setAttribute('data-language', language);
		}
		code.innerHTML = highlightCodeForExport(code.textContent ?? '', language ?? '');
	}

	for (const table of Array.from(doc.querySelectorAll('table'))) {
		const parent = table.parentElement;
		if (parent?.classList.contains('tableWrapper')) {
			continue;
		}

		const wrapper = doc.createElement('div');
		wrapper.className = 'tableWrapper';
		table.replaceWith(wrapper);
		wrapper.append(table);
	}

	return doc.body.innerHTML;
}

async function renderMermaidInHtml(html: string, colorMode: 'light' | 'dark'): Promise<string> {
	if (typeof DOMParser === 'undefined') return html;

	const doc = new DOMParser().parseFromString(html, 'text/html');
	const blocks = Array.from(doc.querySelectorAll('pre > code')).filter((node) => {
		const language = Array.from(node.classList)
			.find((className) => className.startsWith('language-'))
			?.slice('language-'.length);
		return isMermaidLanguage(language);
	});

	if (blocks.length === 0) {
		return doc.body.innerHTML;
	}

	const mermaid = (await import('mermaid')).default;
	const darkMode = colorMode === 'dark';
	mermaid.initialize({
		startOnLoad: false,
		securityLevel: 'strict',
		theme: 'base',
		themeVariables: {
			background: darkMode ? '#18181b' : '#ffffff',
			primaryColor: darkMode ? '#27272a' : '#eff6ff',
			primaryBorderColor: darkMode ? '#60a5fa' : '#2563eb',
			primaryTextColor: darkMode ? '#ffffff' : '#111827',
			secondaryColor: darkMode ? '#3f3f46' : '#fff7ed',
			secondaryBorderColor: darkMode ? '#fb923c' : '#f97316',
			secondaryTextColor: darkMode ? '#ffffff' : '#111827',
			tertiaryColor: darkMode ? '#27272a' : '#f8fafc',
			tertiaryBorderColor: darkMode ? '#38bdf8' : '#0284c7',
			tertiaryTextColor: darkMode ? '#ffffff' : '#111827',
			textColor: darkMode ? '#ffffff' : '#111827',
			nodeTextColor: darkMode ? '#ffffff' : '#111827',
			lineColor: darkMode ? '#e4e4e7' : '#374151',
			edgeLabelBackground: darkMode ? '#18181b' : '#ffffff',
			clusterBkg: darkMode ? '#27272a' : '#f8fafc',
			clusterBorder: darkMode ? '#71717a' : '#cbd5e1'
		}
	});

	for (const code of blocks) {
		const pre = code.parentElement;
		if (!pre) continue;

		const source = code.textContent ?? '';
		const figure = doc.createElement('figure');
		figure.className = 'cy-export-mermaid';

		const chart = doc.createElement('div');
		chart.className = 'cy-export-mermaid__chart';
		figure.append(chart);

		try {
			const result = await mermaid.render(
				`cy-export-mermaid-${hashString(source)}-${Math.random().toString(36).slice(2)}`,
				applyReadableMermaidStyleTextColors(source)
			);
			chart.innerHTML = result.svg;
		} catch (error) {
			chart.classList.add('cy-export-mermaid__chart--error');
			chart.textContent = error instanceof Error ? error.message : 'Mermaid render failed';
		}

		const details = doc.createElement('details');
		details.className = 'cy-export-code-details';
		const summary = doc.createElement('summary');
		summary.textContent = 'Mermaid source';
		details.append(summary, pre.cloneNode(true));
		figure.append(details);
		pre.replaceWith(figure);
	}

	return doc.body.innerHTML;
}

export async function exportHtmlDocument(options: {
	title: string;
	contentJson: JSONContent;
	includeKatexCssLink?: boolean;
	colorMode?: 'light' | 'dark';
}): Promise<string> {
	const { title, contentJson, includeKatexCssLink = true, colorMode = isDarkMode() ? 'dark' : 'light' } = options;
	const darkMode = colorMode === 'dark';
	const generatedBody = enhanceExportHtml(renderKatexInHtml(generateHTML(contentJson, exportExtensions())));
	const renderedBody = await renderMermaidInHtml(generatedBody, colorMode);
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
    :root {
      color-scheme: ${darkMode ? 'dark' : 'light'};
      --page-bg: ${darkMode ? '#09090b' : '#ffffff'};
      --text: ${darkMode ? '#f4f4f5' : '#18181b'};
      --muted: ${darkMode ? '#a1a1aa' : '#71717a'};
      --border: ${darkMode ? '#3f3f46' : '#d4d4d8'};
      --surface: ${darkMode ? '#18181b' : '#fafafa'};
      --surface-soft: ${darkMode ? '#27272a' : '#f4f4f5'};
      --code-bg: ${darkMode ? '#18181b' : '#f8fafc'};
      --code-fg: ${darkMode ? '#e5e7eb' : '#1f2937'};
      --code-border: ${darkMode ? '#52525b' : '#cbd5e1'};
      --code-label: ${darkMode ? '#a1a1aa' : '#64748b'};
      --code-comment: ${darkMode ? '#a1a1aa' : '#64748b'};
      --code-string: ${darkMode ? '#86efac' : '#15803d'};
      --code-number: ${darkMode ? '#fdba74' : '#b45309'};
      --code-keyword: ${darkMode ? '#93c5fd' : '#1d4ed8'};
      --code-type: ${darkMode ? '#67e8f9' : '#0e7490'};
      --code-title: ${darkMode ? '#fde68a' : '#a16207'};
      --code-variable: ${darkMode ? '#c4b5fd' : '#7e22ce'};
      --code-operator: ${darkMode ? '#e5e7eb' : '#374151'};
      --accent: ${darkMode ? '#38bdf8' : '#0284c7'};
      --table-head: ${darkMode ? '#27272a' : '#f4f4f5'};
      --table-stripe: ${darkMode ? '#18181b' : '#fafafa'};
      --danger: ${darkMode ? '#f87171' : '#dc2626'};
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      padding: 48px 20px;
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      color: var(--text);
      background: var(--page-bg);
      line-height: 1.72;
    }
    main { max-width: 920px; margin: 0 auto; }
    h1, h2, h3, h4, h5, h6 { line-height: 1.25; margin: 1.55em 0 0.65em; }
    p { margin: 0.8em 0; }
    a { color: var(--accent); text-decoration-thickness: 0.08em; text-underline-offset: 0.18em; }
    img { display: block; max-width: 100%; height: auto; margin: 1rem auto; border-radius: 8px; }
    code {
      border-radius: 0.35rem;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-size: 0.92em;
    }
    :not(pre) > code {
      background: var(--surface-soft);
      color: var(--text);
      padding: 0.12rem 0.28rem;
    }
    pre {
      position: relative;
      overflow-x: auto;
      margin: 1rem 0;
      padding: 0.95rem 1rem;
      border: 0;
      border-left: 3px solid var(--code-border);
      border-radius: 6px;
      background: var(--code-bg);
      color: var(--code-fg);
      line-height: 1.65;
      page-break-inside: avoid;
    }
    pre[data-language] { padding-top: 1.9rem; }
    pre[data-language]::before {
      content: attr(data-language);
      position: absolute;
      top: 0.45rem;
      right: 0.65rem;
      color: var(--code-label);
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      font-size: 0.68rem;
      font-weight: 700;
      letter-spacing: 0.04em;
      text-transform: uppercase;
    }
    pre code { background: transparent; color: inherit; padding: 0; }
    .hljs-comment,
    .hljs-quote { color: var(--code-comment); font-style: italic; }
    .hljs-string,
    .hljs-regexp,
    .hljs-symbol { color: var(--code-string); }
    .hljs-number,
    .hljs-literal { color: var(--code-number); }
    .hljs-keyword,
    .hljs-meta { color: var(--code-keyword); font-weight: 650; }
    .hljs-type,
    .hljs-built_in { color: var(--code-type); }
    .hljs-title,
    .hljs-title.function_,
    .hljs-name { color: var(--code-title); }
    .hljs-variable,
    .hljs-params,
    .hljs-attr { color: var(--code-variable); }
    .hljs-operator,
    .hljs-punctuation { color: var(--code-operator); }
    blockquote {
      margin: 1.25rem 0;
      padding: 0.85rem 1rem;
      border-left: 3px solid var(--accent);
      border-radius: 0 8px 8px 0;
      background: var(--surface-soft);
      color: var(--text);
    }
    .tableWrapper { overflow-x: auto; margin: 0.9rem 0; }
    table {
      width: 100%;
      border-collapse: separate;
      border-spacing: 0;
      overflow: hidden;
      border: 1px solid var(--border);
      border-radius: 6px;
      font-size: 0.88rem;
      line-height: 1.38;
    }
    th, td {
      min-width: 4.75rem;
      border-right: 1px solid var(--border);
      border-bottom: 1px solid var(--border);
      padding: 0.42rem 0.55rem;
      text-align: left;
      vertical-align: top;
    }
    th:last-child, td:last-child { border-right: 0; }
    tr:last-child > th, tr:last-child > td { border-bottom: 0; }
    th { background: var(--table-head); font-weight: 650; }
    tbody tr:nth-child(even) td { background: var(--table-stripe); }
    .cy-export-mermaid {
      margin: 1.25rem 0;
      overflow: hidden;
      border: 1px solid var(--border);
      border-radius: 8px;
      background: var(--surface);
      page-break-inside: avoid;
    }
    .cy-export-mermaid__chart {
      overflow-x: auto;
      padding: 1rem;
      text-align: center;
    }
    .cy-export-mermaid__chart svg {
      display: inline-block;
      max-width: 100%;
      height: auto;
      vertical-align: middle;
    }
    .cy-export-mermaid__chart--error {
      color: var(--danger);
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-size: 0.85rem;
      text-align: left;
      white-space: pre-wrap;
    }
    .cy-export-code-details {
      border-top: 1px solid var(--border);
      background: var(--page-bg);
    }
    .cy-export-code-details summary {
      cursor: pointer;
      padding: 0.45rem 0.75rem;
      color: var(--muted);
      font-size: 0.78rem;
      font-weight: 650;
    }
    .cy-export-code-details pre {
      margin: 0;
      border: 0;
      border-radius: 0;
    }
    @media print {
      :root {
        color-scheme: light;
        --page-bg: #ffffff;
        --text: #18181b;
        --muted: #52525b;
        --border: #d4d4d8;
        --surface: #ffffff;
        --surface-soft: #f4f4f5;
        --code-bg: #f8fafc;
        --code-fg: #111827;
        --code-border: #cbd5e1;
        --code-label: #64748b;
        --code-comment: #64748b;
        --code-string: #166534;
        --code-number: #9a3412;
        --code-keyword: #1d4ed8;
        --code-type: #0e7490;
        --code-title: #854d0e;
        --code-variable: #6b21a8;
        --code-operator: #374151;
        --accent: #0369a1;
        --table-head: #f1f5f9;
        --table-stripe: #fafafa;
      }
      body { padding: 0; background: #ffffff; }
      main { max-width: none; }
      pre, table, blockquote, .cy-export-mermaid { break-inside: avoid; }
      .tableWrapper { margin: 0.7rem 0; }
      table { font-size: 0.82rem; line-height: 1.28; }
      th, td { min-width: 4rem; padding: 0.32rem 0.42rem; }
      .cy-export-code-details:not([open]) { display: none; }
      a { color: #0369a1; }
    }
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
