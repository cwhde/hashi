import { describe, expect, it, vi } from 'vitest';

import { resolveApiBaseUrl } from './base-url';

describe('resolveApiBaseUrl', () => {
	it.each(['8080', '8081', '8082'])('keeps API calls on the current origin for port %s', (port) => {
		vi.stubGlobal('window', {
			location: {
				protocol: 'https:',
				hostname: 'hashi.example.com',
				port
			}
		});

		expect(resolveApiBaseUrl()).toBe('');
		vi.unstubAllGlobals();
	});
});
