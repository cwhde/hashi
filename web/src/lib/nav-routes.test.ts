import { describe, expect, it } from 'vitest';
import { navRoutes } from './nav-routes.js';

describe('navRoutes', () => {
	it('includes Overview as the first item', () => {
		expect(navRoutes[0]?.label).toBe('Overview');
		expect(navRoutes[0]?.href).toBe('/');
	});

	it('has unique hrefs', () => {
		const hrefs = navRoutes.map((item) => item.href);
		expect(new Set(hrefs).size).toBe(hrefs.length);
	});

	it('matches spec §21 section count', () => {
		expect(navRoutes.length).toBe(13);
	});
});
