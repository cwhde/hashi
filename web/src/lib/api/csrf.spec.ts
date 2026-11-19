import { describe, expect, it, vi } from 'vitest';
import { csrfRetry } from './csrf.ts';

const refused = () =>
	new Response(JSON.stringify({ type: '/problems/csrf', status: 403 }), {
		status: 403,
		headers: { 'content-type': 'application/problem+json' }
	});

// Bug 98: a sign-in in another tab replaces the cookie, so this tab's token is from the old session.
describe('csrfRetry', () => {
	it('fetches the session again and sends the request once more with the new token', async () => {
		let token = 'old';
		const sent: string[] = [];
		const fetchFn = vi.fn(async (r: Request) => {
			sent.push(`${r.method} ${r.headers.get('X-Hashi-CSRF')} ${await r.text()}`);
			return new Response('{}', { status: 200 });
		});
		const m = csrfRetry({
			token: () => token,
			refresh: async () => {
				token = 'new';
			},
			fetch: fetchFn
		});
		const req = new Request('https://h.test/api/x', { method: 'POST', body: '{"a":1}' });
		const sentReq = (await m.onRequest!({ request: req } as never)) as Request;
		const res = await m.onResponse!({ request: sentReq, response: refused() } as never);
		expect(res?.status).toBe(200);
		expect(sent).toEqual(['POST new {"a":1}']);
	});

	it('retries once only, and leaves other refusals alone', async () => {
		const fetchFn = vi.fn(async () => refused());
		const m = csrfRetry({ token: () => 't', refresh: async () => {}, fetch: fetchFn });
		const req = (await m.onRequest!({
			request: new Request('https://h.test/api/x', { method: 'DELETE' })
		} as never)) as Request;
		const res = await m.onResponse!({ request: req, response: refused() } as never);
		expect(res?.status).toBe(403);
		expect(fetchFn).toHaveBeenCalledTimes(1);

		const other = new Response(JSON.stringify({ type: '/problems/forbidden' }), { status: 403 });
		const get = (await m.onRequest!({
			request: new Request('https://h.test/api/x', { method: 'POST' })
		} as never)) as Request;
		expect(await m.onResponse!({ request: get, response: other } as never)).toBeUndefined();
	});
});
