import type { Middleware } from 'openapi-fetch';

const unsafe = (m: string) => !['GET', 'HEAD', 'OPTIONS'].includes(m);

/**
 * Sends the session-bound CSRF token on unsafe requests (02-setup-auth.md §4). When the server refuses the token (a
 * sign-in in another tab replaced the cookie under this page, bug 98), it fetches the session again and sends the
 * request once more with the new token. A second refusal is returned as it is.
 */
export function csrfRetry(deps: {
	token: () => string;
	refresh: () => Promise<void>;
	fetch: (r: Request) => Promise<Response>;
}): Middleware {
	// A copy of each unsafe request taken before its body is read, so it can be sent again.
	const copies = new WeakMap<Request, Request>();
	return {
		onRequest({ request }) {
			if (!unsafe(request.method)) return request;
			const t = deps.token();
			if (t) request.headers.set('X-Hashi-CSRF', t);
			copies.set(request, request.clone());
			return request;
		},
		async onResponse({ request, response }) {
			if (response.status !== 403) return;
			const copy = copies.get(request);
			if (!copy) return;
			const body = await response
				.clone()
				.json()
				.catch(() => null);
			if (body?.type !== '/problems/csrf') return;
			copies.delete(request);
			await deps.refresh();
			copy.headers.set('X-Hashi-CSRF', deps.token());
			return deps.fetch(copy);
		}
	};
}
