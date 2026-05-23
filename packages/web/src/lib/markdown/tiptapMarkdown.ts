import type { JSONContent } from '@tiptap/core';
import Link from '@tiptap/extension-link';
import Mathematics from '@tiptap/extension-mathematics';
import { MarkdownManager } from '@tiptap/markdown';
import StarterKit from '@tiptap/starter-kit';
import { Table } from '@tiptap/extension-table';
import { TableCell } from '@tiptap/extension-table-cell';
import { TableHeader } from '@tiptap/extension-table-header';
import { TableRow } from '@tiptap/extension-table-row';
import { CyImage } from '$lib/components/editor/CyImage';
import { createCodeBlockLowlightExtension } from '$lib/components/editor/codeHighlight';

const EMPTY_DOC: JSONContent = {
	type: 'doc',
	content: [{ type: 'paragraph' }]
};

let manager: MarkdownManager | null = null;

function getMarkdownManager(): MarkdownManager {
	if (!manager) {
		manager = new MarkdownManager({
			extensions: [
				StarterKit.configure({
					heading: {
						levels: [1, 2, 3, 4, 5, 6]
					},
					codeBlock: false,
					link: false
				}),
				createCodeBlockLowlightExtension(),
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
			],
			markedOptions: {
				gfm: true,
				breaks: true
			}
		});
	}

	return manager;
}

function normalizeDoc(value: JSONContent | null | undefined): JSONContent {
	if (!value || value.type !== 'doc') {
		return EMPTY_DOC;
	}
	if (!Array.isArray(value.content) || value.content.length === 0) {
		return EMPTY_DOC;
	}
	return value;
}

export function markdownToTiptapJSON(markdown: string): JSONContent {
	return normalizeDoc(getMarkdownManager().parse(markdown));
}

export function tiptapJSONToMarkdown(contentJson: JSONContent): string {
	return getMarkdownManager().serialize(normalizeDoc(contentJson)).trim();
}
