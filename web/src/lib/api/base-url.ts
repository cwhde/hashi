/** Admin API is only served on the main app port (default 8080). */
export function resolveApiBaseUrl(): string {
	if (typeof window === 'undefined') {
		return '';
	}

	const port = window.location.port;
	if (port === '8081' || port === '8082') {
		const adminPort = import.meta.env.VITE_HASHI_ADMIN_PORT ?? '8080';
		return `${window.location.protocol}//${window.location.hostname}:${adminPort}`;
	}

	return '';
}
