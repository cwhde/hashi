import type { Schemas } from '#lib/api/client.ts';
import type { Health } from '#lib/design/mock.ts';

export type Endpoint = Schemas['MonitorEndpoint'];

export const STRIP_SLOTS = 60;

/** The 60-minute strip: one bar per round, padded on the left so a new endpoint's bars still mean "recent". */
export function stripHealth(strip: { verdict: Health }[] | undefined): Health[] {
	const s = (strip ?? []).slice(-STRIP_SLOTS).map((x) => x.verdict);
	return [...Array<Health>(STRIP_SLOTS - s.length).fill('unknown'), ...s];
}

export const uptimeText = (v: number | undefined) =>
	v === undefined ? 'no data' : `${v >= 99.995 ? '100' : v.toFixed(2)}%`;

/** How an endpoint is checked, in words: the first check's type and target. */
export function checkSummary(e: Endpoint): string {
	const c = e.checks.find((x) => x.role === 'internal') ?? e.checks[0];
	if (!c) return 'no checks';
	// Self checks read Hashi's own state; their target is an internal key, not an address (bug 131).
	if (c.type === 'self')
		return c.target.startsWith('pulse:') ? 'heartbeat from the agent' : 'Hashi itself';
	return `${c.type} · ${c.target}`;
}

export const sourceText: Record<string, string> = {
	service: 'Service',
	dns_record: 'DNS entry',
	firewall_host: 'Firewall host',
	traefik: 'Traefik host',
	hashi: 'Hashi',
	adguard: 'AdGuard',
	pulse: 'Pulse agent',
	user: 'Your check'
};

export const stateOrder: Record<string, number> = {
	down: 0,
	degraded: 1,
	unknown: 2,
	paused: 3,
	up: 4
};

/** The public status page's one-line summary (07 §7, bugs 80, 145): what is wrong, nothing more; null when nothing is listed. */
export function publicSummary(
	states: string[]
): { text: string; tone: 'ok' | 'warn' | 'bad' } | null {
	if (states.length === 0) return null;
	const down = states.filter((s) => s === 'down').length;
	const degraded = states.filter((s) => s === 'degraded').length;
	const n = (k: number, one: string, many: string) =>
		k === 1 ? `1 service ${one}` : `${k} services ${many}`;
	if (down && degraded)
		return { text: `${n(down, 'is down', 'are down')}, ${degraded} degraded.`, tone: 'bad' };
	if (down) return { text: `${n(down, 'is down', 'are down')}.`, tone: 'bad' };
	if (degraded) return { text: `${n(degraded, 'is degraded', 'are degraded')}.`, tone: 'warn' };
	// Paused or without data yet: not known to work (bug 168).
	const unchecked = states.filter((s) => s === 'paused' || s === 'unknown').length;
	if (unchecked === states.length) return null;
	return {
		text: unchecked ? 'All checked services are working.' : 'All services are working.',
		tone: 'ok'
	};
}

/** What the history strip says to a screen reader: only real results, not the grey padding (bug 87). */
export function stripSummary(samples: Health[]): string {
	const real = samples.filter((s) => s !== 'unknown');
	if (real.length === 0) return 'No checks yet';
	const down = real.filter((s) => s === 'down').length;
	const slow = real.filter((s) => s === 'degraded').length;
	return `${real.length} ${real.length === 1 ? 'check' : 'checks'}: ${down} down, ${slow} degraded`;
}
