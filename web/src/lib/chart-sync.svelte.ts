// One pointer for several charts over the same time range: pointing at one chart shows the same moment in the others,
// so a dip in one can be read against the rest. Charts share a time (ms), not a point number: their series may have
// different points (the endpoint page's gap chart only has the rounds with both checks).

export class ChartSync {
	/** The moment pointed at, in ms since the epoch; null when no chart is pointed at. */
	at = $state<number | null>(null);
}

/** The index of the point closest to `t`, or null when `t` is outside the series by more than one step. */
export function nearest(times: number[], t: number): number | null {
	if (times.length === 0) return null;
	let best = 0;
	for (let i = 1; i < times.length; i++)
		if (Math.abs(times[i]! - t) < Math.abs(times[best]! - t)) best = i;
	const step =
		times.length > 1 ? (times.at(-1)! - times[0]!) / (times.length - 1) : Number.POSITIVE_INFINITY;
	return Math.abs(times[best]! - t) <= step ? best : null;
}
