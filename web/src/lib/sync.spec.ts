import { describe, expect, it } from 'vitest';
import { reviewItems, valueText, type Plan } from './sync';

// Bug 74: one plan item without values blanked the whole Pending changes page.
describe('plan review items', () => {
	it('shows an RRSet without values instead of throwing', () => {
		const v = { name: 'a', type: 'CNAME', values: null } as unknown as Parameters<
			typeof valueText
		>[0];
		expect(valueText(v)).toBe('  TTL zone');
	});

	it('names AdGuard plans and keeps their text values', () => {
		const p = {
			subsystem: 'adguard',
			target_name: 'home',
			items: [
				{
					action: 'delete',
					object: 'app.example.com',
					risk: 'high',
					note: '',
					before_text: 'app.example.com → 192.0.2.10'
				}
			]
		} as unknown as Plan;
		const item = reviewItems(p)[0]!;
		expect(item.subsystem).toBe('AdGuard');
		expect(item.before).toBe('app.example.com → 192.0.2.10');
	});
});
