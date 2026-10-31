// Helpers for the service pages (04-traefik-services.md §1).
import { api, type Schemas } from '$lib/api/client';

export type Service = Schemas['Service'];
export type ServiceInput = Schemas['ServiceInput'];
export type Route = Schemas['ServiceRoute'];
export type Target = Schemas['ServiceTarget'];

export const kinds = ['http', 'https', 'h2c', 'tcp', 'udp'] as const;

/** Streams have their own public port and no routes or TLS options. */
export const isStream = (k: string) => k === 'tcp' || k === 'udp';

export const kindText: Record<Service['kind'], string> = {
	http: 'HTTP upstream',
	https: 'HTTPS upstream',
	h2c: 'HTTP/2 without TLS (h2c)',
	tcp: 'TCP on its own public port',
	udp: 'UDP on its own public port'
};

/** A slug from a display name: lowercase letters, digits and dashes. */
export function slugify(name: string): string {
	return name
		.toLowerCase()
		.normalize('NFKD')
		.replace(/[^a-z0-9]+/g, '-')
		.replace(/^-+|-+$/g, '')
		.slice(0, 63);
}

/** How a full host name relates to the root domain (the API stores both). */
export function domainMode(host: string, root: string): Service['domain_mode'] {
	if (!host) return 'none';
	if (host === root) return 'root';
	return host.endsWith('.' + root) ? 'subdomain' : 'custom';
}

/** "192.0.2.5:8080" or "[2001:db8::5]:8080", with "×3" for a weight. */
export function targetText(t: Target): string {
	if (t.set_id) return `@${t.set_id}:${t.port}`;
	const host = t.address.includes(':') ? `[${t.address}]` : t.address;
	return `${host}:${t.port}${t.weight && t.weight > 1 ? ` ×${t.weight}` : ''}`;
}

/** Parses "address:port[*weight]" entries separated by commas or spaces; `bad` names the first entry that does not parse. */
export function parseTargets(text: string): { targets: Target[]; bad?: string } {
	const targets: Target[] = [];
	for (const raw of text.split(/[\s,]+/).filter(Boolean)) {
		const set = /^@([0-9A-Za-z]+):(\d{1,5})$/.exec(raw); // an address set, as targetText writes it
		if (set) {
			targets.push({ address: '', set_id: set[1], port: Number(set[2]) });
			continue;
		}
		const m = /^(\[[^\]]+\]|[^:[\]*]+):(\d{1,5})(?:\*(\d{1,4}))?$/.exec(raw);
		if (!m) return { targets, bad: raw };
		const t: Target = { address: (m[1] ?? '').replace(/^\[|\]$/g, ''), port: Number(m[2]) };
		if (m[3]) t.weight = Number(m[3]);
		targets.push(t);
	}
	return { targets };
}

/** The editable fields of a stored service, ready to send back. */
export function toInput(s: Service): ServiceInput {
	return {
		name: s.name,
		slug: s.slug,
		kind: s.kind,
		domain_mode: s.domain_mode,
		host_name: s.host_name,
		target: { ...s.target },
		tls_skip_verify: s.tls_skip_verify,
		tls_server_name: s.tls_server_name,
		tls_min_version: s.tls_min_version,
		advanced_routes: s.advanced_routes,
		routes: s.routes.map((r) => ({ ...r, targets: r.targets.map((t) => ({ ...t })) })),
		dashboard_visible: s.dashboard_visible,
		monitoring_visible: s.monitoring_visible,
		adguard_rewrite: s.adguard_rewrite,
		public_port: s.public_port,
		proxy_protocol: s.proxy_protocol,
		entry_host_id: s.entry_host_id
	};
}

export const rewriteText: Record<Route['rewrite_mode'], string> = {
	none: 'no rewrite',
	strip_prefix: 'strip prefix',
	replace_prefix: 'replace prefix',
	exact: 'replace path',
	regex: 'regex replace'
};

/** A new route below the existing ones' priorities. */
export function newRoute(routes: Route[], target: Target): Route {
	const top = routes.reduce((m, r) => Math.max(m, r.priority), 0);
	return {
		enabled: true,
		targets: [{ ...target }],
		path_match: 'prefix',
		path_value: '/',
		priority: Math.min(top + 10, 9999),
		rewrite_mode: 'none',
		rewrite_pattern: '',
		rewrite_value: ''
	};
}

/** Plain text for what a save did on the Traefik hosts, the DNS zone and the firewall hosts. */
export function saveText(r: { plans: Schemas['SyncPlan'][]; certificate_pending: boolean }): {
	text: string;
	review: boolean;
	bad: boolean;
} {
	const failed = r.plans.filter((p) => p.status === 'failed');
	const waiting = r.plans.filter((p) => p.status === 'pending');
	const applied = r.plans.filter((p) => p.status === 'applied');
	const onHosts = applied.filter((p) => p.subsystem === 'traefik').length;
	const parts: string[] = [];
	if (onHosts) parts.push(`applied on ${onHosts} Traefik host${onHosts === 1 ? '' : 's'}`);
	if (applied.some((p) => p.subsystem.startsWith('dns-'))) parts.push('DNS record updated');
	if (applied.some((p) => p.subsystem === 'firewall')) parts.push('firewall updated');
	if (waiting.length) parts.push(`${waiting.length} plan(s) need your approval`);
	if (failed.length) parts.push(`failed on ${failed.length}: ${failed[0]?.error}`);
	if (!r.plans.length) parts.push('nothing to change on the Traefik hosts');
	if (r.certificate_pending)
		parts.push('a certificate for the new name is being issued (about a minute)');
	return {
		text: 'Saved: ' + parts.join('; ') + '.',
		review: waiting.length > 0,
		bad: failed.length > 0
	};
}

/** The root domain: General settings for admins, the setup state for a setup session (it has no settings scope). */
export async function rootDomain(): Promise<string> {
	const g = await api.GET('/api/settings/general');
	if (g.data) return g.data.root_domain;
	const st = await api.GET('/api/setup');
	if (st.error || !st.data) throw st.error;
	return st.data.base_settings?.root_domain ?? '';
}
