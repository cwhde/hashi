import { expect, test } from 'vitest';
import createClient from 'openapi-fetch';
import { UNREACHABLE, unreachableMiddleware } from './unreachable.ts';

type Paths = {
	'/x': { get: { responses: { 200: { content: { 'application/json': { ok: boolean } } } } } };
};

test('a request that never got an answer is a 503 problem, not a throw (bugs 174, 177)', async () => {
	const c = createClient<Paths>({
		baseUrl: 'http://localhost',
		fetch: () => Promise.reject(new TypeError('Failed to fetch'))
	});
	c.use(unreachableMiddleware);
	const r = await c.GET('/x');
	expect(r.data).toBeUndefined();
	expect(r.response.status).toBe(503);
	expect(r.error).toMatchObject({ detail: UNREACHABLE });
});

test('a request the page cancelled still throws', async () => {
	const c = createClient<Paths>({
		baseUrl: 'http://localhost',
		fetch: () => Promise.reject(new DOMException('aborted', 'AbortError'))
	});
	c.use(unreachableMiddleware);
	await expect(c.GET('/x')).rejects.toThrow('aborted');
});
