import createClient from 'openapi-fetch';
import type { paths } from './schema.js';
import type { ApiError, UndocumentedJson } from './types.js';

const client = createClient<paths>({
	baseUrl: '',
	credentials: 'include'
});

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

function errorFromResult(status: number, error: unknown): ApiRequestError {
	const body = error as ApiError | undefined;
	return new ApiRequestError(body?.error ?? body?.message ?? 'Request failed', status, body);
}

async function readUndocumentedJson(response: Response): Promise<UndocumentedJson> {
	try {
		return (await response.json()) as UndocumentedJson;
	} catch {
		return {};
	}
}

async function expectOk(response: Response, error: unknown): Promise<void> {
	if (!response.ok) {
		throw errorFromResult(response.status, error);
	}
}

async function expectData<T>(response: Response, error: unknown, data: T | undefined): Promise<T> {
	if (!response.ok) {
		throw errorFromResult(response.status, error);
	}
	if (data === undefined) {
		throw new ApiRequestError('Empty response body', response.status);
	}
	return data;
}

export const api = {
	getSetupStatus: async () => {
		const result = await client.GET('/api/setup/status');
		return expectData(result.response, result.error, result.data);
	},

	getBootstrapAllowed: async () => {
		const result = await client.GET('/api/setup/bootstrap-allowed');
		return expectData(result.response, result.error, result.data);
	},

	completeSetupStep: async (stepSlug: string) => {
		const result = await client.POST('/api/setup/steps/{stepSlug}/complete', {
			params: { path: { stepSlug } }
		});
		await expectOk(result.response, result.error);
		return api.getSetupStatus();
	},

	completeSetup: async () => {
		const result = await client.POST('/api/setup/complete');
		await expectOk(result.response, result.error);
		const body = await readUndocumentedJson(result.response);
		return {
			succeeded: body.succeeded !== false,
			error: typeof body.error === 'string' ? body.error : null
		};
	},

	getHealth: async () => {
		const result = await client.GET('/api/health');
		return expectData(result.response, result.error, result.data);
	},

	getGeneralSettings: async () => {
		const result = await client.GET('/api/settings/general');
		return expectData(result.response, result.error, result.data);
	},

	updateGeneralSettings: async (
		payload: import('./types.js').GeneralSettingsUpdate
	) => {
		const result = await client.PUT('/api/settings/general', { body: payload });
		return expectData(result.response, result.error, result.data);
	},

	getAuditEvents: async () => {
		const result = await client.GET('/api/activity/audit');
		return expectData(result.response, result.error, result.data ?? []);
	},

	bootstrapLogin: async (username: string, password: string) => {
		const result = await client.POST('/api/auth/bootstrap/login', {
			body: { username, password }
		});
		if (!result.response.ok) {
			throw errorFromResult(result.response.status, result.error);
		}
		const body = await readUndocumentedJson(result.response);
		return {
			succeeded: body.succeeded !== false,
			error: typeof body.error === 'string' ? body.error : null
		};
	},

	getSession: async () => {
		const result = await client.GET('/api/auth/session');
		return expectData(result.response, result.error, result.data);
	},

	logout: async () => {
		const result = await client.POST('/api/auth/logout');
		await expectOk(result.response, result.error);
		return { loggedOut: true };
	},

	passkeyRegisterBegin: async (nickname = 'Primary passkey') => {
		const result = await client.POST('/api/auth/passkeys/register/begin', {
			params: { query: { nickname } }
		});
		await expectOk(result.response, result.error);
		return readUndocumentedJson(result.response);
	},

	passkeyRegisterComplete: async (
		attestation: Record<string, unknown>,
		challengeSessionId: string,
		nickname = 'Primary passkey',
		clientReportsPrfSupported = false
	) => {
		const result = await client.POST('/api/auth/passkeys/register/complete', {
			body: { attestation, challengeSessionId, nickname, clientReportsPrfSupported }
		});
		await expectOk(result.response, result.error);
		const body = await readUndocumentedJson(result.response);
		return {
			credentialId: String(body.credentialId ?? ''),
			prfSupported: body.prfSupported === true
		};
	},

	passkeyLoginBegin: async () => {
		const result = await client.POST('/api/auth/passkeys/login/begin');
		await expectOk(result.response, result.error);
		return readUndocumentedJson(result.response);
	},

	passkeyLoginComplete: async (
		assertion: Record<string, unknown>,
		challengeSessionId: string,
		prfOutputBase64?: string | null
	) => {
		const result = await client.POST('/api/auth/passkeys/login/complete', {
			body: { assertion, challengeSessionId, prfOutputBase64: prfOutputBase64 ?? null }
		});
		await expectOk(result.response, result.error);
		const body = await readUndocumentedJson(result.response);
		return {
			succeeded: body.succeeded !== false,
			vaultUnlocked: body.vaultUnlocked === true
		};
	},

	getVaultStatus: async () => {
		const result = await client.GET('/api/vault/status');
		return expectData(result.response, result.error, result.data);
	},

	generateRecoveryKey: async () => {
		const result = await client.POST('/api/vault/recovery-key/generate');
		return expectData(result.response, result.error, result.data);
	},

	setupVault: async (payload: import('./types.js').VaultSetupRequest) => {
		const result = await client.POST('/api/vault/setup', { body: payload });
		await expectOk(result.response, result.error);
		const body = await readUndocumentedJson(result.response);
		return {
			configured: body.configured === true,
			prfWrapStored: body.prfWrapStored === true,
			serviceSyncWrapStored: body.serviceSyncWrapStored === true,
			generatedRecoveryKey: String(body.generatedRecoveryKey ?? payload.recoveryKey)
		};
	},

	verifyVaultUnlock: async (recoveryKey: string) => {
		const result = await client.POST('/api/vault/verify-unlock', {
			body: { recoveryKey }
		});
		await expectOk(result.response, result.error);
		const body = await readUndocumentedJson(result.response);
		return {
			verified: body.verified === true,
			vaultUnlocked: body.vaultUnlocked === true
		};
	},

	listVaultSecrets: async () => {
		const result = await client.GET('/api/vault/secrets');
		return expectData(result.response, result.error, result.data ?? []);
	}
};
