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
import { navRoutes } from './nav-routes.js';

export type NavItem = {
	label: string;
	href: string;
	icon: LucideIcon;
};

const icons: LucideIcon[] = [
	LayoutDashboard,
	Server,
	Globe,
	Network,
	Shield,
	Zap,
	HeartPulse,
	Monitor,
	Lock,
	FileCode,
	Cable,
	Activity,
	SettingsIcon
];

export const navItems = navRoutes.map((route, index) => ({
	...route,
	icon: icons[index]!
})) satisfies NavItem[];
