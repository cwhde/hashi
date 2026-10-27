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
	Settings
} from '@lucide/svelte';

// Navigation grouping decided in S0b (docs/spec/10b-ui-system.md §2). `href` is the /design mock route today and
// becomes the real route (without /design) when a feature slice fills the page.
export const nav = [
	{ items: [{ label: 'Overview', href: '/design/overview', icon: LayoutDashboard }] },
	{
		group: 'Routing',
		items: [
			{ label: 'Resources', href: '/design/resources', icon: Waypoints },
			{ label: 'DNS', href: '/design/dns', icon: Globe },
			{ label: 'Traefik', href: '/design/traefik', icon: Network },
			{ label: 'Firewall hosts', href: '/design/firewall', icon: BrickWall },
			{ label: 'Pulse', href: '/design/pulse', icon: Radio }
		]
	},
	{
		group: 'Watch',
		items: [
			{ label: 'Status', href: '/design/status', icon: Activity },
			{ label: 'App display', href: '/design/app-display', icon: AppWindow },
			{ label: 'Security', href: '/design/security', icon: ShieldCheck }
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
