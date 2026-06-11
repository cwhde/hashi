import { beforeEach, describe, expect, it, vi } from 'vitest';

describe('API client undocumented POST requests', () => {
	beforeEach(() => {
		vi.resetModules();
		vi.restoreAllMocks();
	});

	it('returns JSON from CSRF-exempt passkey login requests', async () => {
		const fetchMock = vi.fn().mockResolvedValue(
			new Response(JSON.stringify({ options: { challenge: 'abc' }, challengeSessionId: 'session-1' }), {
				status: 200,
				headers: { 'Content-Type': 'application/json' }
			})
		);
		vi.stubGlobal('fetch', fetchMock);
		const { api } = await import('./client');

		await expect(api.passkeyLoginBegin()).resolves.toMatchObject({ challengeSessionId: 'session-1' });
		expect(fetchMock).toHaveBeenCalledTimes(1);
		expect(fetchMock).toHaveBeenCalledWith(
			'/api/auth/passkeys/login/begin',
			expect.objectContaining({ method: 'POST', credentials: 'include' })
		);
	});

	it('interpolates protected paths and attaches the CSRF token', async () => {
		const fetchMock = vi
			.fn()
			.mockResolvedValueOnce(
				new Response(JSON.stringify({ token: 'csrf-1' }), {
					status: 200,
					headers: { 'Content-Type': 'application/json' }
				})
			)
			.mockResolvedValueOnce(
				new Response(JSON.stringify({ valid: true }), {
					status: 200,
					headers: { 'Content-Type': 'application/json' }
				})
			);
		vi.stubGlobal('fetch', fetchMock);
		const { api } = await import('./client');

		await expect(api.validateConnection('server/a')).resolves.toMatchObject({ valid: true });
		const [requestPath, requestInit] = fetchMock.mock.calls[1] as [string, RequestInit];
		expect(requestPath).toBe('/api/connections/server%2Fa/validate');
		expect(new Headers(requestInit.headers).get('X-CSRF-TOKEN')).toBe('csrf-1');
	});
});
