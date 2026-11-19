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
