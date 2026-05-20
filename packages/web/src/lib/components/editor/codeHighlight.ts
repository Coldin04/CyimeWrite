import { CodeBlockLowlight } from '@tiptap/extension-code-block-lowlight';
import { common, createLowlight } from 'lowlight';

export const codeBlockLanguageOptions = [
	{ value: '', label: 'Auto' },
	{ value: 'bash', label: 'Bash' },
	{ value: 'mermaid', label: 'Mermaid' },
	{ value: 'c', label: 'C' },
	{ value: 'cpp', label: 'C++' }
] as const;

export function normalizeCodeBlockLanguage(value: string): string {
	return value
		.trim()
		.toLowerCase()
		.replace(/[^a-z0-9_+#.-]/g, '')
		.slice(0, 32);
}

const lowlight = createLowlight(common);

lowlight.registerAlias({
	bash: ['sh', 'shell'],
	c: ['h'],
	cpp: ['cc', 'cxx', 'hpp']
});

export function createCodeBlockLowlightExtension() {
	return CodeBlockLowlight.configure({
		lowlight,
		languageClassPrefix: 'language-',
		defaultLanguage: null,
		HTMLAttributes: {
			class: 'cw-code-block-highlighted'
		}
	});
}
