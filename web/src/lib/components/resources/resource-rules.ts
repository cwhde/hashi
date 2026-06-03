import type { Schemas } from '$lib/api/types';

export type ResourceRuleRequest = Schemas['ResourceRuleRequest'];

export const RESOURCE_RULE_ACTIONS = [
	{ value: 'bypass_auth', label: 'Bypass auth' },
	{ value: 'block_access', label: 'Block access' },
	{ value: 'pass_to_auth', label: 'Pass to auth' },
	{ value: 'require_adaptive_challenge', label: 'Adaptive challenge' }
] as const;

export const RESOURCE_RULE_MATCH_TYPES = [
	{ value: 'ip', label: 'IP' },
	{ value: 'cidr', label: 'CIDR' },
	{ value: 'path', label: 'Path' },
	{ value: 'country', label: 'Country' },
	{ value: 'region', label: 'Region' },
	{ value: 'asn', label: 'ASN' }
] as const;

export const DEFAULT_RULE_PRIORITY = 100;

export function createEmptyRule(): ResourceRuleRequest {
	return {
		enabled: true,
		priority: DEFAULT_RULE_PRIORITY,
		action: 'pass_to_auth',
		matchType: 'path',
		matchValue: '/'
	};
}

export function reorderRule(
	rules: ResourceRuleRequest[],
	index: number,
	direction: -1 | 1
): ResourceRuleRequest[] {
	const nextIndex = index + direction;
	if (index < 0 || nextIndex < 0 || index >= rules.length || nextIndex >= rules.length) {
		return rules;
	}

	const next = [...rules];
	const [item] = next.splice(index, 1);
	next.splice(nextIndex, 0, item);
	return next;
}

export function removeRule(rules: ResourceRuleRequest[], index: number): ResourceRuleRequest[] {
	if (index < 0 || index >= rules.length) {
		return rules;
	}

	return rules.filter((_, ruleIndex) => ruleIndex !== index);
}

export function normalizeRules(rules: ResourceRuleRequest[]): ResourceRuleRequest[] {
	return rules.map((rule) => ({
		...rule,
		priority: Number(rule.priority),
		action: rule.action?.trim() || 'pass_to_auth',
		matchType: rule.matchType?.trim() || 'path',
		matchValue: rule.matchValue?.trim() || '/'
	}));
}
