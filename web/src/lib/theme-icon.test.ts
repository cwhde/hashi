import { expect, test } from 'vitest';
import { markIcon } from './theme.svelte.ts';

// Bug 159: the tab icon is drawn in the theme's colours.
test('the mark takes the colours it is given', () => {
	const svg = decodeURIComponent(markIcon('#e6cf62', '#120c1e').replace('data:image/svg+xml,', ''));
	expect(svg).toContain('fill="#e6cf62"');
	expect(svg).toContain('fill="#120c1e"');
	expect(svg).toContain('橋');
});
