// Plain names for the abuse engine's sources and event types (06 §3, §6.1), shared by the Security tabs.
const sources: Record<string, string> = {
	l7_rate: 'Request limit',
	l7_waf: 'WAF',
	l7_challenge_failed: 'Ignored challenge',
	l4_meter: 'Kernel meter',
	manual: 'By hand'
};

export const sourceLabel = (s: string) => sources[s] ?? (s || '—');

const events: Record<string, string> = {
	challenge_entered: 'Challenged',
	challenge_solved: 'Solved the challenge',
	challenge_skipped_allowed: 'Not challenged: allowed address',
	ban_created: 'Banned',
	ban_applied: 'Ban reached a firewall host',
	ban_apply_failed: 'Ban could not be applied on a host',
	ban_removed_host: 'Ban taken off a firewall host',
	ban_expired: 'Ban ended',
	challenge_expired: 'Challenge ended',
	ban_skipped_allowed: 'Not banned: allowed address',
	waf_match: 'Refused by the WAF',
	dynamic_removed: 'Removed by hand',
	counters_edited: 'Counters edited',
	counters_reset: 'Counters reset'
};

export const eventLabel = (t: string) => events[t] ?? t;
