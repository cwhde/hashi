import { describe, expect, it, vi } from 'vitest';

import { resolveRootPortMode } from './port-mode';

function stubWindow(port: string, runtimeConfig?: Window['__HASHI_RUNTIME_CONFIG__']) {
	vi.stubGlobal('window', {
		location: { port },
		__HASHI_RUNTIME_CONFIG__: runtimeConfig
	});
}

describe('resolveRootPortMode', () => {
	it('uses default public ports when runtime config is absent', () => {
		stubWindow('8081');
		expect(resolveRootPortMode()).toBe('public-dashboard');

		stubWindow('8082');
		expect(resolveRootPortMode()).toBe('public-status');

		vi.unstubAllGlobals();
	});

	it('uses configured public ports from runtime config', () => {
		const config = {
			ports: {
				publicDashboard: 9081,
				publicStatus: 9082
			}
		};

		stubWindow('9081', config);
		expect(resolveRootPortMode()).toBe('public-dashboard');

		stubWindow('9082', config);
		expect(resolveRootPortMode()).toBe('public-status');

		stubWindow('8081', config);
		expect(resolveRootPortMode()).toBe('admin');

		vi.unstubAllGlobals();
	});
});
