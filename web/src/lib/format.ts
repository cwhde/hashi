// Timestamps arrive as UTC (invariant 6); the browser renders them in its locale.
export function time(iso: string | undefined): string {
	if (!iso) return '—';
	return new Date(iso).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}

export function dateTime(iso: string | undefined): string {
	if (!iso) return '—';
	const d = new Date(iso);
	const today = new Date().toDateString() === d.toDateString();
	return today
		? time(iso)
		: d.toLocaleString(undefined, {
				month: 'short',
				day: 'numeric',
				hour: '2-digit',
				minute: '2-digit'
			});
}
