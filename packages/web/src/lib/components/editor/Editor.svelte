<script lang="ts">
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { Editor, Extension } from '@tiptap/core';
	import type { Content, JSONContent } from '@tiptap/core';
	import { Plugin } from '@tiptap/pm/state';
	import Collaboration from '@tiptap/extension-collaboration';
	import CollaborationCursor from '@tiptap/extension-collaboration-cursor';
	import Link from '@tiptap/extension-link';
	import Mathematics from '@tiptap/extension-mathematics';
	import Placeholder from '@tiptap/extension-placeholder';
	import StarterKit from '@tiptap/starter-kit';
	import { Table } from '@tiptap/extension-table';
	import { TableCell } from '@tiptap/extension-table-cell';
	import { TableHeader } from '@tiptap/extension-table-header';
	import { TableRow } from '@tiptap/extension-table-row';
	import { marked } from 'marked';
	import * as m from '$paraglide/messages';
	import TextB from '~icons/ph/text-b';
	import TextItalic from '~icons/ph/text-italic';
	import ListBullets from '~icons/ph/list-bullets';
	import ListNumbers from '~icons/ph/list-numbers';
	import FloppyDisk from '~icons/ph/floppy-disk';
	import Eye from '~icons/ph/eye';
	import ArrowCounterClockwise from '~icons/ph/arrow-counter-clockwise';
	import ArrowClockwise from '~icons/ph/arrow-clockwise';
	import Quotes from '~icons/ph/quotes';
	import Code from '~icons/ph/code';
	import Minus from '~icons/ph/minus';
	import { CyImage, cyImageAlignments, cyImageWidths } from '$lib/components/editor/CyImage';
	import {
		createCodeBlockLowlightExtension,
		normalizeCodeBlockLanguage
	} from '$lib/components/editor/codeHighlight';
	import { createMermaidPreviewExtension } from '$lib/components/editor/mermaidPreview';
	import CodeLanguageMenu from '$lib/components/editor/CodeLanguageMenu.svelte';
	import ImageTitleControls from '$lib/components/editor/ImageTitleControls.svelte';
	import HeadingLevelMenu from '$lib/components/editor/HeadingLevelMenu.svelte';
	import ImageLayoutControls from '$lib/components/editor/ImageLayoutControls.svelte';
	import ImageReplaceButton from '$lib/components/editor/ImageReplaceButton.svelte';
	import ExportControls from '$lib/components/editor/ExportControls.svelte';
	import InlineMathControls from '$lib/components/editor/InlineMathControls.svelte';
	import LinkControls from '$lib/components/editor/LinkControls.svelte';
	import ImageSizeControls from '$lib/components/editor/ImageSizeControls.svelte';
	import ImageInsertDialog from '$lib/components/editor/ImageInsertDialog.svelte';
	import MathInputDialog from '$lib/components/editor/MathInputDialog.svelte';
	import {
		KATEX_MAX_EXPAND,
		KATEX_MAX_SIZE,
		MAX_MATH_LATEX_LENGTH,
		normalizeMathLatexInput,
		sanitizeMathContent,
		sanitizeMathLatexAttr
	} from '$lib/components/editor/mathValidation';
	import TableToolbarControls from '$lib/components/editor/TableToolbarControls.svelte';
	import { pasteDocumentImage, type EditorAPIError } from '$lib/api/editor';
	import type { DocumentImageTargetOption } from '$lib/components/editor/documentImageTargets';
	import type { ExportAction } from '$lib/export/exportActions';
	import { auth } from '$lib/stores/auth';
	import { realtimeConfig } from '$lib/stores/realtime';
	import { get } from 'svelte/store';
	import { toast } from 'svelte-sonner';
	import ImageSquare from '~icons/ph/image-square';
	import type { ProviderInstance } from '$lib/utils/yjsProvider';

	interface Props {
		documentId: string;
		content: JSONContent;
		externalContentOverride?: {
			token: number;
			content: JSONContent;
		} | null;
		currentImageTargetId?: string;
		currentImageTargetLabel?: string;
		imageTargetOptions?: DocumentImageTargetOption[];
		collaboration?: ProviderInstance | null;
		readOnly?: boolean;
		isUpdatingImageTarget?: boolean;
		isSaving?: boolean;
		hasUnsavedChanges?: boolean;
		onContentChange?: (
			content: JSONContent,
			meta?: {
				viaCollaboration: boolean;
				isLocalChange: boolean;
			}
		) => void;
		onImageTargetChange?: (targetId: string) => void | Promise<unknown>;
		hydrateManagedContent?: (content: JSONContent) => Promise<JSONContent>;
		onSave?: () => void | Promise<unknown>;
		onExportAction?: (action: ExportAction) => void | Promise<unknown>;
	}

	let {
		documentId,
		content,
		externalContentOverride = null,
		currentImageTargetId = 'managed-r2',
		currentImageTargetLabel = '',
		imageTargetOptions = [],
		collaboration = null,
		readOnly = false,
		isUpdatingImageTarget = false,
		isSaving = false,
		hasUnsavedChanges = false,
		onContentChange,
		onImageTargetChange,
		hydrateManagedContent,
		onSave,
		onExportAction
	}: Props = $props();

	const EMPTY_DOC: JSONContent = {
		type: 'doc',
		content: [{ type: 'paragraph' }]
	};

	let editorElement: HTMLDivElement | null = null;
	let editor: Editor | null = null;
	let lastSyncedContent = '';
	let editorRevision = $state(0);
	let uploadingImageCount = $state(0);
	let isImageInsertDialogOpen = $state(false);
	let isMathDialogOpen = $state(false);
	let mathDialogMode = $state<'inline' | 'block'>('inline');
	let mathDialogValue = $state('');
	let editingMathPosition = $state<number | null>(null);
	let hasSeededCollaborationContent = false;
	let hasHydratedManagedImages = false;
	let isNormalizingMathInput = false;
	let lastAppliedExternalContentOverrideToken = 0;
	const imageUploadToastId = 'editor-image-upload';

	const allowedImageMimeTypes = new Set([
		'image/png',
		'image/jpeg',
		'image/webp',
		'image/gif'
	]);
	const allowedImageExtensions = new Set(['png', 'jpg', 'jpeg', 'webp', 'gif']);
	const imageUploadAccept = '.png,.jpg,.jpeg,.webp,.gif,image/png,image/jpeg,image/webp,image/gif';
	const headingLevels = [1, 2, 3, 4, 5, 6] as const;
	const externalImagePathPattern = /\.(avif|gif|jpe?g|png|svg|webp)(?:$|[?#])/i;
	const LOCAL_CURSOR_COLOR = '#06b6d4';
	const remoteCursorPalette = [
		'#2563eb',
		'#9333ea',
		'#ea580c',
		'#dc2626',
		'#1d4ed8',
		'#be123c',
		'#0891b2',
		'#7c3aed',
		'#c2410c',
		'#b91c1c'
	] as const;
	const MathValidation = Extension.create({
		name: 'mathValidation',
		addProseMirrorPlugins() {
			return [
				new Plugin({
					appendTransaction: (_transactions, _oldState, newState) => {
						let transaction = newState.tr;
						let changed = false;

						newState.doc.descendants((node, pos) => {
							if (node.type.name !== 'inlineMath' && node.type.name !== 'blockMath') {
								return;
							}

							const latex = sanitizeMathLatexAttr(node.attrs?.latex);
							if (latex === node.attrs?.latex) {
								return;
							}

							transaction = transaction.setNodeMarkup(
								pos,
								undefined,
								{ ...node.attrs, latex },
								node.marks
							);
							changed = true;
						});

						return changed ? transaction : null;
					}
				})
			];
		}
	});

	function hashString(value: string): number {
		let hash = 0;
		for (let index = 0; index < value.length; index += 1) {
			hash = (hash * 31 + value.charCodeAt(index)) >>> 0;
		}
		return hash;
	}

	function getCollaborationUser() {
		const authState = $auth;
		const id = authState.user?.id ?? collaboration?.provider?.awareness?.clientID?.toString() ?? 'unknown';
		const name =
			authState.user?.displayName?.trim() ||
			authState.user?.email?.trim() ||
			`协作者 ${id.slice(0, 6)}`;
		const color = LOCAL_CURSOR_COLOR;

		return { id, name, color };
	}

	function getRemoteCursorColor(userId: string) {
		return remoteCursorPalette[hashString(userId) % remoteCursorPalette.length];
	}

	function renderCollaborationCursor(
		user: { id?: string; name?: string; color?: string },
		localUserId: string
	) {
		const cursor = document.createElement('span');
		cursor.classList.add('collaboration-cursor__caret');
		const isLocal = user.id === localUserId;
		const effectiveColor = isLocal
			? LOCAL_CURSOR_COLOR
			: (user.color ?? getRemoteCursorColor(user.id ?? user.name ?? 'remote-user'));
		cursor.style.setProperty('--user-color', effectiveColor);

		const label = document.createElement('span');
		label.classList.add('collaboration-cursor__label', 'cw-cursor-label');

		const dot = document.createElement('span');
		dot.classList.add('cw-cursor-dot');
		dot.style.backgroundColor = effectiveColor;

		const text = document.createElement('span');
		text.classList.add('cw-cursor-name');
		text.textContent = user.name || '协作者';

		label.append(dot, text);
		cursor.append(label);
		return cursor;
	}

	function sanitizePastedHTML(html: string): string {
		const parser = new DOMParser();
		const doc = parser.parseFromString(html, 'text/html');
		const allowedDataImageMimeTypes = new Set([
			'image/png',
			'image/jpeg',
			'image/webp',
			'image/gif'
		]);

		const isAllowedImageSrc = (src: string): boolean => {
			const value = src.trim();
			if (!value) return false;

			if (value.startsWith('data:')) {
				const mimeType = value.slice(5).split(';', 1)[0]?.toLowerCase() ?? '';
				return allowedDataImageMimeTypes.has(mimeType);
			}

			try {
				const parsed = new URL(value, window.location.origin);
				return parsed.protocol === 'http:' || parsed.protocol === 'https:';
			} catch {
				return false;
			}
		};

		const sanitizeCodeLanguageClass = (element: Element): boolean => {
			const isCodeBlockCode =
				element.tagName.toLowerCase() === 'code' &&
				element.parentElement?.tagName.toLowerCase() === 'pre';
			if (!isCodeBlockCode) {
				return false;
			}

			const languageClass = [...element.classList].find((className) =>
				className.toLowerCase().startsWith('language-')
			);
			const language = normalizeCodeBlockLanguage(languageClass?.slice('language-'.length) ?? '');
			if (!language) {
				element.removeAttribute('class');
				return true;
			}

			element.setAttribute('class', `language-${language}`);
			return true;
		};

		doc.querySelectorAll('script, style, meta, link').forEach((node) => node.remove());

		doc.querySelectorAll('img').forEach((img) => {
			const src = img.getAttribute('src')?.trim();
			if (!src || !isAllowedImageSrc(src)) {
				img.remove();
				return;
			}
		});

		doc.querySelectorAll('*').forEach((element) => {
			for (const attr of [...element.attributes]) {
				const attrName = attr.name.toLowerCase();
				const isUnsafe =
					attrName === 'style' ||
					(attrName === 'class' && !sanitizeCodeLanguageClass(element)) ||
					attrName === 'id' ||
					attrName.startsWith('on') ||
					attrName.startsWith('data-') ||
					attrName.startsWith('aria-');
				if (isUnsafe) {
					element.removeAttribute(attr.name);
				}
			}
		});

		// Flatten styling-only wrappers so pasted content keeps structure but not noisy spans.
		doc.querySelectorAll('span').forEach((span) => {
			const parent = span.parentNode;
			if (!parent) return;
			while (span.firstChild) {
				parent.insertBefore(span.firstChild, span);
			}
			parent.removeChild(span);
		});

		return doc.body.innerHTML;
	}

	function looksLikeMarkdown(text: string): boolean {
		const sample = text.trim();
		if (!sample) return false;

		const markdownPatterns = [
			/^#{1,6}\s/m,
			/^\s*[-*+]\s+/m,
			/^\s*\d+\.\s+/m,
			/```[\s\S]*```/m,
			/`[^`\n]+`/,
			/\[[^\]]+\]\([^)]+\)/,
			/!\[[^\]]*\]\([^)]+\)/,
			/\*\*[^*\n]+\*\*/,
			/\*[^*\n]+\*/,
			/^>\s+/m
		];

		return markdownPatterns.some((pattern) => pattern.test(sample));
	}

	function normalizeDoc(value: JSONContent | null | undefined): JSONContent {
		if (!value || value.type !== 'doc') {
			return EMPTY_DOC;
		}
		if (!Array.isArray(value.content) || value.content.length === 0) {
			return EMPTY_DOC;
		}
		return sanitizeMathContent(value);
	}

	function toTiptapContent(value: JSONContent): Content {
		return normalizeDoc(value);
	}

	function serializeDoc(value: JSONContent): string {
		return JSON.stringify(normalizeDoc(value));
	}

	function hasMeaningfulContent(value: JSONContent): boolean {
		return serializeDoc(value) !== serializeDoc(EMPTY_DOC);
	}

	function isSupportedImageFile(file: File): boolean {
		const type = file.type.trim().toLowerCase();
		if (type && allowedImageMimeTypes.has(type)) {
			return true;
		}

		const ext = file.name.split('.').pop()?.trim().toLowerCase() ?? '';
		return ext !== '' && allowedImageExtensions.has(ext);
	}

	function showUnsupportedImageUploadToast(file: File) {
		const ext = file.name.split('.').pop()?.trim().toLowerCase() ?? 'unknown';
		toast.error(`暂不支持上传 ${ext.toUpperCase()}，请使用 PNG/JPG/WebP/GIF`);
	}

	function closeMathDialog() {
		isMathDialogOpen = false;
		mathDialogValue = '';
		editingMathPosition = null;
	}

	function openMathDialog(mode: 'inline' | 'block', latex = '', pos: number | null = null) {
		if (readOnly) {
			return;
		}

		mathDialogMode = mode;
		mathDialogValue = latex;
		editingMathPosition = pos;
		isMathDialogOpen = true;
	}

	async function submitMathDialog(rawLatex: string): Promise<boolean> {
		const editorInstance = editor;
		if (!editorInstance) {
			return false;
		}

		const latex = normalizeMathLatexInput(rawLatex);
		if (!latex) {
			toast.error(`请输入 1-${MAX_MATH_LATEX_LENGTH} 个字符的 LaTeX 公式内容`);
			return false;
		}

		const editingPos = editingMathPosition ?? undefined;
		const command =
			mathDialogMode === 'inline'
				? editingPos === undefined
					? () => editorInstance.chain().focus().insertInlineMath({ latex }).run()
					: () => editorInstance.chain().focus().updateInlineMath({ latex, pos: editingPos }).run()
				: editingPos === undefined
					? () => editorInstance.chain().focus().insertBlockMath({ latex }).run()
					: () => editorInstance.chain().focus().updateBlockMath({ latex, pos: editingPos }).run();

		const succeeded = command();
		if (!succeeded) {
			toast.error('公式插入失败');
			return false;
		}

		editorRevision += 1;
		closeMathDialog();
		return true;
	}

	async function deleteMathDialogTarget(): Promise<boolean> {
		const editorInstance = editor;
		if (!editorInstance || mathDialogMode !== 'block') {
			return false;
		}

		const editingPos = editingMathPosition ?? undefined;
		const succeeded =
			editingPos === undefined
				? editorInstance.chain().focus().deleteBlockMath().run()
				: editorInstance.chain().focus().deleteBlockMath({ pos: editingPos }).run();
		if (!succeeded) {
			toast.error('公式删除失败');
			return false;
		}

		editorRevision += 1;
		closeMathDialog();
		return true;
	}

	function normalizeMarkdownMathInput(editorInstance: Editor): boolean {
		const inlineMathType = editorInstance.schema.nodes.inlineMath;
		const blockMathType = editorInstance.schema.nodes.blockMath;
		if (!inlineMathType) {
			return false;
		}

		const blockReplacements: Array<{ from: number; to: number; latex: string }> = [];
		const replacements: Array<{ kind: 'inline'; from: number; to: number; latex: string }> = [];

		editorInstance.state.doc.descendants((node, pos, parent) => {
			if (node.type.name === 'paragraph' && blockMathType) {
				const paragraphChildren = node.content.content;
				const hasOnlyTextAndBreaks = paragraphChildren.every(
					(child) => child.type.name === 'text' || child.type.name === 'hardBreak'
				);
				if (hasOnlyTextAndBreaks) {
					const paragraphText = paragraphChildren
						.map((child) => {
							if (child.type.name === 'hardBreak') return '\n';
							return child.text ?? '';
						})
						.join('');
					const lines = paragraphText.split('\n');
					if (
						lines.length >= 3 &&
						lines[0]?.trim() === '$$' &&
						lines[lines.length - 1]?.trim() === '$$'
					) {
						const latex = normalizeMathLatexInput(
							lines
								.slice(1, -1)
								.join('\n')
						);
						if (latex) {
							blockReplacements.push({
								from: pos,
								to: pos + node.nodeSize,
								latex
							});
							return false;
						}
					}
				}
			}

			if (!node.isText || !node.text || node.text.includes('\n')) {
				return;
			}
			if (parent?.type?.name === 'codeBlock') {
				return;
			}
			if (node.marks.some((mark) => mark.type.name === 'code')) {
				return;
			}

			const textValue = node.text;
			const inlinePattern = /(?<!\$)\$([^$\n]+?)\$(?!\$)/g;
			for (const match of textValue.matchAll(inlinePattern)) {
				const raw = match[0];
				const latex = normalizeMathLatexInput(match[1]);
				const start = match.index ?? -1;
				if (raw === undefined || start < 0 || !latex) {
					continue;
				}
				replacements.push({
					kind: 'inline',
					from: pos + start,
					to: pos + start + raw.length,
					latex
				});
			}
		});

		if (blockReplacements.length === 0 && replacements.length === 0) {
			return false;
		}

		const transaction = editorInstance.state.tr;
		for (const replacement of [...blockReplacements].sort((left, right) => right.from - left.from)) {
			if (!blockMathType) {
				continue;
			}
			const mappedFrom = transaction.mapping.map(replacement.from);
			const mappedTo = transaction.mapping.map(replacement.to);
			const fromResolved = transaction.doc.resolve(mappedFrom);
			const parent = fromResolved.parent;
			const index = fromResolved.index();
			if (!parent.canReplaceWith(index, index + 1, blockMathType)) {
				continue;
			}

			transaction.replaceWith(
				mappedFrom,
				mappedTo,
				blockMathType.create({ latex: replacement.latex })
			);
		}

		for (const replacement of [...replacements].sort((left, right) => right.from - left.from)) {
			const mappedFrom = transaction.mapping.map(replacement.from);
			const mappedTo = transaction.mapping.map(replacement.to);
			const fromResolved = transaction.doc.resolve(mappedFrom);
			if (fromResolved.parent.type.name === 'codeBlock') {
				continue;
			}
			if (fromResolved.marks().some((mark) => mark.type.name === 'code')) {
				continue;
			}
			const parent = fromResolved.parent;
			const index = fromResolved.index();
			if (!parent.canReplaceWith(index, index + 1, inlineMathType)) {
				continue;
			}

			transaction.replaceWith(
				mappedFrom,
				mappedTo,
				inlineMathType.create({ latex: replacement.latex })
			);
		}

		if (!transaction.docChanged) {
			return false;
		}

		isNormalizingMathInput = true;
		editorInstance.view.dispatch(transaction);
		isNormalizingMathInput = false;
		return true;
	}

	function parsePastedMathExpression(raw: string): { mode: 'inline' | 'block'; latex: string } | null {
		const trimmed = raw.trim();
		if (!trimmed) {
			return null;
		}

		const blockMatch = trimmed.match(/^\$\$([\s\S]+)\$\$$/);
		if (blockMatch) {
			const latex = normalizeMathLatexInput(blockMatch[1]);
			return latex ? { mode: 'block', latex } : null;
		}

		const inlineMatch = trimmed.match(/^\$([^$\n]+)\$$/);
		if (inlineMatch) {
			const latex = normalizeMathLatexInput(inlineMatch[1]);
			return latex ? { mode: 'inline', latex } : null;
		}

		return null;
	}

	function insertUploadedImage(attrs: Record<string, unknown>) {
		if (!editor) return;

		editor
			.chain()
			.focus()
			.insertContent([
				{
					type: 'image',
					attrs
				},
				{
					type: 'paragraph'
				}
			])
			.run();
	}

	function buildExternalImageTitle(src: string): string {
		try {
			const parsed = new URL(src);
			const filename = parsed.pathname.split('/').pop()?.trim() ?? '';
			return filename || parsed.hostname;
		} catch {
			return '';
		}
	}

	function normalizeExternalImageURL(raw: string): string {
		const trimmed = raw.trim();
		if (!trimmed) return '';

		const candidate = /^[a-zA-Z][a-zA-Z\d+\-.]*:/.test(trimmed) ? trimmed : `https://${trimmed}`;
		try {
			const parsed = new URL(candidate);
			if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
				return '';
			}
			return parsed.toString();
		} catch {
			return '';
		}
	}

	function isExternalImageURL(raw: string): boolean {
		const normalized = normalizeExternalImageURL(raw);
		if (!normalized) return false;

		try {
			const parsed = new URL(normalized);
			return externalImagePathPattern.test(`${parsed.pathname}${parsed.search}${parsed.hash}`);
		} catch {
			return false;
		}
	}

	function insertExternalImage(src: string): boolean {
		const normalized = normalizeExternalImageURL(src);
		if (!normalized || !isExternalImageURL(normalized)) {
			toast.error(m.editor_external_image_invalid());
			return false;
		}

		insertUploadedImage({
			src: normalized,
			title: buildExternalImageTitle(normalized)
		});
		return true;
	}

	function beginImageUpload() {
		uploadingImageCount += 1;
		toast.loading(
			uploadingImageCount > 1
				? `正在上传 ${uploadingImageCount} 张图片...`
				: m.common_uploading(),
			{ id: imageUploadToastId, duration: Infinity }
		);
	}

	function endImageUpload() {
		uploadingImageCount = Math.max(0, uploadingImageCount - 1);
		if (uploadingImageCount > 0) {
			toast.loading(`正在上传 ${uploadingImageCount} 张图片...`, {
				id: imageUploadToastId,
				duration: Infinity
			});
			return;
		}

		toast.dismiss(imageUploadToastId);
	}

	function getDocumentImageMaxBytes(): number | null {
		const value = get(realtimeConfig).config?.documentImageMaxBytes;
		return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : null;
	}

	function formatBytes(value: number): string {
		if (value < 1024) {
			return `${value} B`;
		}
		if (value < 1024 * 1024) {
			return `${(value / 1024).toFixed(1)} KB`;
		}
		return `${(value / (1024 * 1024)).toFixed(1)} MB`;
	}

	function resolveImageUploadErrorMessage(error: unknown): string {
		const apiError = error as EditorAPIError | undefined;
		switch (apiError?.code) {
			case 'DOCUMENT_IMAGE_UNSUPPORTED_FILE_TYPE':
				return m.editor_paste_only_support_image_files();
			case 'DOCUMENT_IMAGE_FILE_TOO_LARGE':
				return m.editor_image_file_too_large();
			case 'DOCUMENT_IMAGE_PROVIDER_NOT_CONFIGURED':
			case 'DOCUMENT_IMAGE_TARGET_NOT_SUPPORTED':
				return m.common_unknown_error();
			default:
				return error instanceof Error && error.message.trim() !== ''
					? error.message
					: m.editor_image_insert_upload_failed();
		}
	}

	async function uploadAndInsertImage(
		file: File,
		source: 'picker' | 'paste' = 'picker'
	): Promise<boolean> {
		if (!editor) return false;
		if (!isSupportedImageFile(file)) {
			showUnsupportedImageUploadToast(file);
			return false;
		}
		const maxBytes = getDocumentImageMaxBytes();
		if (maxBytes !== null && file.size > maxBytes) {
			toast.error(`图片过大：${formatBytes(file.size)}，当前上限为 ${formatBytes(maxBytes)}`);
			return false;
		}
		beginImageUpload();
		try {
			const uploaded = await pasteDocumentImage(documentId, file);
			insertUploadedImage({
				src: uploaded.url,
				title: file.name,
				...(uploaded.assetId ? { assetId: uploaded.assetId } : {})
			});
			return true;
		} catch (error) {
			console.error(`[${source === 'paste' ? 'Paste' : 'Upload'}] Failed to upload image:`, error);
			if (
				maxBytes !== null &&
				error instanceof TypeError &&
				error.message.includes('NetworkError')
			) {
				toast.error(
					`图片上传失败。若文件接近或超过后端限制，请控制在 ${formatBytes(maxBytes)} 以内。`
				);
			} else {
				toast.error(resolveImageUploadErrorMessage(error));
			}
			return false;
		} finally {
			endImageUpload();
		}
	}

	async function uploadAndInsertImages(files: Iterable<File>, source: 'picker' | 'paste' = 'picker') {
		const supportedFiles: File[] = [];
		let blockedCount = 0;
		let uploadedCount = 0;

		for (const file of files) {
			if (!isSupportedImageFile(file)) {
				blockedCount += 1;
				continue;
			}
			supportedFiles.push(file);
		}

		if (blockedCount > 0) {
			toast.error(
				blockedCount === 1
					? '检测到 1 个不支持的图片文件，已跳过。仅支持 PNG/JPG/WebP/GIF。'
					: `检测到 ${blockedCount} 个不支持的图片文件，已跳过。仅支持 PNG/JPG/WebP/GIF。`
			);
		}

		for (const file of supportedFiles) {
			const uploaded = await uploadAndInsertImage(file, source);
			if (uploaded) {
				uploadedCount += 1;
			}
		}

		if (uploadedCount > 0 && uploadingImageCount === 0) {
			toast.success(
				uploadedCount === 1 ? '图片上传完成' : `${uploadedCount} 张图片上传完成`
			);
		}
	}

	function hasClipboardFiles(clipboard: DataTransfer): boolean {
		return Array.from(clipboard.items).some((item) => item.kind === 'file') || clipboard.files.length > 0;
	}

	function collectClipboardImageFiles(clipboard: DataTransfer): File[] {
		const imageTypes = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif']);

		const files = [
			...Array.from(clipboard.items)
				.filter((item) => item.kind === 'file' && imageTypes.has(item.type))
				.map((item) => item.getAsFile())
				.filter((file): file is File => file !== null),
			...Array.from(clipboard.files).filter((file) => imageTypes.has(file.type))
		];

		const uniqueFiles = new Map<string, File>();
		for (const file of files) {
			const key = `${file.name}:${file.size}:${file.type}:${file.lastModified}`;
			if (!uniqueFiles.has(key)) {
				uniqueFiles.set(key, file);
			}
		}

		return [...uniqueFiles.values()];
	}

	function extractImageSourcesFromHTML(html: string): string[] {
		if (!html) return [];
		const parser = new DOMParser();
		const doc = parser.parseFromString(html, 'text/html');
		return Array.from(doc.querySelectorAll('img'))
			.map((img) => img.getAttribute('src')?.trim() ?? '')
			.filter((src) => src.length > 0);
	}

	async function srcToUploadFile(src: string): Promise<File | null> {
		try {
			const response = await fetch(src);
			if (!response.ok) return null;
			const blob = await response.blob();
			const ext = blob.type.split('/')[1] || 'png';
			return new File([blob], `pasted-image.${ext}`, { type: blob.type || 'image/png' });
		} catch {
			return null;
		}
	}

	let editorCleanup: (() => void) | null = null;

	function destroyEditor() {
		editorCleanup?.();
		editorCleanup = null;
		editor?.destroy();
		editor = null;
	}

	function createEditor() {
		if (!editorElement) {
			return;
		}

		lastSyncedContent = serializeDoc(content);
		const collaborationUser = getCollaborationUser();
		const extensions: any[] = [
			StarterKit.configure({
				heading: {
					levels: [...headingLevels]
				},
				codeBlock: false,
				link: false,
				...(collaboration?.doc ? { undoRedo: false } : {})
			}),
			createCodeBlockLowlightExtension(),
			createMermaidPreviewExtension(),
			CyImage.configure({
				inline: false,
				allowBase64: true
			}),
			Link.configure({
				openOnClick: false,
				autolink: true,
				defaultProtocol: 'https'
			}),
			MathValidation,
			Mathematics.configure({
				katexOptions: {
					throwOnError: false,
					strict: 'ignore',
					maxSize: KATEX_MAX_SIZE,
					maxExpand: KATEX_MAX_EXPAND
				},
				...(readOnly
					? {}
					: {
							blockOptions: {
								onClick: (node, pos) => {
									const latex = sanitizeMathLatexAttr(node.attrs?.latex);
									openMathDialog('block', latex, pos);
								}
							}
						})
			}),
			Table.configure({
				resizable: true,
				HTMLAttributes: {
					class: 'cw-editor-table'
				}
			}),
			TableRow,
			TableHeader,
			TableCell,
			...(!readOnly
				? [
						Placeholder.configure({
							placeholder: m.editor_placeholder()
						})
					]
				: [])
		];

		if (collaboration?.doc) {
			collaboration.provider?.setAwarenessField('user', collaborationUser);
			extensions.push(
				Collaboration.configure({
					document: collaboration.doc
				}),
				CollaborationCursor.configure({
					provider: collaboration.provider,
					user: collaborationUser,
					render: (user) => renderCollaborationCursor(user, collaborationUser.id)
				})
			);
		}

		const editorRootClass = [
			'tiptap',
			'min-h-full',
			'w-full',
			'px-4',
			'py-6',
			'text-base',
			'text-zinc-800',
			'outline-none',
			'dark:text-zinc-100',
			'sm:px-8',
			'lg:px-[14%]',
			collaboration?.doc ? 'cw-collab-mode' : ''
		]
			.filter(Boolean)
			.join(' ');

		const editorInstance = new Editor({
			element: editorElement,
			editable: !readOnly,
			extensions,
			content: collaboration?.doc ? undefined : toTiptapContent(content),
			editorProps: {
				transformPastedHTML: (html) => sanitizePastedHTML(html),
				handleDOMEvents: {
					paste: (_view, event) => {
						if (readOnly) {
							return false;
						}
						const clipboardEvent = event as ClipboardEvent;
						const clipboard = clipboardEvent.clipboardData;
						if (!clipboard) return false;

						const clipboardFiles = collectClipboardImageFiles(clipboard);
						if (clipboardFiles.length > 0) {
							clipboardEvent.preventDefault();
							void (async () => {
								await uploadAndInsertImages(clipboardFiles, 'paste');
							})();
							return true;
						}

						if (hasClipboardFiles(clipboard)) {
							clipboardEvent.preventDefault();
							toast.error(m.editor_paste_only_support_image_files());
							return true;
						}

						const html = clipboard.getData('text/html');
						const imageSources = extractImageSourcesFromHTML(html).filter((src) =>
							src.startsWith('data:image/') || src.startsWith('http://') || src.startsWith('https://')
						);
						if (imageSources.length > 0) {
							clipboardEvent.preventDefault();
							void (async () => {
								let blockedSourceCount = 0;
								for (const src of imageSources) {
									const file = await srcToUploadFile(src);
									if (!file) {
										blockedSourceCount += 1;
										continue;
									}
									await uploadAndInsertImage(file, 'paste');
								}

								if (blockedSourceCount > 0) {
									toast.error(
										'检测到不支持或无法读取的粘贴图片内容，已跳过。仅支持 PNG/JPG/WebP/GIF。'
									);
								}
							})();
							return true;
						}

						const text = clipboard.getData('text/plain');
						if (isExternalImageURL(text.trim())) {
							clipboardEvent.preventDefault();
							insertExternalImage(text);
							return true;
						}

						const pastedMath = parsePastedMathExpression(text);
						if (pastedMath) {
							clipboardEvent.preventDefault();
							const chain = editor?.chain().focus();
							const succeeded =
								pastedMath.mode === 'block'
									? chain?.insertBlockMath({ latex: pastedMath.latex }).run()
									: chain?.insertInlineMath({ latex: pastedMath.latex }).run();
							if (!succeeded) {
								toast.error(m.editor_math_insert_failed());
							}
							return true;
						}

						if (!looksLikeMarkdown(text)) return false;

						const rendered = marked.parse(text, {
							async: false,
							gfm: true,
							breaks: true
						});
						if (typeof rendered !== 'string') return false;

						clipboardEvent.preventDefault();
						editor
							?.chain()
							.focus()
							.insertContent(sanitizePastedHTML(rendered))
							.run();
						return true;
					}
				},
				attributes: {
					autocapitalize: 'off',
					autocomplete: 'off',
					autocorrect: 'off',
					class: editorRootClass,
					spellcheck: 'false'
				}
			},
			onUpdate: ({ editor }) => {
				if (readOnly) {
					return;
				}
				if (!isNormalizingMathInput && normalizeMarkdownMathInput(editor)) {
					return;
				}
				const nextContent = editor.getJSON();
				lastSyncedContent = serializeDoc(nextContent);
				onContentChange?.(nextContent, {
					viaCollaboration: Boolean(collaboration?.provider),
					isLocalChange: collaboration?.provider ? collaboration.provider.hasUnsyncedChanges : true
				});
				editorRevision += 1;
			},
			onSelectionUpdate: () => {
				editorRevision += 1;
			}
		});

		editor = editorInstance;

		const reconcileCollaborationContent = async () => {
			if (editor !== editorInstance || !collaboration?.doc) {
				return;
			}

			if (!hasSeededCollaborationContent) {
				hasSeededCollaborationContent = true;
				const currentDoc = normalizeDoc(editorInstance.getJSON());
				if (!hasMeaningfulContent(currentDoc) && hasMeaningfulContent(content)) {
					lastSyncedContent = serializeDoc(content);
					editorInstance.commands.setContent(toTiptapContent(content), { emitUpdate: false });
				}
			}

			if (hasHydratedManagedImages || !hydrateManagedContent) {
				return;
			}

			hasHydratedManagedImages = true;
			const currentDoc = normalizeDoc(editorInstance.getJSON());
			const hydrated = await hydrateManagedContent(currentDoc);
			if (editor !== editorInstance || serializeDoc(hydrated) === serializeDoc(currentDoc)) {
				return;
			}

			lastSyncedContent = serializeDoc(hydrated);
			editorInstance.commands.setContent(toTiptapContent(hydrated), { emitUpdate: false });
		};

		const handleSynced = ({ state }: { state: boolean }) => {
			if (state) {
				void reconcileCollaborationContent();
			}
		};

		const provider = collaboration?.provider;
		if (provider) {
			provider.on('synced', handleSynced);
			if (provider.synced) {
				void reconcileCollaborationContent();
			}
		} else if (collaboration?.doc) {
			void reconcileCollaborationContent();
		}

		editorCleanup = () => {
			provider?.off('synced', handleSynced);
		};
	}

	function getCollaborationKey() {
		return collaboration?.doc ? `collab:${documentId}` : `local:${documentId}`;
	}

	onMount(() => {
		previousCollaborationKey = getCollaborationKey();
		createEditor();

		return () => {
			destroyEditor();
		};
	});

	// Handle collaboration mode changes by recreating the editor
	let previousCollaborationKey = '';

	$effect(() => {
		const currentKey = getCollaborationKey();
		if (currentKey === previousCollaborationKey || !editorElement) {
			previousCollaborationKey = currentKey;
			return;
		}

		// Save current content before destroying the editor
		if (editor) {
			const currentContent = editor.getJSON();
			if (serializeDoc(currentContent) !== lastSyncedContent) {
				lastSyncedContent = serializeDoc(currentContent);
				onContentChange?.(currentContent, {
					viaCollaboration: Boolean(collaboration?.provider),
					isLocalChange: collaboration?.provider ? collaboration.provider.hasUnsyncedChanges : true
				});
			}
		}

		// Destroy old editor
		destroyEditor();
		hasSeededCollaborationContent = false;
		hasHydratedManagedImages = false;
		previousCollaborationKey = currentKey;

		// Recreate editor with new collaboration state
		createEditor();
	});

	$effect(() => {
		if (!editor || collaboration?.doc) {
			return;
		}

		if (serializeDoc(content) === lastSyncedContent) {
			return;
		}

		lastSyncedContent = serializeDoc(content);
		editor.commands.setContent(toTiptapContent(content), { emitUpdate: false });
	});

	$effect(() => {
		if (!editor || !externalContentOverride) {
			return;
		}

		if (externalContentOverride.token === lastAppliedExternalContentOverrideToken) {
			return;
		}
		lastAppliedExternalContentOverrideToken = externalContentOverride.token;

		const nextContent = normalizeDoc(externalContentOverride.content);
		if (serializeDoc(editor.getJSON()) === serializeDoc(nextContent)) {
			return;
		}

		editor.commands.setContent(toTiptapContent(nextContent), { emitUpdate: true });
	});

	function apply(action: (instance: Editor) => void) {
		if (!editor) return;
		action(editor);
		editorRevision += 1;
	}

	function isActive(name: string, attributes?: Record<string, unknown>) {
		editorRevision;
		if (!editor) return false;
		return editor.isActive(name, attributes);
	}

	function canUndo() {
		editorRevision;
		if (!editor) return false;
		return editor.can().chain().focus().undo().run();
	}

	function canRedo() {
		editorRevision;
		if (!editor) return false;
		return editor.can().chain().focus().redo().run();
	}

	function canApply(action: (instance: Editor) => boolean) {
		editorRevision;
		if (!editor) return false;
		return action(editor);
	}

	function currentHeadingValue() {
		editorRevision;
		if (!editor) return 'paragraph';
		for (const level of headingLevels) {
			if (editor.isActive('heading', { level })) {
				return `h${level}`;
			}
		}
		return 'paragraph';
	}

	function applyHeadingValue(value: string) {
		if (!editor) return;
		if (value === 'paragraph') {
			editor.chain().focus().setParagraph().run();
			editorRevision += 1;
			return;
		}

		const level = Number.parseInt(value.replace('h', ''), 10);
		if (!headingLevels.includes(level as (typeof headingLevels)[number])) {
			return;
		}

		editor.chain().focus().setHeading({ level: level as (typeof headingLevels)[number] }).run();
		editorRevision += 1;
	}

	function currentImageWidth() {
		editorRevision;
		if (!editor || !editor.isActive('image')) return 'auto';
		const attrs = editor.getAttributes('image');
		const width = typeof attrs.width === 'string' ? attrs.width : '';
		return cyImageWidths.includes(width as (typeof cyImageWidths)[number]) ? width : 'auto';
	}

	function currentImageAlign() {
		editorRevision;
		if (!editor || !editor.isActive('image')) return 'content';
		const attrs = editor.getAttributes('image');
		const align = typeof attrs.align === 'string' ? attrs.align : 'content';
		return cyImageAlignments.includes(align as (typeof cyImageAlignments)[number]) ? align : 'content';
	}

	function currentImageTitle() {
		editorRevision;
		if (!editor || !editor.isActive('image')) return '';
		const attrs = editor.getAttributes('image');
		return typeof attrs.title === 'string' ? attrs.title : '';
	}

	function currentImageDescription() {
		editorRevision;
		if (!editor || !editor.isActive('image')) return '';
		const attrs = editor.getAttributes('image');
		// 编辑态只展示真实 alt，避免用户无意中把 title 回填成持久化 alt。
		return typeof attrs.alt === 'string' ? attrs.alt : '';
	}

	function currentLinkHref() {
		editorRevision;
		if (!editor || !editor.isActive('link')) return '';
		const attrs = editor.getAttributes('link');
		return typeof attrs.href === 'string' ? attrs.href : '';
	}

	function currentInlineMathLatex() {
		editorRevision;
		if (!editor || !editor.isActive('inlineMath')) return '';
		const attrs = editor.getAttributes('inlineMath');
		return typeof attrs.latex === 'string' ? attrs.latex : '';
	}

	function currentCodeBlockLanguage() {
		editorRevision;
		if (!editor || !editor.isActive('codeBlock')) return '';
		const attrs = editor.getAttributes('codeBlock');
		return typeof attrs.language === 'string' ? attrs.language : '';
	}

	function applyImageWidth(width: string) {
		if (!editor || !editor.isActive('image')) {
			return;
		}

		editor
			.chain()
			.focus()
			.updateAttributes('image', {
				width: width === 'auto' ? null : width
			})
			.run();
		editorRevision += 1;
	}

	function applyImageAlign(align: string) {
		if (!editor || !editor.isActive('image')) {
			return;
		}

		editor
			.chain()
			.focus()
			.updateAttributes('image', {
				align
			})
			.run();
		editorRevision += 1;
	}

	function applyImageTitle(payload: { title: string; description: string }) {
		if (!editor || !editor.isActive('image')) {
			return;
		}

		const title = payload.title.trim();
		const description = payload.description.trim();
		const nextAlt = description === '' ? null : description;

		editor
			.chain()
			.focus()
			.updateAttributes('image', {
				title,
				// 描述为空时不落库 alt，渲染阶段再回退到 title。
				alt: nextAlt
			})
			.run();
		editorRevision += 1;
	}

	async function replaceCurrentImage(file: File) {
		if (!editor || !editor.isActive('image')) {
			return;
		}
		if (!isSupportedImageFile(file)) {
			showUnsupportedImageUploadToast(file);
			return;
		}

		beginImageUpload();
		try {
			const uploaded = await pasteDocumentImage(documentId, file);
			const attrs = editor.getAttributes('image');
			const currentAlt = typeof attrs.alt === 'string' ? attrs.alt : '';
			const nextTitle = file.name;
			const nextAlt = currentAlt.trim() === '' ? null : currentAlt;

			editor
				.chain()
				.focus()
				.updateAttributes('image', {
					src: uploaded.url,
					...(uploaded.assetId ? { assetId: uploaded.assetId } : { assetId: null }),
					title: nextTitle,
					alt: nextAlt
				})
				.run();
			editorRevision += 1;
			toast.success(m.editor_image_replace_success());
		} catch (error) {
			console.error('[Replace] Failed to replace image:', error);
			toast.error(resolveImageUploadErrorMessage(error));
		} finally {
			endImageUpload();
		}
	}

	function normalizeLinkHref(href: string) {
		const trimmed = href.trim();
		if (!trimmed) return '';

		// If the href already has a scheme, only allow a safe subset.
		const schemeMatch = trimmed.match(/^([a-zA-Z][a-zA-Z\d+\-.]*:)/);
		if (schemeMatch) {
			const scheme = schemeMatch[1].toLowerCase();
			const allowedSchemes = new Set(['http:', 'https:', 'mailto:']);
			if (!allowedSchemes.has(scheme)) {
				// Reject unsafe or unknown schemes like javascript:, data:, etc.
				return '';
			}
			return trimmed;
		}

		// No explicit scheme: default to https.
		return `https://${trimmed}`;
	}

	function applyLinkHref(href: string) {
		if (!editor) return;
		const normalizedHref = normalizeLinkHref(href);
		if (!normalizedHref) {
			editor.chain().focus().unsetLink().run();
			editorRevision += 1;
			return;
		}

		editor
			.chain()
			.focus()
			.extendMarkRange('link')
			.setLink({ href: normalizedHref })
			.run();
		editorRevision += 1;
	}

	function removeLink() {
		if (!editor) return;
		editor.chain().focus().extendMarkRange('link').unsetLink().run();
		editorRevision += 1;
	}

	function applyInlineMathLatex(latexInput: string) {
		if (!editor) return;
		const trimmedLatexInput = latexInput.trim();

		if (trimmedLatexInput === '') {
			if (editor.isActive('inlineMath')) {
				editor.chain().focus().deleteInlineMath().run();
				editorRevision += 1;
			}
			return;
		}

		const latex = normalizeMathLatexInput(trimmedLatexInput);
		if (!latex) {
			toast.error(`公式内容不能超过 ${MAX_MATH_LATEX_LENGTH} 个字符`);
			return;
		}

		const succeeded = editor.isActive('inlineMath')
			? editor.chain().focus().updateInlineMath({ latex }).run()
			: editor.chain().focus().insertInlineMath({ latex }).run();
		if (!succeeded) {
			toast.error('公式插入失败');
			return;
		}
		editorRevision += 1;
	}

	function applyCodeBlockLanguage(language: string) {
		if (!editor || !editor.isActive('codeBlock')) return;
		editor
			.chain()
			.focus()
			.updateAttributes('codeBlock', {
				language: language === '' ? null : language
			})
			.run();
		editorRevision += 1;
	}

	function removeInlineMath() {
		if (!editor || !editor.isActive('inlineMath')) return;
		editor.chain().focus().deleteInlineMath().run();
		editorRevision += 1;
	}

	const activeToggleClass = 'bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900';
	const inactiveToggleClass =
		'text-zinc-700 hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-800';
	const iconButtonBaseClass =
		'inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md leading-none transition-colors disabled:cursor-not-allowed disabled:opacity-50';
	const toolbarToggleButtonClass =
		'inline-flex h-8 shrink-0 items-center justify-center rounded-md px-2 text-xs leading-none transition-colors';
</script>

<div class="flex h-full w-full flex-col">
	{#if !readOnly}
		<div class="relative z-10 border-b border-zinc-200 px-3 py-2 dark:border-zinc-800">
			<div class="overflow-visible">
				<div
					class="mx-auto flex w-full max-w-6xl flex-nowrap items-center justify-start gap-2 overflow-x-auto whitespace-nowrap scrollbar-none md:justify-center"
				>
				<button
					type="button"
					title={m.editor_toolbar_save_with_shortcut()}
					aria-label={m.editor_toolbar_save_with_shortcut()}
					disabled={isSaving || !hasUnsavedChanges}
					onclick={() => onSave?.()}
					class={`${iconButtonBaseClass} text-zinc-700 hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-800`}
				>
					<FloppyDisk class="h-4 w-4" />
				</button>
				<ExportControls
					onAction={(action) => {
						void onExportAction?.(action);
					}}
				/>
			<a
				href={`/view/documents/${documentId}`}
				title={m.editor_toolbar_open_reader_mode()}
				aria-label={m.editor_toolbar_open_reader_mode()}
				class={`${iconButtonBaseClass} text-zinc-700 hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-800`}
			>
				<Eye class="h-4 w-4" />
			</a>
			<button
				type="button"
				title={m.editor_toolbar_undo_with_shortcut()}
				aria-label={m.editor_toolbar_undo_with_shortcut()}
				disabled={!canUndo()}
				onclick={() =>
					apply((instance) => {
						instance.chain().focus().undo().run();
					})}
				class={`${iconButtonBaseClass} text-zinc-700 hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-800`}
			>
				<ArrowCounterClockwise class="h-4 w-4" />
			</button>
			<button
				type="button"
				title={m.editor_toolbar_redo_with_shortcut()}
				aria-label={m.editor_toolbar_redo_with_shortcut()}
				disabled={!canRedo()}
				onclick={() =>
					apply((instance) => {
						instance.chain().focus().redo().run();
					})}
				class={`${iconButtonBaseClass} text-zinc-700 hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-800`}
			>
				<ArrowClockwise class="h-4 w-4" />
			</button>
			<div class="mx-0.5 h-5 w-px shrink-0 bg-zinc-200 dark:bg-zinc-700 md:mx-1"></div>
			<HeadingLevelMenu currentValue={currentHeadingValue()} onSelect={applyHeadingValue} />
			<div class="mx-0.5 h-5 w-px shrink-0 bg-zinc-200 dark:bg-zinc-700 md:mx-1"></div>
			<button
				type="button"
				title={m.editor_toolbar_bold()}
				aria-label={m.editor_toolbar_bold()}
				class={`${toolbarToggleButtonClass} font-semibold ${
					isActive('bold') ? activeToggleClass : inactiveToggleClass
				}`}
				onclick={() =>
					apply((instance) => {
						instance.chain().focus().toggleBold().run();
					})}
			>
				<TextB class="h-4 w-4" />
			</button>
			<button
				type="button"
				title={m.editor_toolbar_italic()}
				aria-label={m.editor_toolbar_italic()}
				class={`${toolbarToggleButtonClass} italic ${
					isActive('italic') ? activeToggleClass : inactiveToggleClass
				}`}
				onclick={() =>
					apply((instance) => {
						instance.chain().focus().toggleItalic().run();
					})}
			>
				<TextItalic class="h-4 w-4" />
			</button>
			<LinkControls href={currentLinkHref()} onSave={applyLinkHref} onRemove={removeLink} />
			<div class="mx-0.5 h-5 w-px shrink-0 bg-zinc-200 dark:bg-zinc-700 md:mx-1"></div>
			<button
				type="button"
				title={m.editor_toolbar_bullet_list()}
				aria-label={m.editor_toolbar_bullet_list()}
				class={`${toolbarToggleButtonClass} ${
					isActive('bulletList') ? activeToggleClass : inactiveToggleClass
				}`}
				onclick={() =>
					apply((instance) => {
						instance.chain().focus().toggleBulletList().run();
					})}
			>
				<ListBullets class="h-4 w-4" />
			</button>
			<button
				type="button"
				title={m.editor_toolbar_numbered_list()}
				aria-label={m.editor_toolbar_numbered_list()}
				class={`${toolbarToggleButtonClass} ${
					isActive('orderedList') ? activeToggleClass : inactiveToggleClass
				}`}
				onclick={() =>
					apply((instance) => {
						instance.chain().focus().toggleOrderedList().run();
					})}
			>
				<ListNumbers class="h-4 w-4" />
			</button>
			<button
				type="button"
				title={m.editor_toolbar_blockquote()}
				aria-label={m.editor_toolbar_blockquote()}
				class={`${toolbarToggleButtonClass} ${
					isActive('blockquote') ? activeToggleClass : inactiveToggleClass
				}`}
				onclick={() =>
					apply((instance) => {
						instance.chain().focus().toggleBlockquote().run();
					})}
			>
				<Quotes class="h-4 w-4" />
			</button>
			<button
				type="button"
				title={m.editor_toolbar_code_block()}
				aria-label={m.editor_toolbar_code_block()}
				class={`${toolbarToggleButtonClass} ${
					isActive('codeBlock') ? activeToggleClass : inactiveToggleClass
				}`}
				onclick={() =>
					apply((instance) => {
						instance.chain().focus().toggleCodeBlock().run();
					})}
			>
				<Code class="h-4 w-4" />
			</button>
			{#if isActive('codeBlock')}
				<CodeLanguageMenu currentValue={currentCodeBlockLanguage()} onSelect={applyCodeBlockLanguage} />
			{/if}
			<button
				type="button"
				title={m.editor_toolbar_divider()}
				aria-label={m.editor_toolbar_divider()}
				class={`${toolbarToggleButtonClass} ${inactiveToggleClass}`}
				onclick={() =>
					apply((instance) => {
						instance.chain().focus().setHorizontalRule().run();
					})}
			>
				<Minus class="h-4 w-4" />
			</button>
			<InlineMathControls
				latex={currentInlineMathLatex()}
				isActive={isActive('inlineMath')}
				onSave={applyInlineMathLatex}
				onRemove={removeInlineMath}
			/>
			<button
				type="button"
				title={m.editor_math_block_title()}
				aria-label={m.editor_math_block_title()}
				class={`${toolbarToggleButtonClass} text-[11px] font-medium ${
					isActive('blockMath') ? activeToggleClass : inactiveToggleClass
				}`}
				onclick={() => openMathDialog('block')}
			>
				math
			</button>
			<div class="mx-0.5 h-5 w-px shrink-0 bg-zinc-200 dark:bg-zinc-700 md:mx-1"></div>
			<button
				type="button"
				title={m.editor_toolbar_upload_image()}
				aria-label={m.editor_toolbar_upload_image()}
				disabled={uploadingImageCount > 0}
				class="inline-flex h-8 shrink-0 items-center justify-center gap-1.5 rounded-md px-2 leading-none text-zinc-700 transition-colors hover:bg-zinc-100 disabled:cursor-not-allowed disabled:opacity-50 dark:text-zinc-200 dark:hover:bg-zinc-800"
				onclick={() => (isImageInsertDialogOpen = true)}
			>
				<ImageSquare class="h-4 w-4" />
				{#if uploadingImageCount > 0}
					<span class="text-xs font-medium">{m.common_uploading()}</span>
				{/if}
			</button>
			{#if isActive('image')}
				<div
					in:fade={{ duration: 120 }}
					out:fade={{ duration: 120 }}
					class="inline-flex shrink-0 items-center gap-1 rounded-lg px-1 outline outline-1 -outline-offset-1 outline-zinc-200 dark:outline-zinc-700"
				>
					<ImageReplaceButton
						accept={imageUploadAccept}
						label={m.editor_image_replace()}
						onFileSelected={(file) => {
							void replaceCurrentImage(file);
						}}
					/>
					<ImageTitleControls
						titleValue={currentImageTitle()}
						descriptionValue={currentImageDescription()}
						onSave={applyImageTitle}
					/>
					<ImageSizeControls currentWidth={currentImageWidth()} onSelect={applyImageWidth} />
					<ImageLayoutControls currentAlign={currentImageAlign()} onSelect={applyImageAlign} />
				</div>
			{/if}
				<TableToolbarControls
				isTableActive={isActive('table')}
				isHeaderRowActive={isActive('tableHeader')}
				canInsertTable={canApply((instance) =>
					instance.can().chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()
				)}
				canAddRow={canApply((instance) => instance.can().chain().focus().addRowAfter().run())}
				canDeleteRow={canApply((instance) => instance.can().chain().focus().deleteRow().run())}
				canAddColumn={canApply((instance) => instance.can().chain().focus().addColumnAfter().run())}
				canDeleteColumn={canApply((instance) => instance.can().chain().focus().deleteColumn().run())}
				canToggleHeaderRow={canApply((instance) => instance.can().chain().focus().toggleHeaderRow().run())}
				canDeleteTable={canApply((instance) => instance.can().chain().focus().deleteTable().run())}
				onInsertTable={(rows, cols) =>
					apply((instance) => {
						instance.chain().focus().insertTable({ rows, cols, withHeaderRow: true }).run();
					})}
				onAddRow={() =>
					apply((instance) => {
						instance.chain().focus().addRowAfter().run();
					})}
				onDeleteRow={() =>
					apply((instance) => {
						instance.chain().focus().deleteRow().fixTables().run();
					})}
				onAddColumn={() =>
					apply((instance) => {
						instance.chain().focus().addColumnAfter().run();
					})}
				onDeleteColumn={() =>
					apply((instance) => {
						instance.chain().focus().deleteColumn().fixTables().run();
					})}
				onToggleHeaderRow={() =>
					apply((instance) => {
						instance.chain().focus().toggleHeaderRow().fixTables().run();
					})}
				onDeleteTable={() =>
					apply((instance) => {
						instance.chain().focus().deleteTable().run();
					})}
				/>
				</div>
			</div>
		</div>
	{/if}

	<div class="h-full w-full overflow-y-auto">
		<div bind:this={editorElement} class="h-full w-full"></div>
	</div>
</div>

{#if !readOnly}
	<ImageInsertDialog
		bind:open={isImageInsertDialogOpen}
		accept={imageUploadAccept}
		isUploading={uploadingImageCount > 0}
		isUpdatingTarget={isUpdatingImageTarget}
		currentTargetId={currentImageTargetId}
		currentTargetLabel={currentImageTargetLabel}
		targetOptions={imageTargetOptions}
		onTargetChange={(targetId) => onImageTargetChange?.(targetId)}
		onFilesSelected={(files) => {
			void uploadAndInsertImages(files, 'picker');
		}}
		onInsertLink={async (src) => insertExternalImage(src)}
	/>
{/if}

<MathInputDialog
	bind:open={isMathDialogOpen}
	mode={mathDialogMode}
	initialValue={mathDialogValue}
	showDelete={mathDialogMode === 'block' && editingMathPosition !== null}
	onSubmit={submitMathDialog}
	onDelete={deleteMathDialogTarget}
/>
