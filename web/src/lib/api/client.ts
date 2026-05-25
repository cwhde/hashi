import type { ApiError } from './types.js';

const API_BASE = '';

export class ApiRequestError extends Error {
	constructor(
		message: string,
		readonly status: number,
		readonly body?: ApiError
	) {
		super(message);
		this.name = 'ApiRequestError';
	}
}

export async function apiFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
	const headers = new Headers(init.headers);
	if (init.body && !headers.has('Content-Type')) {
		headers.set('Content-Type', 'application/json');
	}

	const response = await fetch(`${API_BASE}${path}`, {
		credentials: 'include',
		...init,
		headers
	});

	if (!response.ok) {
		let body: ApiError | undefined;
		try {
			body = (await response.json()) as ApiError;
		} catch {
			// ignore parse errors
		}
		throw new ApiRequestError(
			body?.error ?? body?.message ?? response.statusText,
			response.status,
			body
		);
	}

	if (response.status === 204) {
		return undefined as T;
	}

	return (await response.json()) as T;
}

export const api = {
	getSetupStatus: () =>
		apiFetch<import('./types.js').SetupStatusResponse>('/api/setup/status'),

	getBootstrapAllowed: () =>
		apiFetch<import('./types.js').BootstrapAllowedResponse>('/api/setup/bootstrap-allowed'),

	completeSetupStep: (stepSlug: string) =>
		apiFetch<import('./types.js').SetupStatusResponse>(
			`/api/setup/steps/${stepSlug}/complete`,
			{ method: 'POST' }
		),

	completeSetup: () =>
		apiFetch<import('./types.js').SetupCompleteResponse>('/api/setup/complete', {
			method: 'POST'
		}),

	getHealth: () => apiFetch<import('./types.js').HealthResponse>('/api/health'),

	getGeneralSettings: () =>
		apiFetch<import('./types.js').GeneralSettings>('/api/settings/general'),

	updateGeneralSettings: (payload: import('./types.js').GeneralSettingsUpdate) =>
		apiFetch<{ updated: boolean; updatedAtUtc: string }>('/api/settings/general', {
			method: 'PUT',
			body: JSON.stringify(payload)
		}),

	getAuditEvents: () =>
		apiFetch<import('./types.js').AuditEvent[]>('/api/activity/audit'),

	bootstrapLogin: (username: string, password: string) =>
		apiFetch<import('./types.js').BootstrapLoginResponse>('/api/auth/bootstrap/login', {
			method: 'POST',
			body: JSON.stringify({ username, password })
		}),

	getSession: () => apiFetch<import('./types.js').SessionStatus>('/api/auth/session'),

	logout: () => apiFetch<{ loggedOut: boolean }>('/api/auth/logout', { method: 'POST' }),

	passkeyRegisterBegin: (nickname = 'Primary passkey') =>
		apiFetch<import('./types.js').PasskeyBeginResponse>(
			`/api/auth/passkeys/register/begin?nickname=${encodeURIComponent(nickname)}`,
			{ method: 'POST' }
		),

	passkeyRegisterComplete: (
		attestation: Record<string, unknown>,
		challengeSessionId: string,
		nickname = 'Primary passkey',
		clientReportsPrfSupported = false
	) =>
		apiFetch<import('./types.js').PasskeyRegistrationCompleteResponse>(
			'/api/auth/passkeys/register/complete',
			{
				method: 'POST',
				body: JSON.stringify({
					attestation,
					challengeSessionId,
					nickname,
					clientReportsPrfSupported
				})
			}
		),

	passkeyLoginBegin: () =>
		apiFetch<import('./types.js').PasskeyBeginResponse>('/api/auth/passkeys/login/begin', {
			method: 'POST'
		}),

	passkeyLoginComplete: (
		assertion: Record<string, unknown>,
		challengeSessionId: string,
		prfOutputBase64?: string | null
	) =>
		apiFetch<import('./types.js').PasskeyLoginCompleteResponse>(
			'/api/auth/passkeys/login/complete',
			{
				method: 'POST',
				body: JSON.stringify({ assertion, challengeSessionId, prfOutputBase64 })
			}
		),

	getVaultStatus: () => apiFetch<import('./types.js').VaultStatus>('/api/vault/status'),

	generateRecoveryKey: () =>
		apiFetch<{ recoveryKey: string }>('/api/vault/recovery-key/generate', { method: 'POST' }),

	setupVault: (payload: {
		recoveryKey: string;
		prfWrapAttempted: boolean;
		prfOutputBase64?: string | null;
		passkeyCredentialId?: string | null;
	}) =>
		apiFetch<import('./types.js').VaultSetupResponse>('/api/vault/setup', {
			method: 'POST',
			body: JSON.stringify(payload)
		}),

	verifyVaultUnlock: (recoveryKey: string) =>
		apiFetch<{ verified: boolean; vaultUnlocked: boolean }>('/api/vault/verify-unlock', {
			method: 'POST',
			body: JSON.stringify({ recoveryKey })
		})
};
