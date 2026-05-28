export type RootPortMode = 'admin' | 'public-dashboard' | 'public-status';

type HashiRuntimeConfig = {
	ports?: {
		publicDashboard?: number;
		publicStatus?: number;
	};
};

declare global {
	interface Window {
		__HASHI_RUNTIME_CONFIG__?: HashiRuntimeConfig;
	}
}

/** Which root-page experience to render (admin app vs dedicated public ports). */
export function resolveRootPortMode(): RootPortMode {
	if (typeof window === 'undefined') {
		return 'admin';
	}

	const port = window.location.port;
	const ports = window.__HASHI_RUNTIME_CONFIG__?.ports;
	const dashboardPort = ports?.publicDashboard?.toString() ?? '8081';
	const statusPort = ports?.publicStatus?.toString() ?? '8082';

	if (port === dashboardPort) {
		return 'public-dashboard';
	}

	if (port === statusPort) {
		return 'public-status';
	}

	return 'admin';
}
