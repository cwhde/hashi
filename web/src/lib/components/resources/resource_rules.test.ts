import { describe, expect, it } from 'vitest';
import { createEmptyRule, normalizeRules, removeRule, reorderRule } from './resource-rules';

describe('resource_rules', () => {
	it('creates a default pass-to-auth path rule', () => {
		const rule = createEmptyRule();

		expect(rule.enabled).toBe(true);
		expect(rule.priority).toBe(100);
		expect(rule.action).toBe('pass_to_auth');
		expect(rule.matchType).toBe('path');
		expect(rule.matchValue).toBe('/');
	});

	it('reorders rules in requested direction', () => {
		const rules = [
			{ ...createEmptyRule(), matchValue: '/a' },
			{ ...createEmptyRule(), matchValue: '/b' }
		];

		const moved = reorderRule(rules, 0, 1);
		expect(moved[0]?.matchValue).toBe('/b');
		expect(moved[1]?.matchValue).toBe('/a');
	});

	it('removes a rule by index', () => {
		const rules = [
			{ ...createEmptyRule(), matchValue: '/a' },
			{ ...createEmptyRule(), matchValue: '/b' }
		];

		const reduced = removeRule(rules, 0);
		expect(reduced).toHaveLength(1);
		expect(reduced[0]?.matchValue).toBe('/b');
	});

	it('normalizes numeric priority and trims canonical fields', () => {
		const normalized = normalizeRules([
			{
				enabled: true,
				priority: '105',
				action: ' block_access ',
				matchType: ' cidr ',
				matchValue: ' 203.0.113.0/24 '
			}
		]);

		expect(normalized[0]?.priority).toBe(105);
		expect(normalized[0]?.action).toBe('block_access');
		expect(normalized[0]?.matchType).toBe('cidr');
		expect(normalized[0]?.matchValue).toBe('203.0.113.0/24');
	});
});
