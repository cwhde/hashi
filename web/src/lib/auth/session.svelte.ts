import { api, setCSRF, type Schemas } from '$lib/api/client';

export type AuthSession = Schemas['AuthSession'];

// The current admin session as the server sees it. Pages read it; only refresh() changes it.
class SessionState {
	current = $state<AuthSession | null>(null);
	loaded = $state(false);

	async refresh(): Promise<AuthSession> {
		const { data, error } = await api.GET('/api/auth/session');
		if (error || !data) throw error ?? new Error('no session response');
		this.set(data);
		return data;
	}

	set(s: AuthSession) {
		this.current = s;
		this.loaded = true;
		setCSRF(s.csrf_token);
	}

	has(scope: string) {
		return !!this.current?.scopes?.includes(scope);
	}
}

export const session = new SessionState();

/** Whether this is a setup (bootstrap) session that may manage services before setup completes (02 §3.3). */
export function setupServices(s: AuthSession | null): boolean {
	return (
		!!s && !s.setup_completed && s.method === 'bootstrap' && !!s.scopes?.includes('services.manage')
	);
}

/** Where an unauthenticated or setup-incomplete visitor belongs, or null when the admin app is fine. While setup is
 * open, a setup session may use the services pages (services are migrated before passkey sign-in exists). */
export function routeFor(s: AuthSession, path = '/'): '/setup' | '/login' | null {
	if (setupServices(s) && (path === '/services' || path.startsWith('/services/'))) return null;
	if (!s.setup_completed) return '/setup';
	if (!s.authenticated || s.method !== 'passkey') return '/login';
	return null;
}
