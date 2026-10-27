import { describe, expect, it } from 'vitest';
import { contrast } from './contrast';

describe('contrast', () => {
	it('matches WCAG reference values', () => {
		expect(contrast('#000000', '#ffffff')).toBeCloseTo(21, 1);
		expect(contrast('#777777', '#ffffff')).toBeCloseTo(4.48, 1);
	});
	it('rejects malformed colours', () => {
		expect(contrast('blue', '#ffffff')).toBeNaN();
	});
	it('every built-in theme keeps body and muted text readable on its surface', () => {
		// [text, muted, surface] from 10-ui.md §2
		const themes = {
			graphite: ['#F2F5F7', '#9DA6B2', '#13161A'],
			linen: ['#2B251F', '#74695D', '#FAF7F1'],
			verdant: ['#EDF7F0', '#94AA9B', '#0A1A12'],
			violet: ['#F4F0FA', '#AA9FBC', '#110C1D']
		};
		for (const [text, muted, surface] of Object.values(themes)) {
			expect(contrast(text!, surface!)).toBeGreaterThanOrEqual(7);
			expect(contrast(muted!, surface!)).toBeGreaterThanOrEqual(4.5);
		}
	});
});
