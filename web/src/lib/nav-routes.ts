/** Navigation routes without icon imports (safe for unit tests). */
export const navRoutes = [
	{ label: 'Overview', href: '/' },
	{ label: 'Resources', href: '/resources' },
	{ label: 'DNS', href: '/dns' },
	{ label: 'Traefik', href: '/traefik' },
	{ label: 'Firewall Hosts', href: '/firewall-hosts' },
	{ label: 'Pulse', href: '/pulse' },
	{ label: 'Status', href: '/status' },
	{ label: 'App Display', href: '/app-display' },
	{ label: 'Security', href: '/security' },
	{ label: 'Scripts', href: '/scripts' },
	{ label: 'Connections', href: '/connections' },
	{ label: 'Activity', href: '/activity' },
	{ label: 'Settings', href: '/settings' }
] as const;
