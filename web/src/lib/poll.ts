/**
 * Runs fn every ms while the tab is shown. While it is hidden nothing runs, and fn runs once when the tab is shown
 * again, so a page left in a background tab does not poll the server around the clock. Returns the cleanup, for
 * onMount's return.
 */
export function every(ms: number, fn: () => void): () => void {
	const tick = () => {
		if (!document.hidden) fn();
	};
	const t = setInterval(tick, ms);
	document.addEventListener('visibilitychange', tick);
	return () => {
		clearInterval(t);
		document.removeEventListener('visibilitychange', tick);
	};
}
