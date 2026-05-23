import {
	buildOpenAPISpec,
	buildSkillDocument,
	cacheHeaders,
	matchesETag
} from '$lib/skill/cyime-workspace';
import type { RequestHandler } from './$types';

export const prerender = false;

export const GET: RequestHandler = ({ request, url }) => {
	const document = buildSkillDocument(url.origin);
	if (matchesETag(request, document.etag)) {
		return new Response(null, {
			status: 304,
			headers: cacheHeaders(document.etag, 'application/json; charset=utf-8')
		});
	}

	return new Response(JSON.stringify(buildOpenAPISpec(document)), {
		headers: cacheHeaders(document.etag, 'application/json; charset=utf-8')
	});
};
