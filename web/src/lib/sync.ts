// Helpers for showing sync plans (01-platform.md §5) with PlanReview.
import type { Schemas } from '$lib/api/client';
import type { PlanItem } from '$lib/design/mock';

export type Plan = Schemas['SyncPlan'];
type Value = Schemas['SyncPlanItemValue'];

/** One RRSet as the plan shows it: every value, then the TTL ("zone" = the zone default). */
export function valueText(v?: Value): string | undefined {
	if (!v) return undefined;
	return `${v.values.join(', ')}  TTL ${v.ttl ?? 'zone'}`;
}

/** The plan's items in PlanReview's shape. */
export function reviewItems(p: Plan): PlanItem[] {
	return p.items.map((i) => ({
		action: i.action,
		subsystem: 'DNS',
		target: p.target_name,
		object: i.object,
		before: valueText(i.before),
		after: valueText(i.after),
		risk: i.risk,
		note: i.note || undefined
	}));
}

const triggers: Record<Plan['trigger'], string> = {
	save: 'Saved',
	reconcile: 'Background sync',
	manual: 'Sync now',
	probe: 'Write test',
	prune: 'Import table'
};

export const fmtTime = (iso?: string) =>
	iso ? new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : '';

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
