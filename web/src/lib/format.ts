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

/** A country's flag before its code (bug 120): "🇩🇪 DE". Empty or unknown codes stay as they are. */
export function country(code: string | undefined): string {
	if (!code) return '—';
	const cc = code.toUpperCase();
	if (!/^[A-Z]{2}$/.test(cc)) return cc;
	return `${String.fromCodePoint(...[...cc].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65))} ${cc}`;
}
