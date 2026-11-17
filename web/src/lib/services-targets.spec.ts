import { describe, expect, it } from 'vitest';
import { parseTargets, targetText, upstreamOf, upstreamTarget } from './services.ts';

const sets = [{ id: '01HSETID', slug: 'office' }];

describe('route target text', () => {
	it('round-trips sets by slug and weights (bug 63)', () => {
		const ts = [
			{ address: '', set_id: '01HSETID', port: 443 },
			{ address: '2001:db8::5', port: 8080, weight: 3 },
			{ address: '192.0.2.5', port: 80 }
		];
		const text = ts.map((t) => targetText(t, sets)).join(', ');
		expect(text).toBe('@office:443, [2001:db8::5]:8080*3, 192.0.2.5:80');
		expect(parseTargets(text, sets)).toEqual({ targets: ts });
	});
});

describe('upstream choice', () => {
	it('maps an agent target both ways (bug 17)', () => {
		const u = upstreamOf({ address: '', agent_id: 'A1', port: 80 });
		expect(u.mode).toBe('agent');
		expect(upstreamTarget(u, 80)).toEqual({ address: '', agent_id: 'A1', port: 80 });
	});
});
