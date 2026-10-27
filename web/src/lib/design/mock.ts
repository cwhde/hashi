// Mock data for the /design pages (S0b). Shapes follow the spec's data model loosely; feature slices replace this
// with the generated API client. Only example.com names and documentation IP ranges (public repo).

export type Health = 'up' | 'degraded' | 'down' | 'paused' | 'unknown';
export type Origin = 'system' | 'managed' | 'imported' | 'user';
export type Risk = 'low' | 'high';

/** Deterministic pseudo-random history so screenshots are stable. */
function strip(seed: number, n = 60, failAt: number[] = [], slowAt: number[] = []): Health[] {
	return Array.from({ length: n }, (_, i) => {
		if (failAt.includes(i)) return 'down';
		if (slowAt.includes(i)) return 'degraded';
		return (seed * (i + 7)) % 97 === 0 ? 'degraded' : 'up';
	});
}

export function spark(seed: number, n = 48, base = 40, amp = 18): number[] {
	return Array.from({ length: n }, (_, i) => {
		const v =
			base + Math.sin((i + seed) / 3.1) * amp * 0.6 + Math.sin((i * seed) / 7.3) * amp * 0.4;
		return Math.max(1, Math.round(v));
	});
}

export const zone = {
	name: 'example.com',
	provider: 'Hetzner Cloud',
	records: 64,
	lastSync: '2 min ago'
};

export const firewallHosts = [
	{
		id: 'fh1',
		name: 'edge-fra',
		fqdn: 'edge-fra.example.com',
		mode: 'managed' as const,
		publicIp: '198.51.100.24',
		publicIp6: '2001:db8:24::1',
		os: 'Debian 13',
		subnets: ['10.20.4.0/24'],
		traefik: 'traefik-fra',
		traefikIp: '10.20.4.11',
		netbird: { mode: 'routing peer', iface: 'wt0', overlay: '100.110.0.0/16', status: 'connected' },
		ports: ['80/tcp', '443/tcp', '443/udp', '222/tcp', '19132/udp'],
		health: 'up' as Health,
		lastApply: '14 min ago',
		rescue: 'disarmed after verification',
		blocked: 37,
		history: strip(3)
	},
	{
		id: 'fh2',
		name: 'edge-home',
		fqdn: 'edge-home.example.com',
		mode: 'external_nat' as const,
		publicIp: '203.0.113.88',
		publicIp6: null,
		os: 'Debian 13',
		subnets: ['10.20.0.0/22'],
		traefik: 'traefik-home',
		traefikIp: '10.20.1.11',
		netbird: { mode: 'disabled', iface: '—', overlay: '—', status: 'not installed' },
		ports: ['80/tcp', '443/tcp'],
		health: 'up' as Health,
		lastApply: 'never (external NAT)',
		rescue: '—',
		blocked: 0,
		history: strip(5, 60, [], [41])
	}
];

export const traefikHosts = [
	{
		id: 't1',
		name: 'traefik-fra',
		host: '10.20.4.11',
		os: 'Alpine 3.24',
		version: '3.7.16',
		update: '3.7.18',
		files: 6,
		changed: 1,
		routers: 18,
		errors: 0,
		lastVerify: '14 min ago',
		health: 'up' as Health,
		acme: 'hashi-gts',
		certs: 14,
		certSoonest: 'grafana.example.com · 41 d'
	},
	{
		id: 't2',
		name: 'traefik-home',
		host: '10.20.1.11',
		os: 'Alpine 3.24',
		version: '3.7.16',
		update: '3.7.18',
		files: 6,
		changed: 0,
		routers: 18,
		errors: 0,
		lastVerify: '14 min ago',
		health: 'up' as Health,
		acme: 'hashi-gts',
		certs: 14,
		certSoonest: 'media.example.com · 18 d'
	}
];

export type Resource = {
	id: string;
	name: string;
	domain: string;
	kind: 'http' | 'https' | 'h2c' | 'tcp' | 'udp';
	target: string;
	host: string;
	origin: Origin;
	enabled: boolean;
	auth: 'Off' | 'SSO required' | 'Adaptive' | 'Observe';
	waf: 'off' | 'detect' | 'block';
	health: Health;
	uptime: string;
	latency: number;
	sync: 'in sync' | 'pending' | 'invalid';
	dashboard: boolean;
	history: Health[];
	group: string;
	port?: number;
};

export const resources: Resource[] = [
	{
		id: 'r0',
		name: 'Hashi',
		domain: 'hashi.example.com',
		kind: 'http',
		target: 'http://10.20.4.106:8080',
		host: 'edge-fra',
		origin: 'system',
		enabled: true,
		auth: 'SSO required',
		waf: 'block',
		health: 'up',
		uptime: '100%',
		latency: 21,
		sync: 'in sync',
		dashboard: false,
		history: strip(11),
		group: 'System'
	},
	{
		id: 'r1',
		name: 'Grafana',
		domain: 'grafana.example.com',
		kind: 'http',
		target: 'http://10.20.1.103:3000',
		host: 'edge-home',
		origin: 'user',
		enabled: true,
		auth: 'SSO required',
		waf: 'detect',
		health: 'up',
		uptime: '99.98%',
		latency: 48,
		sync: 'in sync',
		dashboard: true,
		history: strip(13),
		group: 'Monitoring'
	},
	{
		id: 'r2',
		name: 'Gitea',
		domain: 'git.example.com',
		kind: 'http',
		target: 'http://10.20.4.106:3232',
		host: 'edge-fra',
		origin: 'imported',
		enabled: true,
		auth: 'Adaptive',
		waf: 'block',
		health: 'up',
		uptime: '99.95%',
		latency: 36,
		sync: 'in sync',
		dashboard: true,
		history: strip(17, 60, [], [12]),
		group: 'Development'
	},
	{
		id: 'r3',
		name: 'Media',
		domain: 'media.example.com',
		kind: 'https',
		target: 'https://10.20.1.102:8920',
		host: 'edge-home',
		origin: 'user',
		enabled: true,
		auth: 'Adaptive',
		waf: 'detect',
		health: 'degraded',
		uptime: '98.71%',
		latency: 412,
		sync: 'in sync',
		dashboard: true,
		history: strip(19, 60, [48], [44, 45, 46, 52, 53, 58, 59]),
		group: 'Media'
	},
	{
		id: 'r4',
		name: 'Photos',
		domain: 'photos.example.com',
		kind: 'http',
		target: 'http://10.20.1.102:2283',
		host: 'edge-home',
		origin: 'user',
		enabled: true,
		auth: 'SSO required',
		waf: 'detect',
		health: 'up',
		uptime: '99.99%',
		latency: 63,
		sync: 'pending',
		dashboard: true,
		history: strip(23),
		group: 'Media'
	},
	{
		id: 'r5',
		name: 'Home Assistant',
		domain: 'home.example.com',
		kind: 'http',
		target: 'http://10.20.1.120:8123',
		host: 'edge-home',
		origin: 'imported',
		enabled: true,
		auth: 'SSO required',
		waf: 'detect',
		health: 'down',
		uptime: '96.40%',
		latency: 0,
		sync: 'in sync',
		dashboard: true,
		history: strip(29, 60, [53, 54, 55, 56, 57, 58, 59]),
		group: 'Home'
	},
	{
		id: 'r6',
		name: 'Passwords',
		domain: 'vault.example.com',
		kind: 'http',
		target: 'http://10.20.4.106:8222',
		host: 'edge-fra',
		origin: 'imported',
		enabled: true,
		auth: 'Adaptive',
		waf: 'block',
		health: 'up',
		uptime: '100%',
		latency: 29,
		sync: 'in sync',
		dashboard: true,
		history: strip(31),
		group: 'Security'
	},
	{
		id: 'r7',
		name: 'Search',
		domain: 'search.example.com',
		kind: 'http',
		target: 'http://10.20.4.106:8082',
		host: 'edge-fra',
		origin: 'user',
		enabled: false,
		auth: 'Observe',
		waf: 'off',
		health: 'paused',
		uptime: '—',
		latency: 0,
		sync: 'in sync',
		dashboard: false,
		history: Array(60).fill('paused'),
		group: 'Tools'
	},
	{
		id: 'r8',
		name: 'Git SSH',
		domain: 'git.example.com',
		kind: 'tcp',
		target: '10.20.4.106:22',
		host: 'edge-fra',
		origin: 'user',
		enabled: true,
		auth: 'Off',
		waf: 'off',
		health: 'up',
		uptime: '99.99%',
		latency: 12,
		sync: 'in sync',
		dashboard: false,
		history: strip(37),
		group: 'Development',
		port: 222
	},
	{
		id: 'r9',
		name: 'Minecraft',
		domain: 'mc.example.com',
		kind: 'udp',
		target: '10.20.1.130:19132',
		host: 'edge-home',
		origin: 'user',
		enabled: true,
		auth: 'Off',
		waf: 'off',
		health: 'unknown',
		uptime: '—',
		latency: 0,
		sync: 'invalid',
		dashboard: true,
		history: Array(60).fill('unknown'),
		group: 'Games',
		port: 19132
	},
	{
		id: 'r10',
		name: 'Challenge',
		domain: 'check.example.com',
		kind: 'http',
		target: 'http://10.20.4.106:3030',
		host: 'edge-fra',
		origin: 'system',
		enabled: true,
		auth: 'Off',
		waf: 'block',
		health: 'up',
		uptime: '100%',
		latency: 18,
		sync: 'in sync',
		dashboard: false,
		history: strip(41),
		group: 'System'
	}
];

export type Rrset = {
	id: string;
	name: string;
	type: string;
	ttl: number | 'zone';
	values: string[];
	origin: Origin | 'unmanaged';
	generated?: boolean;
	protected?: boolean;
};

export const rrsets: Rrset[] = [
	{
		id: 'd1',
		name: '@',
		type: 'NS',
		ttl: 86400,
		values: ['hydrogen.ns.hetzner.com.', 'oxygen.ns.hetzner.com.', 'helium.ns.hetzner.de.'],
		origin: 'unmanaged',
		protected: true
	},
	{
		id: 'd2',
		name: '@',
		type: 'SOA',
		ttl: 86400,
		values: ['hydrogen.ns.hetzner.com. dns.hetzner.com. 2026102701 86400 10800 3600000 3600'],
		origin: 'unmanaged',
		protected: true
	},
	{
		id: 'd3',
		name: '@',
		type: 'MX',
		ttl: 'zone',
		values: ['10 mx1.mail.example.net.', '20 mx2.mail.example.net.'],
		origin: 'imported'
	},
	{
		id: 'd4',
		name: '@',
		type: 'TXT',
		ttl: 3600,
		values: ['"v=spf1 include:_spf.mail.example.net ~all"'],
		origin: 'imported'
	},
	{
		id: 'd5',
		name: 'edge-fra',
		type: 'A',
		ttl: 300,
		values: ['198.51.100.24'],
		origin: 'system',
		generated: true
	},
	{
		id: 'd6',
		name: 'edge-fra',
		type: 'AAAA',
		ttl: 300,
		values: ['2001:db8:24::1'],
		origin: 'system',
		generated: true
	},
	{
		id: 'd7',
		name: 'via.edge-fra',
		type: 'CNAME',
		ttl: 300,
		values: ['edge-fra.example.com.'],
		origin: 'system',
		generated: true
	},
	{
		id: 'd8',
		name: 'on.edge-fra',
		type: 'CNAME',
		ttl: 300,
		values: ['via.edge-fra.example.com.'],
		origin: 'system',
		generated: true
	},
	{
		id: 'd9',
		name: 'on.edge-home',
		type: 'CNAME',
		ttl: 300,
		values: ['via.edge-fra.example.com.'],
		origin: 'system',
		generated: true
	},
	{
		id: 'd10',
		name: 'grafana',
		type: 'CNAME',
		ttl: 300,
		values: ['on.edge-home.example.com.'],
		origin: 'managed',
		generated: true
	},
	{
		id: 'd11',
		name: 'git',
		type: 'CNAME',
		ttl: 300,
		values: ['on.edge-fra.example.com.'],
		origin: 'managed',
		generated: true
	},
	{ id: 'd12', name: 'nas', type: 'A', ttl: 3600, values: ['10.20.1.50'], origin: 'user' },
	{ id: 'd13', name: 'printer', type: 'A', ttl: 3600, values: ['10.20.1.60'], origin: 'user' },
	{
		id: 'd14',
		name: '_dmarc',
		type: 'TXT',
		ttl: 3600,
		values: ['"v=DMARC1; p=quarantine; rua=mailto:dmarc@example.com"'],
		origin: 'imported'
	},
	{
		id: 'd15',
		name: 'legacy',
		type: 'CNAME',
		ttl: 3600,
		values: ['old-host.example.net.'],
		origin: 'unmanaged'
	},
	{
		id: 'd16',
		name: 'mail',
		type: 'CNAME',
		ttl: 'zone',
		values: ['mx1.mail.example.net.'],
		origin: 'imported'
	}
];

export type PlanItem = {
	action: 'create' | 'update' | 'delete' | 'conflict' | 'write file' | 'restart';
	subsystem: 'DNS' | 'Traefik' | 'Firewall' | 'AdGuard';
	target: string;
	object: string;
	before?: string;
	after?: string;
	risk: Risk;
	note?: string;
};

export const plans = [
	{
		id: 'p1',
		title: 'Add Photos at photos.example.com',
		trigger: 'Resource saved · 3 min ago',
		risk: 'high' as Risk,
		status: 'awaiting approval',
		items: [
			{
				action: 'create',
				subsystem: 'DNS',
				target: 'example.com',
				object: 'photos CNAME',
				after: 'on.edge-home.example.com.  TTL 300',
				risk: 'low'
			},
			{
				action: 'write file',
				subsystem: 'Traefik',
				target: 'traefik-fra',
				object: '10-hashi-http-resources.yml',
				before: 'sha256 9f2c…41ab',
				after: 'sha256 1d07…c3e2  (+14 lines)',
				risk: 'low'
			},
			{
				action: 'write file',
				subsystem: 'Traefik',
				target: 'traefik-home',
				object: '10-hashi-http-resources.yml',
				before: 'sha256 9f2c…41ab',
				after: 'sha256 1d07…c3e2  (+14 lines)',
				risk: 'low'
			},
			{
				action: 'create',
				subsystem: 'AdGuard',
				target: 'adguard-home',
				object: 'photos.example.com rewrite',
				after: '10.20.1.11',
				risk: 'low'
			},
			{
				action: 'delete',
				subsystem: 'DNS',
				target: 'example.com',
				object: 'photos A',
				before: '203.0.113.88  TTL 3600',
				risk: 'high',
				note: 'Imported record replaced by the managed CNAME'
			}
		] as PlanItem[]
	},
	{
		id: 'p2',
		title: 'Open public port 19132/udp for Minecraft',
		trigger: 'Resource saved · 1 h ago',
		risk: 'high' as Risk,
		status: 'awaiting approval',
		items: [
			{
				action: 'update',
				subsystem: 'Traefik',
				target: 'traefik-fra',
				object: 'static config · entry point udp-19132',
				before: '—',
				after: ':19132/udp',
				risk: 'high',
				note: 'Traefik restarts; ping must answer within 30 s'
			},
			{
				action: 'update',
				subsystem: 'Firewall',
				target: 'edge-fra',
				object: 'HASHI_DNAT',
				before: '4 rules',
				after: '5 rules (+ udp 19132 → 10.20.4.11:19132)',
				risk: 'high',
				note: 'Applied under a 180 s rescue timer'
			}
		] as PlanItem[]
	}
];

export const agents = [
	{
		id: 'a1',
		name: 'nas',
		status: 'fresh',
		version: '0.3.1',
		publicIp: '203.0.113.88',
		selected: '10.20.1.50',
		candidates: ['10.20.1.50 (eth0)', '172.17.0.1 (docker0)'],
		lastSeen: '28 s ago',
		reach: 'reachable',
		dns: 'nas.hashi.home.arpa',
		install: 'systemd'
	},
	{
		id: 'a2',
		name: 'laptop',
		status: 'stale',
		version: '0.3.0',
		publicIp: '192.0.2.77',
		selected: '192.168.1.23',
		candidates: ['192.168.1.23 (wlan0)'],
		lastSeen: '3 h ago',
		reach: 'not reachable',
		dns: 'laptop.hashi.home.arpa',
		install: 'systemd'
	},
	{
		id: 'a3',
		name: 'game-box',
		status: 'fresh',
		version: '0.3.1',
		publicIp: '203.0.113.88',
		selected: '10.20.1.130',
		candidates: ['10.20.1.130 (enp3s0)'],
		lastSeen: '41 s ago',
		reach: 'reachable',
		dns: 'game-box.hashi.home.arpa',
		install: 'docker'
	}
];

export const monitors = [
	...resources
		.filter((r) => r.kind !== 'udp' || r.health !== 'unknown')
		.map((r) => ({
			id: 'm-' + r.id,
			name: r.name,
			group: r.group,
			kind: r.kind.toUpperCase(),
			address: r.domain,
			state: r.health,
			latency: r.latency,
			uptime24: r.uptime,
			history: r.history,
			public: r.dashboard,
			source: 'Resource'
		})),
	{
		id: 'm-fh1',
		name: 'edge-fra',
		group: 'Infrastructure',
		kind: 'TCP',
		address: 'edge-fra.example.com:22',
		state: 'up' as Health,
		latency: 9,
		uptime24: '100%',
		history: strip(43),
		public: true,
		source: 'Firewall host'
	},
	{
		id: 'm-fh2',
		name: 'edge-home',
		group: 'Infrastructure',
		kind: 'TCP',
		address: 'edge-home.example.com:22',
		state: 'up' as Health,
		latency: 22,
		uptime24: '99.99%',
		history: strip(47),
		public: true,
		source: 'Firewall host'
	},
	{
		id: 'm-dns',
		name: 'nas',
		group: 'Home',
		kind: 'HTTPS',
		address: 'nas.example.com',
		state: 'up' as Health,
		latency: 74,
		uptime24: '99.9%',
		history: strip(53),
		public: false,
		source: 'DNS entry'
	},
	{
		id: 'm-cert',
		name: 'media certificate',
		group: 'Media',
		kind: 'TLS',
		address: 'media.example.com',
		state: 'degraded' as Health,
		latency: 0,
		uptime24: '—',
		history: strip(
			59,
			60,
			[],
			Array.from({ length: 60 }, (_, i) => i)
		),
		public: false,
		source: 'Expires in 18 d'
	}
];

export const securityTop = {
	ips: [
		{
			v: '192.0.2.201',
			count: 4812,
			country: 'NL',
			asn: 'AS64500 Example Hosting',
			reason: 'Firewall block · rate',
			until: '6 d'
		},
		{
			v: '198.51.100.77',
			count: 2290,
			country: 'US',
			asn: 'AS64501 Cloud Example',
			reason: 'Blocklist · Spamhaus DROP',
			until: 'while listed'
		},
		{
			v: '203.0.113.9',
			count: 977,
			country: 'DE',
			asn: 'AS64502 Example DSL',
			reason: 'Soft block · challenge ignored',
			until: '8 min'
		},
		{
			v: '192.0.2.18',
			count: 611,
			country: 'SG',
			asn: 'AS64503 Example Transit',
			reason: 'WAF · 942100 SQLi',
			until: '1 h'
		},
		{
			v: '2001:db8:aa::5',
			count: 203,
			country: 'FR',
			asn: 'AS64504 Example Mobile',
			reason: 'Manual block',
			until: 'permanent'
		}
	],
	countries: [
		{ v: 'NL', n: 5310 },
		{ v: 'US', n: 3902 },
		{ v: 'SG', n: 1214 },
		{ v: 'DE', n: 990 },
		{ v: 'BR', n: 512 }
	],
	resources: [
		{ v: 'git.example.com', n: 6120 },
		{ v: 'vault.example.com', n: 2210 },
		{ v: 'media.example.com', n: 1450 }
	]
};

export const securityEvents = [
	{
		t: '13:41:08',
		subject: '203.0.113.9',
		type: 'challenge ignored',
		resource: 'git.example.com',
		decision: 'soft block',
		sev: 'warn'
	},
	{
		t: '13:40:52',
		subject: '192.0.2.18',
		type: 'WAF match 942100',
		resource: 'vault.example.com',
		decision: 'deny',
		sev: 'bad'
	},
	{
		t: '13:39:31',
		subject: '198.51.100.12',
		type: 'challenge solved',
		resource: 'media.example.com',
		decision: 'allow',
		sev: 'ok'
	},
	{
		t: '13:37:02',
		subject: '192.0.2.201',
		type: 'firewall block applied',
		resource: 'all hosts',
		decision: 'block',
		sev: 'bad'
	},
	{
		t: '13:35:44',
		subject: 'AS64502',
		type: 'rate threshold',
		resource: 'git.example.com',
		decision: 'challenge',
		sev: 'warn'
	}
];

export const blocklists = [
	{
		name: 'Spamhaus DROP',
		enabled: true,
		entries: 1192,
		mode: 'firewall + middleware',
		fp: 'very low',
		updated: '2 h ago',
		status: 'ok'
	},
	{
		name: 'Spamhaus DROPv6',
		enabled: true,
		entries: 82,
		mode: 'firewall + middleware',
		fp: 'very low',
		updated: '2 h ago',
		status: 'ok'
	},
	{
		name: 'Feodo Tracker C2',
		enabled: true,
		entries: 311,
		mode: 'middleware',
		fp: 'low',
		updated: '41 min ago',
		status: 'ok'
	},
	{
		name: 'DShield recommended',
		enabled: false,
		entries: 0,
		mode: '—',
		fp: 'medium',
		updated: 'never',
		status: 'off'
	},
	{
		name: 'FireHOL Level 1',
		enabled: true,
		entries: 4520,
		mode: 'middleware',
		fp: 'low',
		updated: '3 d ago',
		status: 'stale'
	}
];

export const scripts = [
	{
		id: 's1',
		name: 'Rotate Docker logs',
		schedule: '0 4 * * *',
		hosts: 'all firewall hosts',
		last: 'succeeded · today 04:00',
		enabled: true,
		runs: ['ok', 'ok', 'ok', 'ok', 'ok', 'ok', 'ok']
	},
	{
		id: 's2',
		name: 'Prune ZFS snapshots',
		schedule: '30 3 * * 0',
		hosts: 'edge-home',
		last: 'failed · Sun 03:30',
		enabled: true,
		runs: ['ok', 'ok', 'bad', 'ok', 'ok', 'ok', 'bad']
	},
	{
		id: 's3',
		name: 'Update package lists',
		schedule: 'manual',
		hosts: 'all firewall hosts',
		last: 'succeeded · 3 d ago',
		enabled: true,
		runs: ['ok', 'ok']
	}
];

export const connections = [
	{
		type: 'DNS provider',
		name: 'Hetzner Cloud',
		detail: 'zone example.com',
		health: 'up' as Health,
		required: true,
		validated: '2 min ago'
	},
	{
		type: 'Traefik',
		name: 'traefik-fra',
		detail: 'SSH 10.20.4.11:22 · key pinned',
		health: 'up' as Health,
		required: true,
		validated: '14 min ago'
	},
	{
		type: 'Traefik',
		name: 'traefik-home',
		detail: 'SSH 10.20.1.11:22 · key pinned',
		health: 'up' as Health,
		required: false,
		validated: '14 min ago'
	},
	{
		type: 'Firewall host',
		name: 'edge-fra',
		detail: 'SSH 198.51.100.24:22 · managed',
		health: 'up' as Health,
		required: true,
		validated: '14 min ago'
	},
	{
		type: 'Firewall host',
		name: 'edge-home',
		detail: 'external NAT',
		health: 'up' as Health,
		required: false,
		validated: '1 h ago'
	},
	{
		type: 'AdGuard Home',
		name: 'adguard-home',
		detail: 'http://10.20.1.100:3000',
		health: 'up' as Health,
		required: false,
		validated: '5 min ago'
	},
	{
		type: 'AdGuard Home',
		name: 'adguard-fra',
		detail: 'agent-bound · nas (selected IP)',
		health: 'degraded' as Health,
		required: false,
		validated: '5 min ago'
	},
	{
		type: 'OIDC provider',
		name: 'Pocket ID',
		detail: 'https://sso.example.com',
		health: 'down' as Health,
		required: false,
		validated: '6 min ago'
	},
	{
		type: 'Notifications',
		name: 'Telegram',
		detail: 'chat “Ops”',
		health: 'up' as Health,
		required: false,
		validated: '1 d ago'
	},
	{
		type: 'Notifications',
		name: 'Email',
		detail: 'smtp.example.com:587 STARTTLS',
		health: 'unknown' as Health,
		required: false,
		validated: 'never'
	},
	{
		type: 'CAPTCHA',
		name: 'Cap',
		detail: 'https://cap.example.com',
		health: 'up' as Health,
		required: false,
		validated: '1 h ago'
	},
	{
		type: 'GeoIP',
		name: 'MaxMind GeoLite2',
		detail: 'Country + ASN · 6 d old',
		health: 'up' as Health,
		required: false,
		validated: '6 d ago'
	}
];

export const audit = [
	{
		t: '13:42',
		who: 'admin',
		what: 'Saved resource Photos',
		sub: 'Resources',
		result: 'plan p1 created'
	},
	{ t: '13:28', who: 'system', what: 'Applied DNS plan (2 items)', sub: 'DNS', result: 'verified' },
	{
		t: '13:28',
		who: 'system',
		what: 'Agent nas changed IP 10.20.1.49 → 10.20.1.50',
		sub: 'Pulse',
		result: 'DNS + Traefik sync queued'
	},
	{
		t: '12:59',
		who: 'admin',
		what: 'Extended block 192.0.2.201 to 7 d',
		sub: 'Security',
		result: 'firewall plan applied on 1 host'
	},
	{
		t: '12:10',
		who: 'admin',
		what: 'Signed in with passkey “MacBook”',
		sub: 'Auth',
		result: 'session bound to 192.0.2.10'
	},
	{
		t: '04:00',
		who: 'scheduler',
		what: 'Ran script Rotate Docker logs',
		sub: 'Scripts',
		result: 'succeeded on 2 hosts'
	}
];

export const jobs = [
	{
		kind: 'Global reconcile',
		last: '13:00 · 41 s',
		next: '14:00',
		status: 'succeeded',
		summary: 'no changes'
	},
	{
		kind: 'Monitor checks',
		last: '13:42 · 1.2 s',
		next: 'continuous',
		status: 'succeeded',
		summary: '23 endpoints'
	},
	{
		kind: 'Access-log ingestion',
		last: '13:42 · 0.8 s',
		next: '13:43',
		status: 'succeeded',
		summary: '18,204 lines'
	},
	{
		kind: 'Blocklist fetch',
		last: '11:42 · 3.1 s',
		next: '17:42',
		status: 'degraded',
		summary: 'FireHOL Level 1: HTTP 503, kept last good'
	},
	{
		kind: 'Traefik update check',
		last: '06:00 · 0.9 s',
		next: 'tomorrow 06:00',
		status: 'succeeded',
		summary: '3.7.18 available'
	},
	{
		kind: 'Backup',
		last: '02:00 · 2.4 s',
		next: 'tomorrow 02:00',
		status: 'succeeded',
		summary: 'hashi-2026-10-27.db, 14 kept'
	},
	{
		kind: 'Retention',
		last: '02:10 · 6.0 s',
		next: 'tomorrow 02:10',
		status: 'succeeded',
		summary: '41,200 rows removed'
	}
];
