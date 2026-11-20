import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { localSearch, matches, pageTabs, ranked, settingCategories, settings } from './local.ts';

const src = (path: string) => readFileSync(new URL(path, import.meta.url), 'utf8');
const component = (name: string) => src(`../components/hashi/${name}.svelte`);
// Svelte escapes nothing in these labels, but the markup may break a label over two lines.
const flat = (s: string) => s.replace(/\s+/g, ' ');

describe('the search lists stay true to the pages (bug 4)', () => {
	it('every setting is shown by its component, under a category that exists', () => {
		const page = src('../../routes/(app)/settings/+page.svelte');
		for (const s of settings) {
			expect(flat(component(s.component)), `${s.label} in ${s.component}`).toContain(s.label);
			expect(page, s.category).toContain(`id: '${s.category}'`);
			expect(settingCategories[s.category]).toBeTruthy();
		}
	});

	it('every tab is on its page', () => {
		for (const p of pageTabs) {
			const page = flat(src(`../../routes/(app)${p.href}/+page.svelte`));
			for (const t of p.tabs) {
				expect(page, `${p.href} ${t.value}`).toContain(`value="${t.value}"`);
				expect(page, `${p.href} ${t.label}`).toContain(t.label);
			}
		}
	});
});

describe('local search', () => {
	const pages = [
		{ label: 'Security', href: '/security' },
		{ label: 'Services', href: '/services' }
	];

	it('matches every word, in any order', () => {
		expect(matches('limit request', 'Request limit')).toBe(true);
		expect(matches('limit reply', 'Request limit')).toBe(false);
		expect(matches('  ', 'anything')).toBe(false);
	});

	it('ranks an exact title, then a prefix', () => {
		const got = ranked('vault', [
			{ title: 'Unlock vault' },
			{ title: 'Vault mode' },
			{ title: 'Vault' }
		]);
		expect(got.map((h) => h.title)).toEqual(['Vault', 'Vault mode', 'Unlock vault']);
	});

	it('finds a setting by a word it does not show, and opens Settings at it', () => {
		const g = localSearch('rate', pages).find((x) => x.label === 'Settings');
		expect(g?.hits[0]).toMatchObject({
			title: 'Request limit',
			href: '/settings?category=edge&find=Request%20limit'
		});
	});

	it('finds a tab and a page', () => {
		const groups = localSearch('counters', pages);
		expect(groups.find((x) => x.label === 'Tabs')?.hits[0]?.href).toBe('/security?tab=counters');
		expect(localSearch('secur', pages)[0]).toMatchObject({
			label: 'Pages',
			hits: [{ title: 'Security', href: '/security' }]
		});
	});

	it('finds a settings category by its name', () => {
		const g = localSearch('edge protection', pages).find((x) => x.label === 'Settings');
		expect(g?.hits[0]).toMatchObject({ title: 'Edge protection', href: '/settings?category=edge' });
	});
});
