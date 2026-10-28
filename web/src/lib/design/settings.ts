// Settings catalogue for the mock (09-scripts-settings.md §2). kind drives the control; reauth marks passkey-gated fields.
export type Field = {
	label: string;
	value: string | boolean;
	kind?: 'text' | 'switch' | 'mono';
	reauth?: boolean;
	hint?: string;
};

export const settingsCatalogue: Record<string, Field[]> = {
	Security: [
		{
			label: 'Admin idle timeout',
			value: '4 h',
			hint: 'At most 4 h; sessions end after 8 h regardless'
		},
		{ label: 'Edge sign-in session length', value: '7 d' },
		{ label: 'Edge sign-in idle timeout', value: '24 h' },
		{ label: 'Remember device', value: true, kind: 'switch' },
		{ label: 'Fallback for new HTTP services', value: 'Open (None)' },
		{ label: 'Default WAF mode', value: 'enabled' },
		{ label: 'Challenge after requests', value: '300 in 10 s' },
		{ label: 'Ban after requests without solving', value: '15' },
		{ label: 'Ban length', value: '15 min × 2 per ban, max 30 d' },
		{ label: 'Counters drop by one after', value: '7 quiet days' },
		{ label: 'Vault mode', value: 'Service', reauth: true }
	],
	Monitoring: [
		{ label: 'Check interval', value: '60 s' },
		{ label: 'Timeout', value: '10 s' },
		{ label: 'Healthy HTTP codes', value: '200–399', kind: 'mono' },
		{ label: 'Degraded above', value: '300 ms' },
		{ label: 'Keep raw samples', value: '30 d' },
		{ label: 'Public status page', value: true, kind: 'switch' }
	],
	Dashboard: [
		{ label: 'Public app dashboard', value: true, kind: 'switch' },
		{ label: 'Default sort', value: 'Group' },
		{ label: 'New services visible by default', value: true, kind: 'switch' }
	],
	DNS: [
		{ label: 'Default TTL', value: '300 s' },
		{
			label: 'Removing imported records',
			value: 'Always ask',
			hint: 'Removals are always a separate high-risk plan'
		},
		{ label: 'Show newly found records on import', value: true, kind: 'switch' }
	],
	Traefik: [
		{ label: 'Config directory', value: '/etc/hashi/traefik', kind: 'mono' },
		{ label: 'Access log', value: '/var/log/hashi/traefik/access.log', kind: 'mono' },
		{ label: 'ACME email', value: 'admin@example.com' },
		{ label: 'Install patch updates automatically', value: false, kind: 'switch' }
	],
	Firewall: [
		{ label: 'Ask before opening a port', value: true, kind: 'switch' },
		{ label: 'Persistence', value: 'netfilter-persistent' },
		{ label: 'Default NetBird mode', value: 'Peer only' },
		{ label: 'NetBird interface', value: 'wt0', kind: 'mono' },
		{ label: 'Clamp MSS on NetBird traffic', value: true, kind: 'switch' },
		{ label: 'Trusted CIDRs', value: '192.0.2.0/24', kind: 'mono', reauth: true }
	],
	Notifications: [
		{ label: 'Default route', value: 'Telegram “Ops”' },
		{ label: 'Cooldown', value: '10 min' },
		{ label: 'Send recovery messages', value: true, kind: 'switch' }
	],
	Pulse: [
		{ label: 'Heartbeat interval', value: '30 s' },
		{ label: 'Stale after', value: '3 missed heartbeats' }
	],
	'Internal agent DNS': [
		{ label: 'Enabled', value: true, kind: 'switch' },
		{ label: 'Domain', value: 'hashi.home.arpa', kind: 'mono', reauth: true },
		{ label: 'Address to publish', value: 'Selected IP' },
		{ label: 'Remove names of stale agents', value: false, kind: 'switch' }
	]
};
