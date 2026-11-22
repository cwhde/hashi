import type { Middleware } from 'openapi-fetch';

/** What every page shows when a request never reached Hashi (bugs 174, 177): the server is down or the connection
 * dropped. Lower case without a full stop, like the server's own details. */
export const UNREACHABLE = 'Hashi could not be reached; check the connection and try again';

/** A request that never got an answer comes back as a 503 problem instead of throwing, so every page takes its usual
 * error path; before, a page waiting for its first data kept its loading placeholder forever. A request the page
 * cancelled itself still throws. */
export const unreachableMiddleware: Middleware = {
	onError({ error }) {
		if (error instanceof DOMException && error.name === 'AbortError') return error as Error;
		return new Response(
			JSON.stringify({
				type: 'about:blank',
				title: 'Unreachable',
				status: 503,
				detail: UNREACHABLE
			}),
			{ status: 503, headers: { 'Content-Type': 'application/problem+json' } }
		);
	}
};
