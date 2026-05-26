export type RootPortMode = 'admin' | 'public-dashboard' | 'public-status';

/** Which root-page experience to render (admin app vs dedicated public ports). */
export function resolveRootPortMode(): RootPortMode {
	if (typeof window === 'undefined') {
		return 'admin';
	}

	const port = window.location.port;
	if (port === '8081') {
		return 'public-dashboard';
	}

	if (port === '8082') {
		return 'public-status';
	}

	return 'admin';
}
