// Typed API client generated from api/openapi.yaml (`pnpm generate:api`). Never hand-write request types.
import createClient, { type Middleware } from 'openapi-fetch';
import type { components, paths } from './schema.gen';
import { signInHref } from '#lib/auth/next.ts';
import { csrfRetry } from './csrf.ts';
import { unreachableMiddleware } from './unreachable.ts';

export type Schemas = components['schemas'];
export type Problem = Schemas['Problem'];

export const api = createClient<paths>({ baseUrl: '/', credentials: 'same-origin' });

// Unsafe requests carry the session-bound CSRF token from GET /api/auth/session (02-setup-auth.md §4).
let csrfToken = '';
export function setCSRF(token: string | undefined) {
	csrfToken = token ?? '';
}

// A sign-in in another tab replaces the session cookie, so the token this page holds is refused; csrfRetry fetches
// the session again and sends the request once more (bug 98).
export const csrfMiddleware: Middleware = csrfRetry({
	token: () => csrfToken,
	refresh: async () => {
		const r = await fetch('/api/auth/session', { credentials: 'same-origin' });
		if (r.ok) setCSRF((await r.json()).csrf_token);
	},
	fetch: (r) => fetch(r)
});
// A session can end at any time (idle, IP change, revoked, its passkey deleted). Admin pages then go to sign-in
// instead of showing a dead error; pages that work without a session handle 401 themselves.
const publicPaths = ['/login', '/setup'];
export const sessionEndedMiddleware: Middleware = {
	onResponse({ response }) {
		if (response.status !== 401 || typeof location === 'undefined') return;
		if (publicPaths.some((p) => location.pathname.startsWith(p))) return;
		location.assign(signInHref(location, true));
	}
};

api.use(csrfMiddleware, sessionEndedMiddleware, unreachableMiddleware);

/** The problem's own message, or a plain fallback that still says nothing changed. */
export function problemText(e: unknown): string {
	// A passkey prompt cancelled during a reauth retry (withReauth) is the browser's error, not Hashi's (R4-004).
	if (e instanceof DOMException && (e.name === 'NotAllowedError' || e.name === 'AbortError')) {
		return 'The passkey prompt was closed or timed out. Nothing was changed; try again.';
	}
	const p = e as Partial<Problem> | undefined;
	if (p && typeof p === 'object' && (p.detail || p.title)) return p.detail || p.title || '';
	return 'Hashi did not answer as expected. Nothing was changed; reload and try again.';
}

export function isProblem(e: unknown, type: string): boolean {
	return !!e && typeof e === 'object' && (e as Partial<Problem>).type === type;
}
