// Mock data for the security-model pages (ADR 0002, 05a, 06). Same rules as mock.ts: example.com names and
// documentation IP ranges only. Shapes follow 06a loosely; feature slices replace this with the generated client.

export type SetKind = 'user' | 'system' | 'feed' | 'dynamic';
export type ItemType = 'ip' | 'cidr' | 'pulse' | 'host' | 'dns' | 'geo' | 'set';
export type Scope = 'public' | 'internal' | 'all';
export type Action = 'none' | 'deny' | 'pass_captcha' | 'pass_authentication';

export type IpItem = {
	id: string;
	type: ItemType;
	/** ip/cidr: the literal; pulse/host: the agent or host name; dns: FQDN; geo: code; set: the set slug */
	value: string;
	scope?: Scope;
	geoKind?: 'country' | 'region' | 'asn';
	label?: string;
	resolved?: string[];
	note?: string;
	expires?: string;
	problem?: string;
	added: string;
};

export type DynamicEntry = {
	address: string;
	source: 'L7 rate' | 'WAF' | 'challenge failed' | 'L4 meter' | 'manual';
	reason: string;
	created: string;
	expires: string;
	failures?: number;
};

export type Consumer = { position: string; label: string; href: string; kind: 'match' | 'target' };

export type IpSet = {
	id: string;
	slug: string;
	description: string;
	kind: SetKind;
	locked: boolean;
	v4: number;
	v6: number;
	status: 'ok' | 'partial' | 'empty';
	problems?: string[];
	addressSet: boolean;
	items: IpItem[];
	entries?: DynamicEntry[];
	usedBy: Consumer[];
	feed?: {
		url: string;
		format: string;
		refresh: string;
		lastSuccess: string;
		fp: string;
		runs: FeedRun[];
	};
	changed: string;
};

export type FeedRun = {
	at: string;
	status: 'ok' | 'failed';
	parsed: number;
	added: number;
	removed: number;
	note?: string;
};

const fw = (label: string, position: string): Consumer => ({
	position,
	label,
	href: '/design/firewall',
	kind: 'match'
});
const rule = (label: string, href: string): Consumer => ({
	position: 'Rule applies to',
	label,
	href,
	kind: 'match'
});

export const ipSets: IpSet[] = [
	{
		id: 's-trusted',
		slug: 'trusted-devices',
		description: 'My phone and laptop, wherever they are, plus the office',
		kind: 'user',
		locked: false,
		v4: 4,
		v6: 2,
		status: 'ok',
		addressSet: false,
		changed: '3 d ago',
		items: [
			{
				id: 'i1',
				type: 'pulse',
				value: 'phone',
				scope: 'public',
				resolved: ['203.0.113.54', '2001:db8:54::1'],
				note: 'Follows the phone on mobile data and Wi-Fi',
				added: '2026-09-02'
			},
			{
				id: 'i2',
				type: 'pulse',
				value: 'laptop',
				scope: 'public',
				resolved: ['198.51.100.61'],
				added: '2026-09-02'
			},
			{ id: 'i3', type: 'set', value: 'office', added: '2026-09-10' },
			{
				id: 'i4',
				type: 'ip',
				value: '198.51.100.200',
				note: 'Uptime checker',
				expires: '2026-12-31',
				added: '2026-10-01'
			},
			{
				id: 'i5',
				type: 'dns',
				value: 'vpn.example.org',
				resolved: ['2001:db8:77::10'],
				note: 'Friend’s VPN exit',
				added: '2026-10-12'
			}
		],
		usedBy: [
			fw('Global firewall allow', 'Firewall allow'),
			rule('Photos · priority 1', '/design/services/r4'),
			rule('home-only · priority 1', '/design/rule-sets/rs-home')
		]
	},
	{
		id: 's-office',
		slug: 'office',
		description: 'Office network',
		kind: 'user',
		locked: false,
		v4: 1,
		v6: 0,
		status: 'ok',
		addressSet: false,
		changed: '1 month ago',
		items: [{ id: 'i6', type: 'cidr', value: '192.0.2.0/24', added: '2026-09-10' }],
		usedBy: [
			{
				position: 'Included in',
				label: 'trusted-devices',
				href: '/design/ip-sets/s-trusted',
				kind: 'match'
			}
		]
	},
	{
		id: 's-homewan',
		slug: 'home-wan',
		description: 'The home router’s public address, reported by Pulse',
		kind: 'user',
		locked: false,
		v4: 1,
		v6: 1,
		status: 'ok',
		addressSet: true,
		changed: '2 weeks ago',
		items: [
			{
				id: 'i7',
				type: 'pulse',
				value: 'home-router',
				scope: 'public',
				resolved: ['203.0.113.88', '2001:db8:88::1'],
				added: '2026-09-02'
			}
		],
		usedBy: [
			{
				position: 'DNS value',
				label: 'home.example.com A/AAAA',
				href: '/design/dns',
				kind: 'target'
			},
			{
				position: 'Connection target',
				label: 'edge-home SSH',
				href: '/design/connections',
				kind: 'target'
			},
			fw('Host access on edge-fra', 'Host access')
		]
	},
	{
		id: 's-media',
		slug: 'media-upstream',
		description: 'Where the media server runs; follows the home Docker host',
		kind: 'user',
		locked: false,
		v4: 1,
		v6: 0,
		status: 'ok',
		addressSet: true,
		changed: '2 weeks ago',
		items: [
			{
				id: 'i8',
				type: 'pulse',
				value: 'home-docker',
				scope: 'internal',
				label: 'selector eth0',
				resolved: ['10.20.1.102'],
				added: '2026-09-04'
			}
		],
		usedBy: [
			{ position: 'Service upstream', label: 'Media', href: '/design/services/r3', kind: 'target' },
			{ position: 'Service upstream', label: 'Photos', href: '/design/services/r4', kind: 'target' }
		]
	},
	{
		id: 's-ch',
		slug: 'switzerland',
		description: 'Requests from Switzerland skip the CAPTCHA on some services',
		kind: 'user',
		locked: false,
		v4: 1918,
		v6: 711,
		status: 'ok',
		addressSet: false,
		changed: '1 month ago',
		items: [
			{
				id: 'i9',
				type: 'geo',
				geoKind: 'country',
				value: 'CH',
				label: 'Switzerland',
				added: '2026-09-12'
			}
		],
		usedBy: [rule('Media · priority 1', '/design/services/r3')]
	},
	{
		id: 's-dc',
		slug: 'datacenters',
		description: 'Hosting and VPN networks; they get a CAPTCHA instead of a block',
		kind: 'user',
		locked: false,
		v4: 6204,
		v6: 1880,
		status: 'partial',
		problems: ['feed-dc-ranges: last fetch failed (HTTP 503), keeping the copy from 9 h ago'],
		addressSet: false,
		changed: '5 d ago',
		items: [
			{
				id: 'i10',
				type: 'geo',
				geoKind: 'asn',
				value: 'AS64500',
				label: 'Example Hosting',
				added: '2026-10-20'
			},
			{
				id: 'i11',
				type: 'geo',
				geoKind: 'asn',
				value: 'AS64501',
				label: 'Cloud Example',
				added: '2026-10-20'
			},
			{
				id: 'i12',
				type: 'set',
				value: 'feed-dc-ranges',
				problem: 'Feed stale',
				added: '2026-10-21'
			}
		],
		usedBy: [rule('global · priority 2', '/design/rule-sets/rs-global')]
	},
	{
		id: 's-all',
		slug: 'all',
		description: 'Every address: 0.0.0.0/0 and ::/0',
		kind: 'system',
		locked: true,
		v4: 1,
		v6: 1,
		status: 'ok',
		addressSet: false,
		changed: '—',
		items: [
			{ id: 'i13', type: 'cidr', value: '0.0.0.0/0', added: 'system' },
			{ id: 'i14', type: 'cidr', value: '::/0', added: 'system' }
		],
		usedBy: [
			rule('Every service fallback', '/design/rule-sets'),
			rule('admin-protect · priority 1', '/design/rule-sets/rs-admin')
		]
	},
	{
		id: 's-hosts',
		slug: 'hashi-hosts',
		description: 'Public addresses of every firewall and edge host, kept up to date by Hashi',
		kind: 'system',
		locked: true,
		v4: 2,
		v6: 1,
		status: 'ok',
		addressSet: true,
		changed: 'automatic',
		items: [
			{
				id: 'i15',
				type: 'host',
				value: 'edge-fra',
				scope: 'public',
				resolved: ['198.51.100.24', '2001:db8:24::1'],
				added: 'system'
			},
			{
				id: 'i16',
				type: 'host',
				value: 'edge-home',
				scope: 'public',
				resolved: ['203.0.113.88'],
				added: 'system'
			}
		],
		usedBy: [
			fw('Global firewall allow', 'Firewall allow'),
			fw('Host access on edge-fra', 'Host access')
		]
	},
	{
		id: 's-private',
		slug: 'private-networks',
		description: 'RFC 1918, RFC 6598, loopback, link-local and ULA ranges',
		kind: 'system',
		locked: true,
		v4: 6,
		v6: 3,
		status: 'ok',
		addressSet: false,
		changed: '—',
		items: [
			{ id: 'i17', type: 'cidr', value: '10.0.0.0/8', added: 'system' },
			{ id: 'i18', type: 'cidr', value: '172.16.0.0/12', added: 'system' },
			{ id: 'i19', type: 'cidr', value: '192.168.0.0/16', added: 'system' },
			{ id: 'i20', type: 'cidr', value: '100.64.0.0/10', added: 'system' },
			{ id: 'i21', type: 'cidr', value: 'fc00::/7', added: 'system' }
		],
		usedBy: []
	},
	{
		id: 's-drop',
		slug: 'feed-spamhaus-drop',
		description: 'Spamhaus DROP: hijacked and criminal networks',
		kind: 'feed',
		locked: true,
		v4: 1192,
		v6: 0,
		status: 'ok',
		addressSet: false,
		changed: '2 h ago',
		items: [
			{ id: 'f1', type: 'cidr', value: '192.0.2.128/25', note: 'line 14', added: 'feed' },
			{ id: 'f2', type: 'cidr', value: '198.51.100.0/26', note: 'line 15', added: 'feed' },
			{ id: 'f3', type: 'cidr', value: '203.0.113.192/27', note: 'line 16', added: 'feed' }
		],
		usedBy: [fw('Global firewall block', 'Firewall block')],
		feed: {
			url: 'https://www.spamhaus.org/drop/drop.txt',
			format: 'Plain CIDR lines with ; comments',
			refresh: 'every 6 h',
			lastSuccess: '2 h ago',
			fp: 'very low',
			runs: [
				{ at: 'today 12:00', status: 'ok', parsed: 1192, added: 2, removed: 1 },
				{
					at: 'today 06:00',
					status: 'ok',
					parsed: 1191,
					added: 0,
					removed: 0,
					note: 'not modified (ETag)'
				},
				{ at: 'yesterday 24:00', status: 'ok', parsed: 1191, added: 4, removed: 3 }
			]
		}
	},
	{
		id: 's-feodo',
		slug: 'feed-feodo',
		description: 'Feodo Tracker botnet command-and-control servers',
		kind: 'feed',
		locked: true,
		v4: 311,
		v6: 0,
		status: 'ok',
		addressSet: false,
		changed: '41 min ago',
		items: [],
		usedBy: [fw('Global firewall block', 'Firewall block')]
	},
	{
		id: 's-dcfeed',
		slug: 'feed-dc-ranges',
		description: 'Custom URL: published hosting and VPN ranges',
		kind: 'feed',
		locked: true,
		v4: 5312,
		v6: 1877,
		status: 'partial',
		problems: [
			'Last fetch failed (HTTP 503). Keeping the copy from 9 h ago; empty after 72 h without success.'
		],
		addressSet: false,
		changed: '9 h ago',
		items: [],
		usedBy: [
			{ position: 'Included in', label: 'datacenters', href: '/design/ip-sets/s-dc', kind: 'match' }
		]
	},
	{
		id: 's-challenge',
		slug: 'dynamic-challenge',
		description: 'Addresses that must solve a CAPTCHA before anything else. Filled by the engine.',
		kind: 'dynamic',
		locked: true,
		v4: 3,
		v6: 1,
		status: 'ok',
		addressSet: false,
		changed: 'live',
		items: [],
		entries: [
			{
				address: '203.0.113.9',
				source: 'L7 rate',
				reason: '412 requests in 10 s',
				created: '13:41',
				expires: 'in 26 min',
				failures: 11
			},
			{
				address: '198.51.100.12',
				source: 'WAF',
				reason: '5 matches in 60 s (930120)',
				created: '13:38',
				expires: 'in 23 min',
				failures: 2
			},
			{
				address: '2001:db8:aa::/64',
				source: 'L7 rate',
				reason: '388 requests in 10 s',
				created: '13:30',
				expires: 'in 15 min',
				failures: 0
			},
			{
				address: '192.0.2.77',
				source: 'manual',
				reason: 'Challenge now',
				created: '12:58',
				expires: 'in 1 h',
				failures: 0
			}
		],
		usedBy: []
	},
	{
		id: 's-blocked',
		slug: 'dynamic-blocked',
		description: 'Banned addresses, dropped on every firewall host. Filled by the engine.',
		kind: 'dynamic',
		locked: true,
		v4: 4,
		v6: 0,
		status: 'ok',
		addressSet: false,
		changed: 'live',
		items: [],
		entries: [
			{
				address: '192.0.2.201',
				source: 'challenge failed',
				reason: '15 requests without solving',
				created: '13:37',
				expires: 'in 3 h 58 min'
			},
			{
				address: '198.51.100.77',
				source: 'L4 meter',
				reason: 'edge-fra: over 250 open TCP connections',
				created: '13:02',
				expires: 'in 1 d 23 h'
			},
			{
				address: '192.0.2.18',
				source: 'challenge failed',
				reason: '15 requests without solving',
				created: '11:20',
				expires: 'in 44 min'
			},
			{
				address: '203.0.113.240',
				source: 'manual',
				reason: 'Ban now · scanner',
				created: 'yesterday',
				expires: 'permanent'
			}
		],
		usedBy: [fw('Firewall dyn set (via host link)', 'Kernel set')]
	}
];

export const setBySlug = (slug: string) => ipSets.find((s) => s.slug === slug);

// Ban and challenge counters (06 §5.1). Values are effective (step decay already applied).
export type CounterRow = {
	address: string;
	country: string;
	asn: string;
	ban: number;
	challenge: number;
	state: 'banned' | 'challenged' | 'none';
	stateDetail: string;
	lastBan: string;
	lastChallenge: string;
	nextDecrement: string;
	source: DynamicEntry['source'];
};

export const counters: CounterRow[] = [
	{
		address: '192.0.2.201',
		country: 'NL',
		asn: 'AS64500 Example Hosting',
		ban: 4,
		challenge: 6,
		state: 'banned',
		stateDetail: 'until 17:35 (4 h)',
		lastBan: '13:37',
		lastChallenge: '13:31',
		nextDecrement: '7 d after the ban ends',
		source: 'challenge failed'
	},
	{
		address: '198.51.100.77',
		country: 'US',
		asn: 'AS64501 Cloud Example',
		ban: 8,
		challenge: 2,
		state: 'banned',
		stateDetail: 'until Fri 13:02 (2 d)',
		lastBan: '13:02',
		lastChallenge: '3 d ago',
		nextDecrement: '7 d after the ban ends',
		source: 'L4 meter'
	},
	{
		address: '203.0.113.9',
		country: 'DE',
		asn: 'AS64502 Example DSL',
		ban: 1,
		challenge: 3,
		state: 'challenged',
		stateDetail: '11 of 15 · expires in 26 min',
		lastBan: '9 d ago',
		lastChallenge: '13:41',
		nextDecrement: 'ban: in 5 d · challenge: in 7 d',
		source: 'L7 rate'
	},
	{
		address: '198.51.100.12',
		country: 'CH',
		asn: 'AS64505 Example Mobile CH',
		ban: 0,
		challenge: 1,
		state: 'challenged',
		stateDetail: '2 of 15 · expires in 23 min',
		lastBan: '—',
		lastChallenge: '13:38',
		nextDecrement: 'challenge: in 7 d',
		source: 'WAF'
	},
	{
		address: '2001:db8:aa::/64',
		country: 'FR',
		asn: 'AS64504 Example Mobile',
		ban: 0,
		challenge: 2,
		state: 'challenged',
		stateDetail: '0 of 15 · expires in 15 min',
		lastBan: '—',
		lastChallenge: '13:30',
		nextDecrement: 'challenge: in 7 d',
		source: 'L7 rate'
	},
	{
		address: '192.0.2.18',
		country: 'SG',
		asn: 'AS64503 Example Transit',
		ban: 2,
		challenge: 2,
		state: 'banned',
		stateDetail: 'until 14:24 (44 min)',
		lastBan: '11:20',
		lastChallenge: '11:12',
		nextDecrement: '7 d after the ban ends',
		source: 'challenge failed'
	},
	{
		address: '203.0.113.240',
		country: 'BR',
		asn: 'AS64506 Example Telecom',
		ban: 1,
		challenge: 0,
		state: 'banned',
		stateDetail: 'permanent',
		lastBan: 'yesterday',
		lastChallenge: '—',
		nextDecrement: 'never (permanent)',
		source: 'manual'
	},
	{
		address: '192.0.2.150',
		country: 'NL',
		asn: 'AS64500 Example Hosting',
		ban: 3,
		challenge: 3,
		state: 'none',
		stateDetail: '—',
		lastBan: '6 d ago',
		lastChallenge: '6 d ago',
		nextDecrement: 'ban: tomorrow 09:10',
		source: 'L7 rate'
	},
	{
		address: '198.51.100.140',
		country: 'US',
		asn: 'AS64501 Cloud Example',
		ban: 0,
		challenge: 4,
		state: 'none',
		stateDetail: '—',
		lastBan: '—',
		lastChallenge: '2 d ago',
		nextDecrement: 'challenge: in 5 d',
		source: 'WAF'
	},
	{
		address: '203.0.113.31',
		country: 'DE',
		asn: 'AS64502 Example DSL',
		ban: 1,
		challenge: 1,
		state: 'none',
		stateDetail: '—',
		lastBan: '12 d ago',
		lastChallenge: '12 d ago',
		nextDecrement: 'ban: in 2 d',
		source: 'L4 meter'
	}
];

// Rule sets (06 §4). Entries reference IP sets by slug and rule sets by id.
export type RuleItem = {
	kind: 'item';
	priority: number;
	appliesTo: string;
	condition?: { type: 'exact' | 'prefix' | 'regex'; pattern: string };
	action: Action;
	auth?: { provider?: string; groups?: string[] };
	enabled: boolean;
	note?: string;
	fallback?: boolean;
};
export type RuleRef = { kind: 'ref'; priority: number; ruleSet: string };
export type RuleEntry = RuleItem | RuleRef;

export type RuleSet = {
	id: string;
	slug: string;
	description: string;
	kind: 'global' | 'user' | 'service_root';
	entries: RuleEntry[];
	usedBy: { label: string; href: string }[];
	warnings: string[];
	changed: string;
};

export const ruleSets: RuleSet[] = [
	{
		id: 'rs-global',
		slug: 'global',
		description: 'Applies to every service, after the service’s own rules',
		kind: 'global',
		entries: [
			{
				kind: 'item',
				priority: 1,
				appliesTo: 'feed-dc-ranges',
				action: 'pass_captcha',
				enabled: false,
				note: 'Replaced by datacenters (priority 2)'
			},
			{
				kind: 'item',
				priority: 2,
				appliesTo: 'datacenters',
				action: 'pass_captcha',
				enabled: true,
				note: 'Hosting and VPN addresses solve a CAPTCHA; signed-in users pass'
			}
		],
		usedBy: [{ label: 'Every HTTP service (10)', href: '/design/services' }],
		warnings: ['datacenters is partial: feed-dc-ranges is stale'],
		changed: '5 d ago'
	},
	{
		id: 'rs-admin',
		slug: 'admin-protect',
		description: 'Sign-in for /admin, for admins only',
		kind: 'user',
		entries: [
			{
				kind: 'item',
				priority: 1,
				appliesTo: 'all',
				condition: { type: 'prefix', pattern: '/admin' },
				action: 'pass_authentication',
				auth: { groups: ['admins'] },
				enabled: true
			}
		],
		usedBy: [
			{ label: 'Gitea · priority 2', href: '/design/services/r2' },
			{ label: 'Search · priority 1', href: '/design/services/r7' }
		],
		warnings: [],
		changed: '2 weeks ago'
	},
	{
		id: 'rs-home',
		slug: 'home-only',
		description: 'My devices get in without anything; everyone else signs in',
		kind: 'user',
		entries: [
			{ kind: 'item', priority: 1, appliesTo: 'trusted-devices', action: 'none', enabled: true },
			{
				kind: 'item',
				priority: 2,
				appliesTo: 'all',
				action: 'pass_authentication',
				auth: { groups: ['family'] },
				enabled: true
			}
		],
		usedBy: [{ label: 'Home Assistant · priority 1', href: '/design/services/r5' }],
		warnings: [],
		changed: '1 month ago'
	},
	{
		id: 'rs-hooks',
		slug: 'webhooks',
		description: 'Let CI webhooks through without CAPTCHA or sign-in',
		kind: 'user',
		entries: [
			{
				kind: 'item',
				priority: 1,
				appliesTo: 'all',
				condition: { type: 'prefix', pattern: '/api/hooks' },
				action: 'none',
				enabled: true,
				note: 'Webhooks come from hosting networks'
			}
		],
		usedBy: [{ label: 'Gitea · priority 1', href: '/design/services/r2' }],
		warnings: [],
		changed: '1 month ago'
	}
];

export const ruleSetById = (id: string) => ruleSets.find((r) => r.id === id);

// Service root rule sets (one per HTTP service), keyed by service id. The fallback is always the last entry.
export const serviceRoots: Record<string, { entries: RuleEntry[]; fallback: 'none' | 'deny' }> = {
	r0: { entries: [], fallback: 'none' },
	r1: {
		entries: [
			{
				kind: 'item',
				priority: 1,
				appliesTo: 'all',
				action: 'pass_authentication',
				auth: { groups: ['admins'] },
				enabled: true
			}
		],
		fallback: 'deny'
	},
	r2: {
		entries: [
			{ kind: 'ref', priority: 1, ruleSet: 'rs-hooks' },
			{ kind: 'ref', priority: 2, ruleSet: 'rs-admin' }
		],
		fallback: 'none'
	},
	r3: {
		entries: [
			{ kind: 'item', priority: 1, appliesTo: 'switzerland', action: 'none', enabled: true },
			{ kind: 'item', priority: 2, appliesTo: 'all', action: 'pass_captcha', enabled: true }
		],
		fallback: 'none'
	},
	r4: {
		entries: [
			{ kind: 'item', priority: 1, appliesTo: 'trusted-devices', action: 'none', enabled: true },
			{
				kind: 'item',
				priority: 2,
				appliesTo: 'all',
				condition: { type: 'prefix', pattern: '/share' },
				action: 'none',
				enabled: true,
				note: 'Shared albums'
			}
		],
		fallback: 'deny'
	},
	r5: { entries: [{ kind: 'ref', priority: 1, ruleSet: 'rs-home' }], fallback: 'deny' },
	r6: {
		entries: [
			{
				kind: 'item',
				priority: 1,
				appliesTo: 'all',
				condition: { type: 'prefix', pattern: '/admin' },
				action: 'deny',
				enabled: true,
				note: 'Admin panel only over NetBird'
			}
		],
		fallback: 'none'
	},
	r7: { entries: [{ kind: 'ref', priority: 1, ruleSet: 'rs-admin' }], fallback: 'none' },
	r10: { entries: [], fallback: 'none' }
};

export type FlatRow = {
	stage: 'A' | 'B';
	label: string;
	appliesTo?: string;
	condition?: RuleItem['condition'];
	action?: Action;
	auth?: RuleItem['auth'];
	origin: string;
	locked: boolean;
	note?: string;
	enabled?: boolean;
};

export const stageA: FlatRow[] = [
	{
		stage: 'A',
		label: 'A1 Block guard',
		origin: 'engine',
		locked: true,
		note: 'Firewall block and dynamic-blocked → 403'
	},
	{
		stage: 'A',
		label: 'A2 WAF and rate trigger',
		origin: 'engine',
		locked: true,
		note: '5 WAF matches in 60 s or 300 requests in 10 s → dynamic-challenge'
	},
	{
		stage: 'A',
		label: 'A3 Challenge',
		origin: 'engine',
		locked: true,
		note: 'Challenged: CAPTCHA first; 15 requests without solving → ban'
	}
];

/** Pre-order flatten of a service: root (without fallback) + global + fallback (06 §4.3). */
export function flatten(serviceId: string): FlatRow[] {
	const root = serviceRoots[serviceId] ?? { entries: [], fallback: 'none' as const };
	const out: FlatRow[] = [];
	const walk = (entries: RuleEntry[], path: string) => {
		for (const e of [...entries].sort((a, b) => a.priority - b.priority)) {
			if (e.kind === 'ref') {
				const rs = ruleSetById(e.ruleSet);
				if (rs) walk(rs.entries, `${path} › ${rs.slug}`);
			} else if (e.enabled) {
				out.push({
					stage: 'B',
					label: '',
					appliesTo: e.appliesTo,
					condition: e.condition,
					action: e.action,
					auth: e.auth,
					origin: `${path} · ${e.priority}`,
					locked: false,
					note: e.note
				});
			}
		}
	};
	walk(root.entries, 'service');
	walk(ruleSets[0]!.entries, 'global');
	out.push({
		stage: 'B',
		label: '',
		appliesTo: 'all',
		action: root.fallback,
		origin: 'fallback',
		locked: true,
		note: root.fallback === 'deny' ? 'Closed service' : 'Open service'
	});
	return out;
}

export const actionMeta: Record<
	Action,
	{ label: string; tone: 'ok' | 'bad' | 'warn' | 'info'; help: string }
> = {
	none: {
		label: 'None',
		tone: 'ok',
		help: 'Let the request through; later CAPTCHA and sign-in rules are skipped'
	},
	deny: { label: 'Deny', tone: 'bad', help: 'Answer 403' },
	pass_captcha: {
		label: 'Pass CAPTCHA',
		tone: 'warn',
		help: 'Solve a CAPTCHA once per service (24 h); a signed-in user passes'
	},
	pass_authentication: {
		label: 'Pass Authentication',
		tone: 'info',
		help: 'Sign in through the edge SSO, optionally only some groups'
	}
};

export const firewallPolicy = {
	allow: ['trusted-devices', 'hashi-hosts'],
	block: ['feed-spamhaus-drop', 'feed-feodo'],
	folded: { allow: { v4: 7, v6: 3 }, block: { v4: 1503, v6: 0 }, carveOuts: 1 },
	warnings: [
		'198.51.100.0/26 (feed-spamhaus-drop) contains 198.51.100.61 (laptop in trusted-devices): the allow wins for that address only'
	]
};

const v4 = (a: string) => a.split('.').reduce((n, o) => n * 256 + Number(o), 0);

/** IPv4 prefix containment for the mocks (IPv6 compares literally). */
export function inCidr(address: string, cidr: string): boolean {
	const [net = '', bits = '32'] = cidr.split('/');
	if (!address.includes('.') || !net.includes('.')) return address === net;
	const size = 2 ** (32 - Number(bits));
	return Math.floor(v4(address) / size) === Math.floor(v4(net) / size);
}

/** Mock membership test: an item's literal or resolved address, a containing CIDR, a nested set or a dynamic entry. */
export function containsAddress(set: IpSet, address: string): boolean {
	if (set.slug === 'all') return true;
	const a = address.split('/')[0] ?? address;
	return (
		set.items.some(
			(i) =>
				i.value === a ||
				i.resolved?.includes(a) ||
				(i.type === 'cidr' && inCidr(a, i.value)) ||
				(i.type === 'set' && !!setBySlug(i.value) && containsAddress(setBySlug(i.value)!, a))
		) || !!set.entries?.some((e) => e.address === a)
	);
}
