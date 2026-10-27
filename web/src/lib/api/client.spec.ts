import { describe, expect, it, vi } from 'vitest';
import { api } from './client';

describe('api client', () => {
	it('decodes /api/version with the generated types', async () => {
		const fetchMock = vi.fn<(req: Request) => Promise<Response>>(
			async () =>
				new Response(JSON.stringify({ version: 'v0.0.1', commit: 'abc1234' }), {
					headers: { 'Content-Type': 'application/json' }
				})
		);
		const { data, error } = await api.GET('/api/version', {
			fetch: fetchMock,
			baseUrl: 'http://hashi.test'
		});
		expect(error).toBeUndefined();
		expect(data?.commit).toBe('abc1234');
		expect(fetchMock.mock.calls[0]?.[0].url).toBe('http://hashi.test/api/version');
	});
});
