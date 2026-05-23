import { json, type RequestHandler } from '@sveltejs/kit';
import type { JSONContent } from '@tiptap/core';
import { env } from '$env/dynamic/private';
import { markdownToTiptapJSON, tiptapJSONToMarkdown } from '$lib/markdown/tiptapMarkdown';

const DEFAULT_MAX_BYTES = 2 * 1024 * 1024;

type ConvertRequest =
	| {
			direction: 'markdown-to-json';
			markdown: string;
	  }
	| {
			direction: 'json-to-markdown';
			contentJson: JSONContent;
	  };

function errorResponse(status: number, code: string, message: string) {
	return json({ code, message }, { status });
}

function configuredToken(): string {
	return (env.MARKDOWN_CONVERTER_TOKEN ?? '').trim();
}

function maxRequestBytes(): number {
	const raw = Number.parseInt((env.MARKDOWN_CONVERTER_MAX_BYTES ?? '').trim(), 10);
	return Number.isFinite(raw) && raw > 0 ? raw : DEFAULT_MAX_BYTES;
}

function bearerToken(request: Request): string {
	const header = request.headers.get('authorization') ?? '';
	const match = header.match(/^Bearer\s+(.+)$/i);
	return match?.[1]?.trim() ?? '';
}

function jsonSize(value: unknown): number {
	return new TextEncoder().encode(JSON.stringify(value)).byteLength;
}

export const POST: RequestHandler = async ({ request }) => {
	const token = configuredToken();
	if (!token) {
		return errorResponse(
			503,
			'markdown_converter_not_configured',
			'Markdown converter token is not configured.'
		);
	}
	if (bearerToken(request) !== token) {
		return errorResponse(401, 'unauthorized', 'Invalid markdown converter token.');
	}

	const contentLength = Number.parseInt(request.headers.get('content-length') ?? '', 10);
	if (Number.isFinite(contentLength) && contentLength > maxRequestBytes()) {
		return errorResponse(413, 'payload_too_large', 'Markdown conversion request is too large.');
	}

	let body: ConvertRequest;
	try {
		body = (await request.json()) as ConvertRequest;
	} catch {
		return errorResponse(400, 'invalid_json', 'Request body must be valid JSON.');
	}

	if (jsonSize(body) > maxRequestBytes()) {
		return errorResponse(413, 'payload_too_large', 'Markdown conversion request is too large.');
	}

	try {
		if (body.direction === 'markdown-to-json') {
			if (typeof body.markdown !== 'string') {
				return errorResponse(400, 'invalid_markdown', 'markdown must be a string.');
			}
			return json({ contentJson: markdownToTiptapJSON(body.markdown) });
		}

		if (body.direction === 'json-to-markdown') {
			if (!body.contentJson || typeof body.contentJson !== 'object') {
				return errorResponse(400, 'invalid_content_json', 'contentJson must be an object.');
			}
			return json({ markdown: tiptapJSONToMarkdown(body.contentJson) });
		}

		return errorResponse(400, 'invalid_direction', 'Unsupported markdown conversion direction.');
	} catch {
		return errorResponse(
			422,
			'markdown_conversion_failed',
			'Markdown conversion failed. The document was not changed.'
		);
	}
};
