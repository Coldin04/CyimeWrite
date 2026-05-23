import {
	buildSkillDocument,
	cacheHeaders,
	matchesETag,
	renderSkillMarkdown
} from '$lib/skill/cyime-workspace';

export function skillMarkdownResponse(request: Request, frontendOrigin: string): Response {
	const document = buildSkillDocument(frontendOrigin);
	if (matchesETag(request, document.etag)) {
		return new Response(null, {
			status: 304,
			headers: cacheHeaders(document.etag, 'text/markdown; charset=utf-8')
		});
	}

	return new Response(renderSkillMarkdown(document), {
		headers: cacheHeaders(document.etag, 'text/markdown; charset=utf-8')
	});
}
