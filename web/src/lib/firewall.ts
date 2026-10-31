import type { Schemas } from '$lib/api/client';

export type FirewallHost = Schemas['FirewallHost'];
export type FirewallInput = Schemas['FirewallHostInput'];

/** The editable fields of a stored host, ready to send back. The pinned key stays on the server. */
export function firewallInput(h: FirewallHost): FirewallInput {
	return {
		name: h.name,
		edge_mode: h.edge_mode,
		host: h.host,
		port: h.port,
		user: h.user,
		auth: h.auth,
		traefik_connection_id: h.traefik_connection_id,
		internal_traefik_ip: h.internal_traefik_ip,
		route_via_host_id: h.route_via_host_id,
		public_ip_override: h.public_ip_override,
		public_ip6_override: h.public_ip6_override,
		wan_iface_override: h.wan_iface_override,
		subnets: [...h.subnets],
		netbird_mode: h.netbird_mode,
		netbird_iface: h.netbird_iface,
		netbird_port: h.netbird_port,
		drop_other_inbound: h.drop_other_inbound
	};
}

/** A /24 around an IPv4 address: the first guess for the network the Traefik host sits in. */
export function subnetOf(ip: string): string {
	const m = /^(\d+\.\d+\.\d+)\.\d+$/.exec(ip.trim());
	return m ? `${m[1]}.0/24` : '';
}

export const lines = (s: string) =>
	s
		.split(/[\s,]+/)
		.map((x) => x.trim())
		.filter(Boolean);

export const healthState = (h: FirewallHost) =>
	h.health === 'healthy'
		? ('up' as const)
		: h.health === 'degraded'
			? ('degraded' as const)
			: h.health === 'failed'
				? ('down' as const)
				: ('unknown' as const);
