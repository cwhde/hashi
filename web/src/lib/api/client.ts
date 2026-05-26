import createClient, { type Middleware } from 'openapi-fetch';
import type { paths } from './schema.js';
import type { ApiError, UndocumentedJson } from './types.js';

export const client = createClient<paths>({
	baseUrl: '',
	credentials: 'include'
});

let csrfToken: string | null = null;

export async function ensureCsrfToken(): Promise<string | null> {
	if (csrfToken) return csrfToken;
	try {
		const response = await fetch('/api/auth/csrf', { credentials: 'include' });
		if (!response.ok) return null;
		const body = (await response.json()) as { token?: string };
		csrfToken = body.token ?? null;
		return csrfToken;
	} catch {
		return null;
	}
}

const csrfMiddleware: Middleware = {
	async onRequest({ request }) {
		const method = request.method.toUpperCase();
		if (method === 'GET' || method === 'HEAD' || method === 'OPTIONS') {
			return request;
		}
		if (!request.url.includes('/api/auth/')) {
			if (!csrfToken) {
				csrfToken = await ensureCsrfToken();
			}
			if (csrfToken) {
				request.headers.set('X-CSRF-TOKEN', csrfToken);
			}
		}
		return request;
	}
};

client.use(csrfMiddleware);

export class ApiRequestError extends Error {
	constructor(
		message: string,
		readonly status: number,
		readonly body?: ApiError,
		readonly code?: string
	) {
		super(message);
		this.name = 'ApiRequestError';
	}
}

function errorFromResult(status: number, error: unknown): ApiRequestError {
	const body = error as (ApiError & { code?: string }) | undefined;
	return new ApiRequestError(
		body?.error ?? body?.message ?? 'Request failed',
		status,
		body,
		body?.code
	);
}

async function readUndocumentedJson(response: Response): Promise<UndocumentedJson> {
	try {
		return (await response.json()) as UndocumentedJson;
	} catch {
		return {};
	}
}

async function expectOk(response: Response, error: unknown): Promise<void> {
	if (!response.ok) throw errorFromResult(response.status, error);
}

async function expectData<T>(response: Response, error: unknown, data: T | undefined): Promise<T> {
	if (!response.ok) throw errorFromResult(response.status, error);
	if (data === undefined) throw new ApiRequestError('Empty response body', response.status);
	return data;
}

async function postUndocumented(path: string, init?: Record<string, unknown>): Promise<UndocumentedJson> {
	const result = await client.POST(path as never, (init ?? {}) as never);
	await expectOk(result.response, result.error);
	return readUndocumentedJson(result.response);
}

export const api = {
	getSetupStatus: async () => {
		const r = await client.GET('/api/setup/status');
		return expectData(r.response, r.error, r.data);
	},
	getBootstrapAllowed: async () => {
		const r = await client.GET('/api/setup/bootstrap-allowed');
		return expectData(r.response, r.error, r.data);
	},
	completeSetupStep: async (stepSlug: string) => {
		await postUndocumented('/api/setup/steps/{stepSlug}/complete', { params: { path: { stepSlug } } });
		return api.getSetupStatus();
	},
	verifySetupHttps: async () => {
		const body = await postUndocumented('/api/setup/verify-https');
		return {
			verified: body.verified === true,
			error: typeof body.error === 'string' ? body.error : null
		};
	},
	planSystemResourceSync: async () => {
		const body = await postUndocumented('/api/setup/system-resource/plan');
		return body as import('./types.js').SystemResourceSyncResult;
	},
	syncSystemResource: async () => {
		const body = await postUndocumented('/api/setup/system-resource/sync');
		return body as import('./types.js').SystemResourceSyncResult;
	},
	completeSetup: async () => {
		const body = await postUndocumented('/api/setup/complete');
		return { succeeded: body.succeeded !== false, error: typeof body.error === 'string' ? body.error : null };
	},
	getHealth: async () => {
		const r = await client.GET('/api/health');
		return expectData(r.response, r.error, r.data);
	},
	getGeneralSettings: async () => {
		const r = await client.GET('/api/settings/general');
		return expectData(r.response, r.error, r.data);
	},
	updateGeneralSettings: async (payload: import('./types.js').GeneralSettingsUpdate) => {
		const r = await client.PUT('/api/settings/general', { body: payload });
		return expectData(r.response, r.error, r.data);
	},
	getAuditEvents: async () => {
		const r = await client.GET('/api/activity/audit');
		return expectData(r.response, r.error, r.data ?? []);
	},
	bootstrapLogin: async (username: string, password: string) => {
		const r = await client.POST('/api/auth/bootstrap/login', { body: { username, password } });
		if (!r.response.ok) throw errorFromResult(r.response.status, r.error);
		const body = await readUndocumentedJson(r.response);
		return { succeeded: body.succeeded !== false, error: typeof body.error === 'string' ? body.error : null };
	},
	getSession: async () => {
		const r = await client.GET('/api/auth/session');
		return expectData(r.response, r.error, r.data);
	},
	logout: async () => {
		const r = await client.POST('/api/auth/logout');
		await expectOk(r.response, r.error);
		return { loggedOut: true };
	},
	reauthenticateBegin: () => postUndocumented('/api/auth/reauthenticate'),
	reauthenticateComplete: (
		assertion: Record<string, unknown>,
		challengeSessionId: string
	) =>
		postUndocumented('/api/auth/reauthenticate/complete', {
			body: { assertion, challengeSessionId }
		}),
	passkeyRegisterBegin: async (nickname = 'Primary passkey') =>
		postUndocumented('/api/auth/passkeys/register/begin', { params: { query: { nickname } } }),
	passkeyRegisterComplete: async (
		attestation: Record<string, unknown>,
		challengeSessionId: string,
		nickname = 'Primary passkey',
		clientReportsPrfSupported = false
	) => {
		const body = await postUndocumented('/api/auth/passkeys/register/complete', {
			body: { attestation, challengeSessionId, nickname, clientReportsPrfSupported }
		});
		return { credentialId: String(body.credentialId ?? ''), prfSupported: body.prfSupported === true };
	},
	passkeyLoginBegin: async () => postUndocumented('/api/auth/passkeys/login/begin'),
	passkeyLoginComplete: async (
		assertion: Record<string, unknown>,
		challengeSessionId: string,
		prfOutputBase64?: string | null
	) => {
		const body = await postUndocumented('/api/auth/passkeys/login/complete', {
			body: { assertion, challengeSessionId, prfOutputBase64: prfOutputBase64 ?? null }
		});
		return { succeeded: body.succeeded !== false, vaultUnlocked: body.vaultUnlocked === true };
	},
	getVaultStatus: async () => {
		const r = await client.GET('/api/vault/status');
		return expectData(r.response, r.error, r.data);
	},
	generateRecoveryKey: async () => {
		const r = await client.POST('/api/vault/recovery-key/generate');
		return expectData(r.response, r.error, r.data);
	},
	setupVault: async (payload: import('./types.js').VaultSetupRequest) => {
		const body = await postUndocumented('/api/vault/setup', { body: payload });
		return {
			configured: body.configured === true,
			prfWrapStored: body.prfWrapStored === true,
			serviceSyncWrapStored: body.serviceSyncWrapStored === true,
			generatedRecoveryKey: String(body.generatedRecoveryKey ?? payload.recoveryKey)
		};
	},
	verifyVaultUnlock: async (recoveryKey: string) => {
		const body = await postUndocumented('/api/vault/verify-unlock', { body: { recoveryKey } });
		return { verified: body.verified === true, vaultUnlocked: body.vaultUnlocked === true };
	},
	listVaultSecrets: async () => {
		const r = await client.GET('/api/vault/secrets');
		return expectData(r.response, r.error, r.data ?? []);
	},

	validateHetznerDnsProvider: (body: import('./types.js').DnsProviderValidationRequest) =>
		postUndocumented('/api/dns/providers/hetzner/validate', { body }),
	listDnsConnections: async () => {
		const r = await client.GET('/api/dns/connections');
		return expectData(r.response, r.error, r.data ?? []);
	},
	createHetznerDnsConnection: (body: import('./types.js').CreateHetznerDnsConnectionRequest) =>
		postUndocumented('/api/dns/connections/hetzner', { body }),
	validateDnsConnection: (connectionId: string) =>
		postUndocumented('/api/dns/connections/{connectionId}/validate', { params: { path: { connectionId } } }),
	validateDnsWrite: (connectionId: string, confirmDryRun: boolean) =>
		postUndocumented('/api/dns/connections/{connectionId}/validate-write', {
			params: { path: { connectionId } },
			body: { confirmDryRun }
		}),
	listProviderDnsRecords: async (connectionId: string) => {
		const r = await client.GET('/api/dns/connections/{connectionId}/records/provider', {
			params: { path: { connectionId } }
		});
		return expectData(r.response, r.error, r.data ?? []);
	},
	previewDnsImport: async (connectionId: string) => {
		const r = await client.POST('/api/dns/connections/{connectionId}/import/preview', {
			params: { path: { connectionId } }
		});
		return expectData(r.response, r.error, r.data ?? []);
	},
	applyDnsImport: (connectionId: string, body: import('./types.js').DnsImportApplyRequest) =>
		postUndocumented('/api/dns/connections/{connectionId}/import/apply', {
			params: { path: { connectionId } },
			body
		}),
	planDnsSync: (connectionId: string) =>
		postUndocumented('/api/dns/connections/{connectionId}/sync/plan', { params: { path: { connectionId } } }),
	applyDnsSync: (connectionId: string, body: import('./types.js').DnsSyncApplyRequest) =>
		postUndocumented('/api/dns/connections/{connectionId}/sync/apply', { params: { path: { connectionId } }, body }),
	listDnsRecords: async () => {
		const r = await client.GET('/api/dns/records');
		return expectData(r.response, r.error, r.data ?? []);
	},

	listConnections: async (type?: string) => {
		const r = await client.GET('/api/connections', { params: { query: type ? { type } : {} } });
		return expectData(r.response, r.error, r.data ?? []);
	},
	createSshConnection: (body: import('./types.js').CreateSshConnectionRequest) =>
		postUndocumented('/api/connections/ssh', { body }),
	validateConnection: (connectionId: string, body: import('./types.js').CreateSshConnectionRequest) =>
		postUndocumented('/api/connections/{connectionId}/validate', { params: { path: { connectionId } }, body }),
	writeRemoteFile: (connectionId: string, body: import('./types.js').RemoteWriteRequest) =>
		postUndocumented('/api/connections/{connectionId}/write', { params: { path: { connectionId } }, body }),

	listResources: async () => {
		const r = await client.GET('/api/resources');
		return expectData(r.response, r.error, r.data ?? []);
	},
	createResource: async (body: import('./types.js').CreateResourceRequest) => {
		const r = await client.POST('/api/resources', { body });
		await expectOk(r.response, r.error);
		return readUndocumentedJson(r.response);
	},
	updateResource: async (id: string, body: import('./types.js').UpdateResourceRequest) => {
		const r = await client.PUT('/api/resources/{id}', { params: { path: { id } }, body });
		await expectOk(r.response, r.error);
		return readUndocumentedJson(r.response);
	},
	deleteResource: async (id: string) => {
		const r = await client.DELETE('/api/resources/{id}', { params: { path: { id } } });
		await expectOk(r.response, r.error);
	},

	getTraefikRender: async () => {
		const r = await client.GET('/api/traefik/render');
		return expectData(r.response, r.error, r.data);
	},
	renderFirewall: async (body: import('./types.js').FirewallRenderRequest) => {
		const r = await client.POST('/api/firewall/render', { body });
		return expectData(r.response, r.error, r.data);
	},
	listFirewallHosts: async () => {
		const r = await client.GET('/api/firewall/hosts');
		return expectData(r.response, r.error, r.data ?? []);
	},
	createFirewallHost: async (body: import('./types.js').CreateFirewallHostRequest) => {
		const r = await client.POST('/api/firewall/hosts', { body });
		return expectData(r.response, r.error, r.data);
	},
	applyTraefik: (body: import('./types.js').TraefikApplyRequest) =>
		postUndocumented('/api/traefik/apply', { body }),
	rollbackTraefik: (body: import('./types.js').TraefikApplyRequest) =>
		postUndocumented('/api/traefik/rollback', { body }),

	listStatusEndpoints: async () => {
		const r = await client.GET('/api/status/endpoints');
		return expectData(r.response, r.error, r.data ?? []);
	},
	listStatusRollups: async (params?: {
		endpointId?: string;
		intervalMinutes?: number;
		hours?: number;
	}) => {
		const r = await client.GET('/api/status/rollups', {
			params: {
				query: {
					endpointId: params?.endpointId,
					intervalMinutes: params?.intervalMinutes,
					hours: params?.hours
				}
			}
		});
		return expectData(r.response, r.error, r.data ?? []);
	},
	getSecurityDashboard: async (params?: { hours?: number }) => {
		const r = await client.GET('/api/security/dashboard', {
			params: {
				query: {
					hours: params?.hours
				}
			}
		});
		return expectData(r.response, r.error, r.data);
	},
	listPulseAgents: async () => {
		const r = await client.GET('/api/pulse/agents');
		return expectData(r.response, r.error, r.data ?? []);
	},
	createPulseAgent: async (body: import('./types.js').CreatePulseAgentRequest) => {
		const r = await client.POST('/api/pulse/agents', { body });
		return expectData(r.response, r.error, r.data);
	},
	listScripts: async () => {
		const r = await client.GET('/api/scripts');
		return expectData(r.response, r.error, r.data ?? []);
	},
	createScript: async (body: import('./types.js').CreateScriptRequest) => {
		const r = await client.POST('/api/scripts', { body });
		return expectData(r.response, r.error, r.data);
	},
	runScript: async (
		scriptId: string,
		body: import('./types.js').RunScriptRequest = { port: 22, authMode: 'password' }
	) => {
		const r = await client.POST('/api/scripts/{scriptId}/run', {
			params: { path: { scriptId } },
			body
		});
		return expectData(r.response, r.error, r.data);
	},
	listNotificationProviders: async () => {
		const r = await client.GET('/api/settings/notifications/providers');
		return expectData(r.response, r.error, r.data ?? []);
	},
	createNotificationProvider: async (body: import('./types.js').CreateNotificationProviderRequest) => {
		const r = await client.POST('/api/settings/notifications/providers', { body });
		return expectData(r.response, r.error, r.data);
	},
	getPublicApps: async () => {
		const r = await client.GET('/api/public/apps');
		return expectData(r.response, r.error, r.data ?? []);
	},
	getPublicStatus: async () => {
		const r = await client.GET('/api/public/status');
		return expectData(r.response, r.error, r.data ?? []);
	},

	listSyncRuns: async () => {
		const r = await client.GET('/api/sync/runs');
		return expectData(r.response, r.error, r.data ?? []);
	},
	getSyncRun: async (id: string) => {
		const r = await client.GET('/api/sync/runs/{id}', { params: { path: { id } } });
		return expectData(r.response, r.error, r.data);
	},
	planGlobalSync: async () => {
		const body = await postUndocumented('/api/sync/plan');
		return body as import('./types.js').SyncPlanPreview;
	},
	applyGlobalSync: async (confirmDestructive: boolean) => {
		const body = await postUndocumented('/api/sync/apply', {
			body: { confirmDestructive }
		});
		return {
			runId: String(body.runId ?? ''),
			succeeded: body.succeeded === true,
			status: String(body.status ?? ''),
			error: typeof body.error === 'string' ? body.error : null
		};
	},
	reconcileGlobalSync: async () => {
		const body = await postUndocumented('/api/sync/reconcile');
		return body as import('./types.js').SyncReconcileResult;
	},

	listAdGuardConnections: async () => {
		const r = await client.GET('/api/adguard/connections');
		return expectData(r.response, r.error, r.data ?? []);
	},
	createAdGuardConnection: async (body: import('./types.js').CreateAdGuardConnectionRequest) => {
		const r = await client.POST('/api/adguard/connections', { body });
		return expectData(r.response, r.error, r.data);
	},
	listAdGuardRewrites: async (connectionId: string) => {
		const r = await client.GET('/api/adguard/{connectionId}/rewrites', {
			params: { path: { connectionId } }
		});
		return expectData(r.response, r.error, r.data ?? []);
	},
	upsertAdGuardRewrite: async (
		connectionId: string,
		body: import('./types.js').UpsertAdGuardRewriteRequest
	) => {
		const r = await client.POST('/api/adguard/{connectionId}/rewrites', {
			params: { path: { connectionId } },
			body
		});
		return expectData(r.response, r.error, r.data);
	},
	syncAdGuardConnection: async (connectionId: string) => {
		const r = await client.POST('/api/adguard/{connectionId}/sync', {
			params: { path: { connectionId } }
		});
		if (!r.response.ok) throw errorFromResult(r.response.status, r.error);
		return readUndocumentedJson(r.response) as { synced?: boolean };
	}
};
