import { describe, expect, it } from 'vitest';
import { navItems } from './nav.js';

describe('navItems', () => {
	it('includes Overview as the first item', () => {
		expect(navItems[0]?.label).toBe('Overview');
		expect(navItems[0]?.href).toBe('/');
	});

	it('has unique hrefs', () => {
		const hrefs = navItems.map((item) => item.href);
		expect(new Set(hrefs).size).toBe(hrefs.length);
	});
});
