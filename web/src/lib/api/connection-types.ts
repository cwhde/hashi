export const CONNECTION_TYPES = {
	dnsProvider: 'dns_provider',
	traefikHost: 'traefik_host',
	firewallHost: 'firewall_host'
} as const;

export const SSH_CONNECTION_TYPE_OPTIONS = [
	{ value: CONNECTION_TYPES.traefikHost, label: 'Traefik host' },
	{ value: CONNECTION_TYPES.firewallHost, label: 'Firewall host' }
] as const;

export type SshConnectionType = (typeof SSH_CONNECTION_TYPE_OPTIONS)[number]['value'];
