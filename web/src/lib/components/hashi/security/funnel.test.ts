import { expect, test } from 'vitest';
import { challengeFunnel } from './funnel.ts';

// Bug 157: bans also come from meters and blocklists, so there can be more bans than challenges. Every bar is a share of
// the largest count, so none runs past its track.
test('no bar is wider than its track', () => {
	const steps = challengeFunnel({ challenged: 29, solved: 0, banned: 34 });
	expect(steps.map((s) => [s.k, s.n])).toEqual([
		['Challenged', 29],
		['Solved', 0],
		['Not solved yet', 0],
		['Banned', 34]
	]);
	for (const s of steps) expect(s.width).toBeLessThanOrEqual(100);
	expect(steps.find((s) => s.k === 'Banned')?.width).toBe(100);
});

test('nothing counted draws empty bars', () => {
	for (const s of challengeFunnel({ challenged: 0, solved: 0, banned: 0 })) expect(s.width).toBe(0);
});
