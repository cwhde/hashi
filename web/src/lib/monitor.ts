import type { Schemas } from '$lib/api/client';
import type { Health } from '$lib/design/mock';

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
	return c ? `${c.type} · ${c.target}` : 'no checks';
}

export const sourceText: Record<string, string> = {
	service: 'Service',
	dns_record: 'DNS entry',
	firewall_host: 'Firewall host',
	traefik: 'Traefik host',
	hashi: 'Hashi',
	user: 'Your check'
};

export const stateOrder: Record<string, number> = {
	down: 0,
	degraded: 1,
	unknown: 2,
	paused: 3,
	up: 4
};
