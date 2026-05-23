import { skillMarkdownResponse } from '$lib/skill/skill-response';
import type { RequestHandler } from './$types';

export const prerender = false;

export const GET: RequestHandler = ({ request, url }) => {
	return skillMarkdownResponse(request, url.origin);
};
