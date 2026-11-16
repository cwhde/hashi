import { describe, expect, it } from 'vitest';
import { hrefWithTab, tabFrom } from './url-tab';

const tabs = ['routing', 'access', 'waf'] as const;
const at = (s: string) => new URL(s, 'https://hashi.example.com');

describe('url tab', () => {
	it('reads the tab, falling back to the first', () => {
		expect(tabFrom(at('/services/1?tab=waf'), tabs)).toBe('waf');
		expect(tabFrom(at('/services/1'), tabs)).toBe('routing');
		expect(tabFrom(at('/services/1?tab=nope'), tabs)).toBe('routing');
	});
	it('writes the tab and keeps everything else', () => {
		expect(hrefWithTab(at('/settings?category=pulse#x'), tabs, 'access')).toBe(
			'/settings?category=pulse&tab=access#x'
		);
		expect(hrefWithTab(at('/services/1?tab=waf&q=a'), tabs, 'routing')).toBe('/services/1?q=a');
	});
});
