// Fake data for the 13.x idea mocks (layout editor, public status groups, service detail, badges). Not approved mocks:
// rough sketches to choose a direction from, removed once one is picked.
import type { Health } from '#lib/design/mock.ts';
import { spark } from '#lib/design/mock.ts';

export type Entry = {
	id: string;
	name: string;
	group: string;
	kind: 'web' | 'udp' | 'tcp' | 'link';
	apps: boolean; // shown on the apps page
	status: boolean; // shown on the status page
	check: boolean; // has an uptime check (UDP and plain links may have none)
	state: Health;
	uptime: { h24: number; d7: number; d30: number; d90: number };
	ms: number;
	history: Health[];
	days: Health[]; // 90 days, one per day
	letter: string;
	hue: number;
};

function strip(seed: number, n: number, down: number[] = [], slow: number[] = []): Health[] {
	return Array.from({ length: n }, (_, i) =>
		down.includes(i)
			? 'down'
			: slow.includes(i)
				? 'degraded'
				: (seed * (i + 7)) % 89 === 0
					? 'degraded'
					: 'up'
	);
}

const raw: [string, string, Entry['kind'], boolean, boolean, boolean, Health?][] = [
	['Jellyfin', 'Media', 'web', true, true, true],
	['Jellyseerr', 'Media', 'web', true, true, true],
	['Immich', 'Media', 'web', true, true, true, 'degraded'],
	['Audiobookshelf', 'Media', 'web', true, true, true],
	['Navidrome', 'Media', 'web', true, true, true],
	['Sonarr', 'Media', 'web', true, false, true],
	['Radarr', 'Media', 'web', true, false, true],
	['Prowlarr', 'Media', 'web', true, false, true],
	['Bazarr', 'Media', 'web', true, false, true],
	['qBittorrent', 'Media', 'web', true, false, true],
	['Gitea', 'Development', 'web', true, true, true],
	['Arcane', 'Development', 'web', true, false, true],
	['Docs', 'Development', 'link', true, false, false],
	['Home Assistant', 'Home', 'web', true, true, true, 'down'],
	['AdGuard', 'Home', 'web', true, true, true],
	['Minecraft', 'Games', 'udp', false, true, true],
	['Satisfactory', 'Games', 'udp', false, false, false],
	['Vaultwarden', 'Security', 'web', true, true, true],
	['Pocket ID', 'Security', 'web', true, true, true],
	['Grafana', '', 'web', true, true, true],
	['SearXNG', '', 'web', true, false, true],
	['Nezha', '', 'web', false, true, true]
];

export const entries: Entry[] = raw.map(([name, group, kind, apps, status, check, state], i) => {
	const s: Health = !check ? 'unknown' : (state ?? 'up');
	return {
		id: `e${i}`,
		name,
		group,
		kind,
		apps,
		status,
		check,
		state: s,
		uptime: {
			h24: s === 'down' ? 97.92 : s === 'degraded' ? 99.31 : 100,
			d7: s === 'down' ? 99.4 : 99.97,
			d30: 99.9 - (i % 4) * 0.07,
			d90: 99.8 - (i % 5) * 0.11
		},
		ms: 30 + ((i * 37) % 180),
		history: strip(
			i + 3,
			60,
			s === 'down' ? [54, 55, 56, 57, 58, 59] : [],
			s === 'degraded' ? [40, 41, 47, 52] : []
		),
		days: strip(i + 5, 90, i % 6 === 0 ? [61] : [], i % 4 === 0 ? [12, 77] : []),
		letter: name[0]!,
		hue: (i * 47) % 360
	};
});

/** Group order as the owner set it; '' is "Other", always last. */
export const groupOrder = ['Media', 'Home', 'Development', 'Security', 'Games'];

export function grouped(list: Entry[], order = groupOrder) {
	const names = [
		...order,
		...new Set(list.map((e) => e.group).filter((g) => g && !order.includes(g)))
	];
	const out = names.map((g) => ({ name: g, items: list.filter((e) => e.group === g) }));
	const other = list.filter((e) => !e.group);
	if (other.length) out.push({ name: 'Other', items: other });
	return out.filter((g) => g.items.length);
}

export const worst = (list: Entry[]): Health =>
	list.some((e) => e.state === 'down')
		? 'down'
		: list.some((e) => e.state === 'degraded')
			? 'degraded'
			: 'up';

export const responseSeries = (seed: number) => spark(seed, 48, 80, 50);

export const downtimes = [
	{ start: '20 Nov 19:55', end: '19:56', length: '1 min' },
	{ start: '19 Nov 09:23', end: '09:25', length: '2 min' },
	{ start: '12 Nov 03:10', end: '03:41', length: '31 min' }
];

/** Badges Hashi could serve, Gatus/Uptime Kuma style. */
export const badges = [
	{
		id: 'status',
		name: 'Status',
		about: 'Up, degraded or down, right now',
		label: 'status',
		value: 'up',
		tone: 'ok'
	},
	{
		id: 'uptime',
		name: 'Uptime',
		about: 'Share of checks that passed over the period',
		label: 'uptime 30d',
		value: '99.94%',
		tone: 'ok'
	},
	{
		id: 'response',
		name: 'Response time',
		about: 'Average response time over the period',
		label: 'response 24h',
		value: '84 ms',
		tone: 'ok'
	},
	{
		id: 'cert',
		name: 'Certificate',
		about: 'Days until the TLS certificate expires',
		label: 'certificate',
		value: '61 days',
		tone: 'ok'
	},
	{
		id: 'chart',
		name: 'Response chart',
		about: 'A small chart of the response time, for a README',
		label: '',
		value: '',
		tone: 'chart'
	}
] as const;
