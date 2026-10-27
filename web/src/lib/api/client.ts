// Typed API client generated from api/openapi.yaml (`pnpm generate:api`). Never hand-write request types.
import createClient, { type Middleware } from 'openapi-fetch';
import type { components, paths } from './schema.gen';

export type Schemas = components['schemas'];
export type Problem = Schemas['Problem'];

export const api = createClient<paths>({ baseUrl: '/', credentials: 'same-origin' });

// Unsafe requests carry the session-bound CSRF token from GET /api/auth/session (02-setup-auth.md §4).
let csrfToken = '';
export function setCSRF(token: string | undefined) {
	csrfToken = token ?? '';
}

export const csrfMiddleware: Middleware = {
	onRequest({ request }) {
		if (csrfToken && !['GET', 'HEAD', 'OPTIONS'].includes(request.method)) {
			request.headers.set('X-Hashi-CSRF', csrfToken);
		}
		return request;
	}
};
api.use(csrfMiddleware);

/** The problem's own message, or a plain fallback that still says nothing changed. */
export function problemText(e: unknown): string {
	const p = e as Partial<Problem> | undefined;
	if (p && typeof p === 'object' && (p.detail || p.title)) return p.detail || p.title || '';
	return 'Hashi did not answer as expected. Nothing was changed; reload and try again.';
}

export function isProblem(e: unknown, type: string): boolean {
	return !!e && typeof e === 'object' && (e as Partial<Problem>).type === type;
}
