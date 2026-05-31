export type OverviewWidgetDef = {
	id: string;
	title: string;
	description: string;
};

export const DEFAULT_WIDGETS: OverviewWidgetDef[] = [
	{ id: 'resource-health', title: 'Resource health', description: 'Healthy, degraded, and down counts.' },
	{ id: 'firewall-hosts', title: 'Firewall hosts', description: 'SSH reachability and last checks.' },
	{ id: 'traefik-sync', title: 'Traefik sync', description: 'Last reconcile and pending changes.' },
	{ id: 'dns-sync', title: 'DNS sync', description: 'Provider drift and last sync.' },
	{ id: 'incidents', title: 'Recent incidents', description: 'Open and recent status incidents.' },
	{ id: 'security-events', title: 'Security events', description: 'Active abuse and block activity.' },
	{ id: 'pending-sync', title: 'Pending sync', description: 'Queued plans awaiting approval.' },
	{ id: 'cert-expiry', title: 'Certificate expiry', description: 'ACME certs nearing expiration.' },
	{ id: 'vault-lock', title: 'Vault lock state', description: 'Passkey and vault unlock status.' },
	{ id: 'audit', title: 'Recent audit', description: 'Latest privileged actions.' }
];

const STORAGE_KEY = 'hashi.overview.widgets';

export type WidgetPrefs = {
	enabled: Record<string, boolean>;
	order: string[];
};

export function loadWidgetPrefs(): WidgetPrefs {
	return normalizeWidgetPrefs(readStoredWidgetPrefs());
}

export function parseWidgetPrefsJson(json: string | null | undefined): WidgetPrefs {
	if (!json) return loadWidgetPrefs();
	try {
		return normalizeWidgetPrefs(JSON.parse(json) as Partial<WidgetPrefs>);
	} catch {
		return loadWidgetPrefs();
	}
}

export function normalizeWidgetPrefs(prefs?: Partial<WidgetPrefs> | null): WidgetPrefs {
	const defaults: WidgetPrefs = {
		enabled: Object.fromEntries(DEFAULT_WIDGETS.map((w) => [w.id, true])),
		order: DEFAULT_WIDGETS.map((w) => w.id)
	};
	const enabled = { ...defaults.enabled, ...(prefs?.enabled ?? {}) };
	const requestedOrder = Array.isArray(prefs?.order) ? prefs.order : [];
	const known = new Set(DEFAULT_WIDGETS.map((w) => w.id));
	const order = [
		...requestedOrder.filter((id) => known.has(id)),
		...defaults.order.filter((id) => !requestedOrder.includes(id))
	];
	return { enabled, order };
}

function readStoredWidgetPrefs(): Partial<WidgetPrefs> | null {
	if (typeof localStorage === 'undefined') return null;
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) return null;
		return JSON.parse(raw) as Partial<WidgetPrefs>;
	} catch {
		return null;
	}
}

export function saveWidgetPrefs(prefs: WidgetPrefs): void {
	if (typeof localStorage === 'undefined') return;
	localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs));
}
