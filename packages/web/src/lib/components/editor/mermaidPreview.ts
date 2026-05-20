import { Extension, findChildren } from '@tiptap/core';
import { Plugin, PluginKey, TextSelection } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';
import type { EditorState, Transaction } from '@tiptap/pm/state';
import type { EditorView } from '@tiptap/pm/view';

const mermaidPreviewPluginKey = new PluginKey('mermaidPreview');

type MermaidPreviewPluginState = {
	decorations: DecorationSet;
	hiddenByKey: Record<string, boolean>;
};

let mermaidModulePromise: Promise<typeof import('mermaid').default> | null = null;
let initializedMermaidTheme: 'light' | 'dark' | null = null;

function isMermaidLanguage(value: unknown): boolean {
	return typeof value === 'string' && value.trim().toLowerCase() === 'mermaid';
}

function hashString(value: string): string {
	let hash = 5381;
	for (let index = 0; index < value.length; index += 1) {
		hash = (hash * 33) ^ value.charCodeAt(index);
	}
	return (hash >>> 0).toString(36);
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

function applyReadableStyleTextColors(source: string): string {
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

function getMermaidModule() {
	if (!mermaidModulePromise) {
		mermaidModulePromise = import('mermaid').then((module) => module.default);
	}
	return mermaidModulePromise;
}

function createThemeVariables(darkMode: boolean) {
	return {
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
	};
}

function createPreviewElement(
	source: string,
	key: string,
	isSourceHidden: boolean,
	onToggleSource: () => void,
	onActivateSource: () => void,
	onDelete: () => void
): HTMLElement {
	const root = document.createElement('div');
	root.className = 'cw-mermaid-preview';
	root.dataset.mermaidPreviewKey = key;

	const header = document.createElement('div');
	header.className = 'cw-mermaid-preview__header';

	const title = document.createElement('span');
	title.className = 'cw-mermaid-preview__title';
	title.textContent = 'Mermaid';

	const toggleButton = document.createElement('button');
	toggleButton.className = 'cw-mermaid-preview__toggle';
	toggleButton.type = 'button';
	toggleButton.textContent = isSourceHidden ? '编辑源码' : '隐藏源码';
	toggleButton.setAttribute('aria-label', isSourceHidden ? '展开 Mermaid 源码进行编辑' : '隐藏 Mermaid 源码');
	toggleButton.addEventListener('click', (event) => {
		event.preventDefault();
		event.stopPropagation();
		onToggleSource();
	});

	const deleteButton = document.createElement('button');
	deleteButton.className = 'cw-mermaid-preview__delete';
	deleteButton.type = 'button';
	deleteButton.textContent = '删除';
	deleteButton.setAttribute('aria-label', '删除 Mermaid 图表');
	deleteButton.addEventListener('click', (event) => {
		event.preventDefault();
		event.stopPropagation();
		onDelete();
	});

	const actions = document.createElement('div');
	actions.className = 'cw-mermaid-preview__actions';
	actions.append(toggleButton, deleteButton);

	header.append(title, actions);

	const canvas = document.createElement('div');
	canvas.className = 'cw-mermaid-preview__canvas';
	canvas.setAttribute('aria-live', 'polite');
	canvas.setAttribute('role', 'button');
	canvas.tabIndex = 0;
	const activateSourceLabel = isSourceHidden ? '点击编辑 Mermaid 源码' : '点击定位 Mermaid 源码';
	canvas.title = activateSourceLabel;
	canvas.setAttribute('aria-label', activateSourceLabel);
	canvas.addEventListener('click', (event) => {
		event.preventDefault();
		onActivateSource();
	});
	canvas.addEventListener('keydown', (event) => {
		if (event.key !== 'Enter' && event.key !== ' ') {
			return;
		}
		event.preventDefault();
		onActivateSource();
	});

	root.append(header, canvas);

	void renderMermaid(source, canvas);
	return root;
}

async function renderMermaid(source: string, target: HTMLElement) {
	if (!source.trim()) {
		target.textContent = '';
		return;
	}

	target.classList.remove('cw-mermaid-preview__canvas--error');
	target.textContent = 'Rendering...';

	try {
		const mermaid = await getMermaidModule();
		const darkMode = isDarkMode();
		const theme = darkMode ? 'dark' : 'light';
		if (initializedMermaidTheme !== theme) {
			mermaid.initialize({
				startOnLoad: false,
				securityLevel: 'strict',
				theme: 'base',
				themeVariables: createThemeVariables(darkMode)
			});
			initializedMermaidTheme = theme;
		}
		const id = `cw-mermaid-${hashString(source)}-${Date.now().toString(36)}`;
		const result = await mermaid.render(id, applyReadableStyleTextColors(source));
		if (!target.isConnected) {
			return;
		}
		target.innerHTML = result.svg;
	} catch (error) {
		if (!target.isConnected) {
			return;
		}
		target.classList.add('cw-mermaid-preview__canvas--error');
		target.textContent = error instanceof Error ? error.message : 'Mermaid render failed';
	}
}

function toggleSourceVisibility(
	view: EditorView,
	key: string,
	sourcePosition: number,
	isSourceHidden: boolean
) {
	view.dispatch(view.state.tr.setMeta(mermaidPreviewPluginKey, { type: 'toggle-source', key }));

	if (isSourceHidden) {
		focusSource(view, sourcePosition);
	}
}

function focusSource(view: EditorView, sourcePosition: number) {
	const position = Math.min(sourcePosition, view.state.doc.content.size);
	const selection = TextSelection.near(view.state.doc.resolve(position));
	view.dispatch(view.state.tr.setSelection(selection).scrollIntoView());
	view.focus();
}

function activateSource(view: EditorView, key: string, sourcePosition: number, isSourceHidden: boolean) {
	if (isSourceHidden) {
		view.dispatch(view.state.tr.setMeta(mermaidPreviewPluginKey, { type: 'toggle-source', key }));
	}
	focusSource(view, sourcePosition);
}

function deleteSourceBlock(view: EditorView, from: number, to: number) {
	const end = Math.min(to, view.state.doc.content.size);
	if (from >= end) {
		return;
	}

	view.dispatch(view.state.tr.delete(from, end).scrollIntoView());
	view.focus();
}

function buildDecorations(state: EditorState, hiddenByKey: Record<string, boolean>) {
	const decorations: Decoration[] = [];
	const nextHiddenByKey: Record<string, boolean> = {};

	for (const block of findChildren(state.doc, (node) => node.type.name === 'codeBlock')) {
		const language = block.node.attrs.language;
		if (!isMermaidLanguage(language)) {
			continue;
		}

		const source = block.node.textContent;
		const key = `mermaid-${block.pos}`;
		const isSourceHidden = hiddenByKey[key] ?? true;
		nextHiddenByKey[key] = isSourceHidden;
		const sourcePosition = block.pos + 1;
		const sourceEndPosition = block.pos + block.node.nodeSize;

		if (isSourceHidden) {
			decorations.push(
				Decoration.node(block.pos, sourceEndPosition, {
					class: 'cw-mermaid-source--hidden'
				})
			);
		}

		decorations.push(
			Decoration.widget(
				sourceEndPosition,
				(view) =>
					createPreviewElement(
						source,
						key,
						isSourceHidden,
						() => toggleSourceVisibility(view, key, sourcePosition, isSourceHidden),
						() => activateSource(view, key, sourcePosition, isSourceHidden),
						() => deleteSourceBlock(view, block.pos, sourceEndPosition)
					),
				{
					key: `${key}-${isSourceHidden ? 'hidden' : 'visible'}`,
					side: 1,
					ignoreSelection: true,
					stopEvent: () => true
				}
			)
		);
	}

	return {
		decorations: DecorationSet.create(state.doc, decorations),
		hiddenByKey: nextHiddenByKey
	};
}

function getMermaidSources(state: EditorState): string[] {
	return findChildren(state.doc, (node) => node.type.name === 'codeBlock')
		.filter((block) => isMermaidLanguage(block.node.attrs.language))
		.map((block) => block.node.textContent);
}

function hasMermaidSourceChanges(oldState: EditorState, newState: EditorState): boolean {
	const previousSources = getMermaidSources(oldState);
	const nextSources = getMermaidSources(newState);
	if (previousSources.length !== nextSources.length) {
		return true;
	}
	return previousSources.some((source, index) => source !== nextSources[index]);
}

function mapHiddenByKey(
	hiddenByKey: Record<string, boolean>,
	transaction: Transaction
) {
	const nextHiddenByKey: Record<string, boolean> = {};
	for (const [key, isHidden] of Object.entries(hiddenByKey)) {
		if (!key.startsWith('mermaid-')) {
			nextHiddenByKey[key] = isHidden;
			continue;
		}
		const position = Number.parseInt(key.slice('mermaid-'.length), 10);
		if (!Number.isFinite(position)) {
			continue;
		}
		const mapped = transaction.mapping.mapResult(position, 1);
		if (mapped.deleted) {
			continue;
		}
		nextHiddenByKey[`mermaid-${mapped.pos}`] = isHidden;
	}
	return nextHiddenByKey;
}

export function createMermaidPreviewExtension() {
	return Extension.create({
		name: 'mermaidPreview',

		addProseMirrorPlugins() {
			return [
				new Plugin({
					key: mermaidPreviewPluginKey,
					state: {
						init: (_, state) => buildDecorations(state, {}),
						apply: (transaction, previous: MermaidPreviewPluginState, oldState, newState) => {
							const meta = transaction.getMeta(mermaidPreviewPluginKey);
							if (meta?.type === 'toggle-source' && typeof meta.key === 'string') {
								return buildDecorations(newState, {
									...previous.hiddenByKey,
									[meta.key]: !(previous.hiddenByKey[meta.key] ?? true)
								});
							}

							if (!transaction.docChanged) {
								return {
									decorations: previous.decorations.map(transaction.mapping, transaction.doc),
									hiddenByKey: previous.hiddenByKey
								};
							}
							const mappedHiddenByKey = mapHiddenByKey(previous.hiddenByKey, transaction);
							if (!hasMermaidSourceChanges(oldState, newState)) {
								return {
									decorations: previous.decorations.map(transaction.mapping, transaction.doc),
									hiddenByKey: mappedHiddenByKey
								};
							}
							return buildDecorations(newState, mappedHiddenByKey);
						}
					},
					props: {
						decorations(state) {
							return mermaidPreviewPluginKey.getState(state)?.decorations;
						},
						handleDOMEvents: {
							mousedown: (_view: EditorView, event: MouseEvent) => {
								const target = event.target;
								if (target instanceof Element && target.closest('.cw-mermaid-preview')) {
									event.preventDefault();
									return true;
								}
								return false;
							}
						}
					}
				})
			];
		}
	});
}
