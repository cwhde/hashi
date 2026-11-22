import { expect, test } from 'vitest';
import { nearest } from './chart-sync.svelte.ts';

test('nearest point to a shared moment', () => {
	const t = [0, 60, 120, 180];
	expect(nearest(t, 0)).toBe(0);
	expect(nearest(t, 100)).toBe(2);
	expect(nearest(t, 175)).toBe(3);
	// A series with gaps (the gap chart) still finds its closest point inside its range…
	expect(nearest([0, 180], 100)).toBe(1);
	// …but a moment well outside a series shows nothing there.
	expect(nearest(t, 400)).toBeNull();
	expect(nearest(t, -100)).toBeNull();
	expect(nearest([], 5)).toBeNull();
	expect(nearest([50], 5000)).toBe(0);
});
