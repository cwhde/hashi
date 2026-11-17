import type { Schemas } from '#lib/api/client.ts';

// What uses an IP set, as a kind and a link to where it is set (bugs 57, 63: every reference is a named link).
type Consumer = Pick<Schemas['IPSetConsumer'], 'type' | 'id'>;

const kinds: Record<string, [string, (id: string) => string]> = {
	ip_set: ['Included in set', (id) => `/ip-sets/${id}`],
	service: ['Service upstream', (id) => `/services/${id}`],
	service_rules: ['Service access', (id) => `/services/${id}?tab=access`],
	rule_set: ['Rule set', (id) => `/rule-sets/${id}`],
	dns_record: ['DNS record', () => '/dns?tab=records'],
	adguard: ['AdGuard connection', () => '/connections'],
	firewall_binding: ['Firewall policy', () => '/firewall'],
	firewall_host_access: ['Host access', (id) => `/firewall/${id}?tab=access`],
	firewall_ssh: ['SSH target', (id) => `/firewall/${id}`],
	traefik_ssh: ['SSH target', () => '/traefik']
};

export function consumerOf(c: Consumer): { kind: string; href?: string } {
	const k = kinds[c.type];
	return k ? { kind: k[0], href: k[1](c.id) } : { kind: c.type };
}
