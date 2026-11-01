import { api, type Schemas } from '$lib/api/client';
import type { IPSet } from '$lib/ipsets';

export type RuleSet = Schemas['RuleSet'];
export type RuleEntry = Schemas['RuleEntry'];
export type RuleEntryInput = Schemas['RuleEntryInput'];
export type FlatRule = Schemas['FlatRule'];
export type Provider = Schemas['SSOProvider'];
export type Action = 'none' | 'deny' | 'pass_authentication';

// What each action means, in the words the rule editor shows (06 §4.1). Pass CAPTCHA arrives with the abuse engine.
export const actionMeta: Record<
	Action | 'pass_captcha',
	{ label: string; tone: 'ok' | 'bad' | 'warn' | 'info'; help: string }
> = {
	none: {
		label: 'None',
		tone: 'ok',
		help: 'Let the request through; later sign-in rules are skipped'
	},
	deny: { label: 'Deny', tone: 'bad', help: 'Answer 403' },
	pass_authentication: {
		label: 'Pass sign-in',
		tone: 'info',
		help: 'Ask for a login; with allowed groups, only those get in'
	},
	pass_captcha: { label: 'Pass CAPTCHA', tone: 'warn', help: '' }
};
export const actions: Action[] = ['none', 'deny', 'pass_authentication'];

export const toInput = (e: RuleEntry): RuleEntryInput =>
	e.type === 'set'
		? { type: 'set', child_id: e.child_id, note: e.note }
		: {
				type: 'item',
				applies_to: e.applies_to,
				match_type: e.match_type,
				pattern: e.pattern,
				action: e.action,
				provider_id: e.provider_id,
				groups: e.groups,
				enabled: e.enabled,
				note: e.note
			};

export const emptyRule = (allId: string): RuleEntry => ({
	id: '',
	priority: 0,
	type: 'item',
	child_id: '',
	child_slug: '',
	applies_to: allId,
	match_type: '',
	pattern: '',
	action: 'none',
	provider_id: '',
	groups: [],
	origin: 'user',
	locked: false,
	fallback: false,
	enabled: true,
	note: ''
});

export const allSetId = (sets: IPSet[]) => sets.find((s) => s.system_key === 'all')?.id ?? '';

export async function listRuleSets(): Promise<RuleSet[]> {
	const r = await api.GET('/api/rule-sets');
	if (r.error) throw r.error;
	return r.data ?? [];
}

export async function listProviders(): Promise<Provider[]> {
	const r = await api.GET('/api/security/sso/providers');
	return r.data?.providers ?? [];
}
