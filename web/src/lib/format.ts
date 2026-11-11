// Timestamps arrive as UTC (invariant 6); the browser renders them in its locale, in local time or in UTC as the
// General settings say (09 §2).
import { tz } from './timezone.svelte';

/** Applies the General settings' time display ('local' or 'utc'). */
export function setTimeDisplay(v: 'local' | 'utc') {
	tz.zone = v === 'utc' ? 'UTC' : undefined;
}

/** The time zone every formatter uses: undefined is the browser's own. Reading it in a template tracks changes. */
export const timeZone = () => tz.zone;

export function time(iso: string | undefined): string {
	if (!iso) return '—';
	return new Date(iso).toLocaleTimeString(undefined, {
		hour: '2-digit',
		minute: '2-digit',
		timeZone: tz.zone,
		timeZoneName: tz.zone ? 'short' : undefined
	});
}

export function dateTime(iso: string | undefined): string {
	if (!iso) return '—';
	const d = new Date(iso);
	const zone = tz.zone;
	const day = (x: Date) => x.toLocaleDateString('en-CA', { timeZone: zone });
	return day(new Date()) === day(d)
		? time(iso)
		: d.toLocaleString(undefined, {
				month: 'short',
				day: 'numeric',
				hour: '2-digit',
				minute: '2-digit',
				timeZone: zone,
				timeZoneName: zone ? 'short' : undefined
			});
}
