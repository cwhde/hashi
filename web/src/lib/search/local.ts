// What the top bar search finds without asking the server (10b §2, bug 4): pages, the tabs of the top-level pages and
// every setting by its label. The server finds stored objects and their pages' tabs (/api/search).
// local.test.ts checks every tab and setting below against the page that shows it, so a renamed label breaks the test.

export type LocalHit = { title: string; detail: string; href: string; words?: string };
export type LocalGroup = { label: string; hits: LocalHit[] };

/** Every word of the query appears in the hit's text, case ignored (the server matches the same way). */
export function matches(q: string, text: string): boolean {
	const words = q.toLowerCase().split(/\s+/).filter(Boolean);
	const hay = text.toLowerCase();
	return words.length > 0 && words.every((w) => hay.includes(w));
}

/** The best hits first: title equal to the query, then starting with it, then the rest in their order. */
export function ranked<T extends { title: string }>(q: string, hits: T[]): T[] {
	const needle = q.trim().toLowerCase();
	const rank = (h: T) => {
		const t = h.title.toLowerCase();
		return t === needle ? 0 : t.startsWith(needle) ? 1 : 2;
	};
	return hits
		.map((h, i) => ({ h, i }))
		.sort((a, b) => rank(a.h) - rank(b.h) || a.i - b.i)
		.map((x) => x.h);
}

// Top-level pages with tabs. The first tab is the page itself; the others open with ?tab=.
export const pageTabs: { page: string; href: string; tabs: { value: string; label: string }[] }[] =
	[
		{
			page: 'Security',
			href: '/security',
			tabs: [{ value: 'counters', label: 'Counters' }]
		},
		{
			page: 'DNS',
			href: '/dns',
			tabs: [
				{ value: 'import', label: 'Import' },
				{ value: 'adguard', label: 'AdGuard rewrites' },
				{ value: 'agents', label: 'Internal agent names' }
			]
		},
		{
			page: 'Traefik',
			href: '/traefik',
			tabs: [
				{ value: 'middlewares', label: 'Your middlewares' },
				{ value: 'certificates', label: 'Certificates' }
			]
		},
		{
			page: 'Activity',
			href: '/activity',
			tabs: [
				{ value: 'jobs', label: 'Jobs' },
				{ value: 'alerts', label: 'Alerts sent' },
				{ value: 'log', label: 'Live log' }
			]
		}
	];

// Settings by the label each shows (10c §1.2), with the words people may search for instead. `find` scrolls Settings to
// the label (lib/find.ts); `component` is the file that shows it, which the test reads.
export type SettingEntry = { label: string; category: string; component: string; words?: string };

export const settingCategories: Record<string, string> = {
	general: 'General',
	security: 'Security',
	edge: 'Edge protection',
	traefik: 'Services and Traefik',
	dns: 'DNS and agent names',
	firewall: 'Firewall',
	pulse: 'Pulse',
	monitoring: 'Monitoring and notifications',
	public: 'Public pages',
	appearance: 'Appearance'
};

const general = 'settings/GeneralSettings';
const security = 'settings/SecuritySettings';
const vault = 'VaultPanel';
const sessions = 'settings/EdgeSessions';
const cap = 'security/ChallengeSettings';
const engine = 'security/EngineSummary';
const notify = 'status/Notifications';

export const settings: SettingEntry[] = [
	{ label: 'Root domain', category: 'general', component: general, words: 'domain' },
	{ label: 'Admin domain', category: 'general', component: general, words: 'hostname address' },
	{ label: 'Internal Hashi URL', category: 'general', component: general },
	{
		label: 'Reconcile every (minutes)',
		category: 'general',
		component: general,
		words: 'sync interval'
	},
	{
		label: 'Show times in',
		category: 'general',
		component: general,
		words: 'timezone utc local time'
	},
	{ label: 'Passkeys', category: 'security', component: security, words: 'webauthn sign-in' },
	{ label: 'Active sessions', category: 'security', component: security, words: 'sign out' },
	{
		label: 'Sign out after inactivity (minutes)',
		category: 'security',
		component: security,
		words: 'idle timeout session'
	},
	{ label: 'Recovery links work from', category: 'security', component: security },
	{
		label: 'Believe forwarded addresses from',
		category: 'security',
		component: security,
		words: 'trusted proxies x-forwarded-for'
	},
	{
		label: 'Vault',
		category: 'security',
		component: vault,
		words: 'secrets strict service mode master key'
	},
	{
		label: 'Request limit',
		category: 'edge',
		component: engine,
		words: 'rate abuse engine challenge'
	},
	{ label: 'Error answers', category: 'edge', component: engine, words: '4xx scanners trigger' },
	{ label: 'Web firewall matches', category: 'edge', component: engine, words: 'waf' },
	{ label: 'Challenge expires after', category: 'edge', component: engine },
	{ label: 'Attempts before a ban', category: 'edge', component: engine, words: 'max failed' },
	{ label: 'CAPTCHA pass lasts', category: 'edge', component: engine, words: 'clearance' },
	{
		label: 'Forget one offence after',
		category: 'edge',
		component: engine,
		words: 'decay counters'
	},
	{ label: 'Default web firewall', category: 'edge', component: engine, words: 'waf mode' },
	{
		label: 'Connection limits',
		category: 'edge',
		component: engine,
		words: 'meters tcp new connections per minute open'
	},
	{ label: 'Blocklists refresh every', category: 'edge', component: engine, words: 'feeds' },
	{ label: 'Ban length', category: 'edge', component: engine, words: 'bans grow' },
	{ label: 'Challenge visitors with a CAPTCHA', category: 'edge', component: cap, words: 'cap' },
	{ label: 'Challenge page', category: 'edge', component: cap, words: 'cap captcha' },
	{ label: 'Cap dashboard service', category: 'edge', component: cap },
	{ label: 'Site key', category: 'edge', component: cap, words: 'cap' },
	{ label: 'Secret key', category: 'edge', component: cap, words: 'cap' },
	{
		label: 'Longest session (hours)',
		category: 'edge',
		component: sessions,
		words: 'edge sign-in sso'
	},
	{
		label: 'Ends when unused for (hours)',
		category: 'edge',
		component: sessions,
		words: 'edge sign-in idle'
	},
	{
		label: 'Stay signed in after closing the browser',
		category: 'edge',
		component: sessions,
		words: 'remember device edge sign-in'
	},
	{
		label: 'Middlewares on every service',
		category: 'traefik',
		component: 'settings/TraefikSettings',
		words: 'traefik default'
	},
	{
		label: 'Create names for agents',
		category: 'dns',
		component: 'pulse/AgentDns',
		words: 'agent dns'
	},
	{ label: 'Internal domain', category: 'dns', component: 'pulse/AgentDns', words: 'agent dns' },
	{
		label: 'Answer with',
		category: 'dns',
		component: 'pulse/AgentDns',
		words: 'agent dns address'
	},
	{
		label: "Remove a stale agent's name after (hours)",
		category: 'dns',
		component: 'pulse/AgentDns',
		words: 'agent dns'
	},
	{
		label: 'When Hashi finds NetBird on a new host',
		category: 'firewall',
		component: 'settings/FirewallSettings'
	},
	{
		label: 'Fit TCP packets to narrow networks',
		category: 'firewall',
		component: 'settings/FirewallSettings',
		words: 'mss clamp packet size'
	},
	{
		label: 'Heartbeat every (seconds)',
		category: 'pulse',
		component: 'pulse/PulseSettings',
		words: 'agent'
	},
	{
		label: 'Stale after this many missed heartbeats',
		category: 'pulse',
		component: 'pulse/PulseSettings',
		words: 'agent'
	},
	{
		label: 'Update agents automatically',
		category: 'pulse',
		component: 'pulse/PulseSettings',
		words: 'self-update'
	},
	{
		label: 'Healthy HTTP codes',
		category: 'monitoring',
		component: 'settings/MonitoringSettings',
		words: 'status check'
	},
	{
		label: 'Check every (seconds)',
		category: 'monitoring',
		component: 'settings/MonitoringSettings',
		words: 'interval'
	},
	{
		label: 'Give up after (seconds)',
		category: 'monitoring',
		component: 'settings/MonitoringSettings',
		words: 'timeout'
	},
	{
		label: 'Slow above (ms)',
		category: 'monitoring',
		component: 'settings/MonitoringSettings',
		words: 'degraded latency'
	},
	{
		label: 'Keep the up/down history for (days)',
		category: 'monitoring',
		component: 'settings/MonitoringSettings',
		words: 'retention'
	},
	{ label: 'Where alerts go', category: 'monitoring', component: notify, words: 'notifications' },
	{ label: 'Alert when', category: 'monitoring', component: notify, words: 'notifications' },
	{
		label: 'Send recovery messages',
		category: 'monitoring',
		component: notify,
		words: 'notifications'
	},
	{
		label: 'Time between alerts for one check',
		category: 'monitoring',
		component: notify,
		words: 'cooldown notifications'
	},
	{
		label: 'Group alerts within',
		category: 'monitoring',
		component: notify,
		words: 'notifications'
	},
	{
		label: 'Providers',
		category: 'monitoring',
		component: notify,
		words: 'telegram email smtp notifications'
	},
	{ label: 'Public app dashboard', category: 'public', component: 'settings/PublicPages' },
	{ label: 'Public status page', category: 'public', component: 'settings/PublicPages' },
	{
		label: 'Visitors first see the apps sorted by',
		category: 'public',
		component: 'settings/PublicPages',
		words: 'order sort dashboard'
	},
	{
		label: 'New services show on the dashboard',
		category: 'public',
		component: 'settings/PublicPages'
	},
	{
		label: 'Show categories',
		category: 'public',
		component: 'settings/PublicPages',
		words: 'dashboard'
	},
	{
		label: 'Theme',
		category: 'appearance',
		component: 'settings/AppearanceSettings',
		words: 'colours colors dark light'
	},
	{ label: 'Overview widgets', category: 'appearance', component: 'settings/AppearanceSettings' },
	{ label: 'Reduce transparency', category: 'appearance', component: 'settings/AppearanceSettings' }
];

export const settingHref = (s: SettingEntry) =>
	`/settings?category=${s.category}&find=${encodeURIComponent(s.label)}`;

/** The local groups that match q: pages from the navigation, tabs, settings. */
export function localSearch(q: string, pages: { label: string; href: string }[]): LocalGroup[] {
	const pick = (hits: LocalHit[]) =>
		ranked(
			q,
			hits.filter((h) => matches(q, `${h.title} ${h.detail} ${h.words ?? ''}`))
		).slice(0, 8);
	const groups: LocalGroup[] = [
		{
			label: 'Pages',
			hits: pick(pages.map((p) => ({ title: p.label, detail: 'Page', href: p.href })))
		},
		{
			label: 'Tabs',
			hits: pick(
				pageTabs.flatMap((p) =>
					p.tabs.map((t) => ({
						title: t.label,
						detail: p.page,
						href: `${p.href}?tab=${t.value}`
					}))
				)
			)
		},
		{
			label: 'Settings',
			hits: pick([
				...Object.entries(settingCategories).map(([id, label]) => ({
					title: label,
					detail: 'Settings',
					href: `/settings?category=${id}`
				})),
				...settings.map((s) => ({
					title: s.label,
					detail: `Settings · ${settingCategories[s.category]}`,
					href: settingHref(s),
					words: s.words
				}))
			])
		}
	];
	return groups.filter((g) => g.hits.length > 0);
}
