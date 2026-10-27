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

/** Where an unauthenticated or setup-incomplete visitor belongs, or null when the admin app is fine. */
export function routeFor(s: AuthSession): '/setup' | '/login' | null {
	if (!s.setup_completed) return '/setup';
	if (!s.authenticated || s.method !== 'passkey') return '/login';
	return null;
}
