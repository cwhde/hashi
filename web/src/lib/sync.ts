// Helpers for showing sync plans (01-platform.md §5) with PlanReview.
import type { Schemas } from '#lib/api/client.ts';
import type { PlanItem } from '#lib/design/mock.ts';
import { dateTime } from '#lib/format.ts';

export type Plan = Schemas['SyncPlan'];
type Value = Schemas['SyncPlanItemValue'];

/** One RRSet as the plan shows it: every value, then the TTL ("zone" = the zone default). A malformed item must not
 * stop the page from rendering every other plan (bug 74), so missing values show as none. */
export function valueText(v?: Value | null): string | undefined {
	if (!v) return undefined;
	return `${(v.values ?? []).join(', ')}  TTL ${v.ttl ?? 'zone'}`;
}

/** The plan's items in PlanReview's shape. */
const subsystems: Record<Plan['subsystem'], PlanItem['subsystem']> = {
	'dns-records': 'DNS',
	'dns-record-deletes': 'DNS',
	'dns-prune': 'DNS',
	'dns-probe': 'DNS',
	traefik: 'Traefik',
	certificates: 'Certificates',
	firewall: 'Firewall',
	scripts: 'Scripts',
	adguard: 'AdGuard'
};

export function reviewItems(p: Plan): PlanItem[] {
	return p.items.map((i) => ({
		action: i.action,
		subsystem: subsystems[p.subsystem],
		target: p.target_name,
		object: i.object,
		before: valueText(i.before) ?? i.before_text,
		after: valueText(i.after) ?? i.after_text,
		risk: i.risk,
		note: i.note || undefined,
		content:
			i.before_content !== undefined || i.after_content !== undefined
				? { before: i.before_content ?? '', after: i.after_content ?? '' }
				: undefined
	}));
}

const triggers: Record<Plan['trigger'], string> = {
	save: 'Saved',
	reconcile: 'Background sync',
	manual: 'Sync now',
	probe: 'Write test',
	prune: 'Import table'
};

/** One date format app-wide (bug 101): the shared `dateTime`. */
export const fmtTime = (iso?: string) => (iso ? dateTime(iso) : '');

/** "Saved · 28 Oct 2026, 11:04" */
export const triggerText = (p: Plan) => `${triggers[p.trigger]} · ${fmtTime(p.created_at)}`;

/** Plain-language result of a finished plan. */
export function outcomeText(p: Plan): string {
	switch (p.status) {
		case 'applied':
			return p.result ? `Applied: ${p.result}` : 'Applied';
		case 'failed':
			return `Failed: ${p.error}`;
		case 'discarded':
			return 'Discarded';
		case 'superseded':
			return p.result ? `Replaced: ${p.result}` : 'Replaced by a newer plan';
		default:
			return p.status;
	}
}

/**
 * What approving a high-risk plan does, in plain lines (bug 35): the reasons the plan is high risk, and for the firewall
 * what Hashi checks afterwards and that the host puts its old rules back by itself when a check fails (05 §7).
 * Empty for a low-risk plan.
 */
export function stakes(p: Plan): string[] {
	if (p.risk !== 'high') return [];
	const reasons = [
		...new Set(
			p.items
				.filter((i) => i.risk === 'high')
				.flatMap((i) => (i.note ?? '').split('; '))
				.map((n) => n.trim())
				.filter(Boolean)
		)
	].map((n) => n.charAt(0).toUpperCase() + n.slice(1) + '.');
	if (p.subsystem !== 'firewall') return reasons;
	return [
		...reasons,
		`Afterwards Hashi logs in to ${p.target_name} over a new SSH connection and checks that NetBird still reaches its management server.`,
		'If either check fails, the host puts its previous rules back by itself after 3 minutes.'
	];
}
