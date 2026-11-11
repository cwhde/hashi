// Timestamps arrive as UTC (invariant 6); the browser renders them in its locale, in local time or in UTC as the
// General settings say (09 §2).
let zone: 'UTC' | undefined;

/** Applies the General settings' time display ('local' or 'utc'). */
export function setTimeDisplay(v: 'local' | 'utc') {
	zone = v === 'utc' ? 'UTC' : undefined;
}

/** The time zone every formatter uses: undefined is the browser's own. */
export const timeZone = () => zone;

export function time(iso: string | undefined): string {
	if (!iso) return '—';
	return new Date(iso).toLocaleTimeString(undefined, {
		hour: '2-digit',
		minute: '2-digit',
		timeZone: zone,
		timeZoneName: zone ? 'short' : undefined
	});
}

export function dateTime(iso: string | undefined): string {
	if (!iso) return '—';
	const d = new Date(iso);
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
