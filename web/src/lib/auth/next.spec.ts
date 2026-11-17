import { describe, expect, it } from 'vitest';
import { safeNext, signInHref } from './next.ts';

describe('signInHref', () => {
	it('keeps the tab in the address (bug 97)', () => {
		const href = signInHref({ pathname: '/security', search: '?tab=counters' });
		expect(href).toBe('/login?next=%2Fsecurity%3Ftab%3Dcounters');
		expect(safeNext(new URL(href, 'https://h.test').searchParams.get('next'))).toBe(
			'/security?tab=counters'
		);
	});
	it('says when the session ended', () => {
		expect(signInHref({ pathname: '/dns', search: '' }, true)).toBe('/login?ended=1&next=%2Fdns');
	});
});

describe('safeNext', () => {
	it('refuses other origins', () => {
		for (const bad of ['//evil.test', '/\\evil.test', 'https://evil.test', '', null])
			expect(safeNext(bad)).toBe('/activity');
	});
});
