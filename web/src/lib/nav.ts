import {
	Activity,
	Cable,
	FileCode,
	Globe,
	HeartPulse,
	LayoutDashboard,
	Lock,
	Monitor,
	Network,
	Server,
	Settings as SettingsIcon,
	Shield,
	Zap
} from 'lucide-svelte';

import type { LucideIcon } from '$lib/icons';

export type NavItem = {
	label: string;
	href: string;
	icon: LucideIcon;
};

export const navItems = [
	{ label: 'Overview', href: '/', icon: LayoutDashboard },
	{ label: 'Resources', href: '/resources', icon: Server },
	{ label: 'DNS', href: '/dns', icon: Globe },
	{ label: 'Traefik', href: '/traefik', icon: Network },
	{ label: 'Firewall Hosts', href: '/firewall-hosts', icon: Shield },
	{ label: 'Pulse', href: '/pulse', icon: Zap },
	{ label: 'Status', href: '/status', icon: HeartPulse },
	{ label: 'App Display', href: '/app-display', icon: Monitor },
	{ label: 'Security', href: '/security', icon: Lock },
	{ label: 'Scripts', href: '/scripts', icon: FileCode },
	{ label: 'Connections', href: '/connections', icon: Cable },
	{ label: 'Activity', href: '/activity', icon: Activity },
	{ label: 'Settings', href: '/settings', icon: SettingsIcon }
] satisfies NavItem[];
