import {
	LayoutDashboard,
	Globe,
	Network,
	Waypoints,
	BrickWall,
	Radio,
	Activity,
	AppWindow,
	ShieldCheck,
	SquareTerminal,
	History,
	Cable,
	Settings,
	Layers,
	ListOrdered
} from '@lucide/svelte';

// Navigation grouping decided in S0b (docs/spec/10b-ui-system.md §2). `href` is the /design mock route today and
// becomes the real route (without /design) when a feature slice fills the page.
export const nav = [
	{ items: [{ label: 'Overview', href: '/design/overview', icon: LayoutDashboard }] },
	{
		group: 'Routing',
		items: [
			{ label: 'Services', href: '/design/services', icon: Waypoints },
			{ label: 'DNS', href: '/design/dns', icon: Globe },
			{ label: 'Traefik', href: '/design/traefik', icon: Network },
			{ label: 'Firewall hosts', href: '/design/firewall', icon: BrickWall },
			{ label: 'Pulse', href: '/design/pulse', icon: Radio }
		]
	},
	{
		group: 'Protect',
		items: [
			{ label: 'Security', href: '/design/security', icon: ShieldCheck },
			{ label: 'IP sets', href: '/design/ip-sets', icon: Layers },
			{ label: 'Rule sets', href: '/design/rule-sets', icon: ListOrdered }
		]
	},
	{
		group: 'Watch',
		items: [
			{ label: 'Status', href: '/design/status', icon: Activity },
			{ label: 'App display', href: '/design/app-display', icon: AppWindow }
		]
	},
	{
		group: 'Operate',
		items: [
			{ label: 'Scripts', href: '/design/scripts', icon: SquareTerminal },
			{ label: 'Activity', href: '/design/activity', icon: History }
		]
	},
	{
		group: 'Configure',
		items: [
			{ label: 'Connections', href: '/design/connections', icon: Cable },
			{ label: 'Settings', href: '/design/settings', icon: Settings }
		]
	}
];

// Real pages that exist so far (feature slices add theirs). The live shell shows only these, at their real paths.
const built = new Set([
	'/services',
	'/dns',
	'/traefik',
	'/firewall',
	'/ip-sets',
	'/status',
	'/app-display',
	'/activity',
	'/connections',
	'/settings'
]);
export const liveNav = nav
	.map((g) => ({
		...g,
		items: g.items
			.map((i) => ({ ...i, href: i.href.replace(/^\/design/, '') }))
			.filter((i) => built.has(i.href))
	}))
	.filter((g) => g.items.length > 0);

// A setup session (before passkey sign-in exists) sees only the services pages.
export const setupNav = liveNav
	.map((g) => ({ ...g, items: g.items.filter((i) => i.href === '/services') }))
	.filter((g) => g.items.length > 0);
