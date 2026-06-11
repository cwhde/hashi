import createClient, { type Middleware } from 'openapi-fetch';
import type { components, paths } from './schema.js';
import type { ApiError, UndocumentedJson } from './types.js';
import { resolveApiBaseUrl } from './base-url.js';

export const client = createClient<paths>({
	baseUrl: resolveApiBaseUrl(),
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
		if (!isCsrfExemptRequest(request.url, method)) {
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

function isCsrfExemptRequest(url: string, method: string): boolean {
	const path = new URL(url, 'http://hashi.local').pathname;
	return (
		(method === 'POST' && path === '/api/auth/bootstrap/login') ||
		(method === 'POST' && path === '/api/auth/passkeys/login/begin') ||
		(method === 'POST' && path === '/api/auth/passkeys/login/complete') ||
		(method === 'POST' && path === '/api/edge-challenge/verify')
	);
}

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

type BlocklistSourceRequest = components['schemas']['UpsertBlocklistSourceRequest'];
type BlocklistSourceResponse = components['schemas']['BlocklistSourceResponse'];
type BlocklistFetchPreviewResponse = components['schemas']['BlocklistFetchPreviewResponse'];
type BlocklistSourceMutationResponse = components['schemas']['BlocklistSourceMutationResponse'];

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
	getCertificateSetup: async () => {
		const r = await client.GET('/api/setup/certificate');
		return expectData(r.response, r.error, r.data);
	},
	validateCertificateSetup: async (body: import('./types.js').CertificateSetupRequest) => {
		const r = await client.POST('/api/setup/certificate/validate', { body });
		return expectData(r.response, r.error, r.data);
	},
	saveCertificateSetup: async (body: import('./types.js').CertificateSetupRequest) => {
		const r = await client.POST('/api/setup/certificate/save', { body });
		return expectData(r.response, r.error, r.data);
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
	getAdminDashboard: async () => {
		const r = await client.GET('/api/dashboard');
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
	listBackgroundJobs: async () => {
		const r = await client.GET('/api/activity/jobs');
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
	previewDnsPrune: async (connectionId: string) => {
		const r = await client.POST('/api/dns/connections/{connectionId}/prune/preview', {
			params: { path: { connectionId } }
		});
		return expectData(r.response, r.error, r.data);
	},
	getDnsConnectionCapabilities: async (connectionId: string) => {
		const r = await client.GET('/api/dns/connections/{connectionId}/capabilities' as never, {
			params: { path: { connectionId } }
		} as never);
		return expectData(r.response, r.error, r.data) as unknown as Promise<import('./types.js').DnsProviderCapabilities>;
	},
	applyDnsPrune: (connectionId: string) =>
		postUndocumented('/api/dns/connections/{connectionId}/prune/apply', {
			params: { path: { connectionId } },
			body: { confirmDestructive: true }
		}),
	planDnsSync: (connectionId: string) =>
		postUndocumented('/api/dns/connections/{connectionId}/sync/plan', { params: { path: { connectionId } } }),
	applyDnsSync: (connectionId: string, body: import('./types.js').DnsSyncApplyRequest) =>
		postUndocumented('/api/dns/connections/{connectionId}/sync/apply', { params: { path: { connectionId } }, body }),
	listDnsZones: async () => {
		const r = await client.GET('/api/dns/zones' as never);
		return (await expectData(r.response, r.error, r.data ?? [])) as import('./types.js').DnsZone[];
	},
	listDnsRecords: async () => {
		const r = await client.GET('/api/dns/records');
		return expectData(r.response, r.error, r.data ?? []) as Promise<import('./types.js').DnsRecord[]>;
	},
	createDnsRecord: async (body: import('./types.js').ManualDnsRecordRequest) => {
		const r = await client.POST('/api/dns/records' as never, { body } as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').DnsRecord;
	},
	updateDnsRecord: async (recordId: string, body: import('./types.js').ManualDnsRecordRequest) => {
		const r = await client.PUT('/api/dns/records/{recordId}' as never, {
			params: { path: { recordId } },
			body
		} as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').DnsRecord;
	},
	deleteDnsRecord: async (recordId: string) => {
		const r = await client.DELETE('/api/dns/records/{recordId}' as never, {
			params: { path: { recordId } }
		} as never);
		await expectOk(r.response, r.error);
	},

	listConnections: async (type?: string) => {
		const r = await client.GET('/api/connections', { params: { query: type ? { type } : {} } });
		return expectData(r.response, r.error, r.data ?? []);
	},
	createSshConnection: (body: import('./types.js').CreateSshConnectionRequest) =>
		postUndocumented('/api/connections/ssh', { body }),
	validateConnection: (connectionId: string) =>
		postUndocumented('/api/connections/{connectionId}/validate', { params: { path: { connectionId } } }),
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
	validateTraefikConfig: async () => {
		const r = await client.POST('/api/traefik/validate');
		return expectData(r.response, r.error, r.data);
	},
	getTraefikUserMiddlewares: async () => {
		const r = await client.GET('/api/traefik/user-middlewares');
		return expectData(r.response, r.error, r.data);
	},
	updateTraefikUserMiddlewares: async (body: import('./types.js').UpdateTraefikUserMiddlewareRequest) => {
		const r = await client.PUT('/api/traefik/user-middlewares', { body });
		return expectData(r.response, r.error, r.data);
	},
	validateTraefikUserMiddlewares: async (body: import('./types.js').TraefikUserMiddlewareValidationRequest) => {
		const r = await client.POST('/api/traefik/user-middlewares/validate', { body });
		return expectData(r.response, r.error, r.data);
	},
	getTraefikHostState: async (connectionId: string) => {
		const r = await client.GET('/api/traefik/connections/{connectionId}/state', {
			params: { path: { connectionId } }
		});
		return expectData(r.response, r.error, r.data);
	},
	detectExistingTraefikConfig: async (connectionId: string) => {
		const r = await client.POST('/api/traefik/connections/{connectionId}/detect-existing', {
			params: { path: { connectionId } }
		});
		return expectData(r.response, r.error, r.data);
	},
	applyTraefikConnection: async (
		connectionId: string,
		body: import('./types.js').TraefikApplyConnectionRequest
	) => {
		const r = await client.POST('/api/traefik/connections/{connectionId}/apply', {
			params: { path: { connectionId } },
			body
		});
		return expectData(r.response, r.error, r.data);
	},
	rollbackTraefikConnection: async (connectionId: string) => {
		const r = await client.POST('/api/traefik/connections/{connectionId}/rollback', {
			params: { path: { connectionId } }
		});
		return expectData(r.response, r.error, r.data);
	},
	listPendingTraefikEntryPoints: async () => {
		const r = await client.GET('/api/traefik/entrypoints/pending');
		return expectData(r.response, r.error, r.data ?? []);
	},
	confirmTraefikEntryPoint: async (entryPointId: string) => {
		const r = await client.POST('/api/traefik/entrypoints/{entryPointId}/confirm', {
			params: { path: { entryPointId } }
		});
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
		const r = await client.GET('/api/status/endpoints' as never, {} as never);
		return (await expectData(r.response, r.error, r.data ?? [])) as import('./types.js').MonitorEndpoint[];
	},
	updateStatusEndpoint: async (
		endpointId: string,
		body: import('./types.js').UpdateMonitorEndpointRequest
	) => {
		const r = await client.PUT('/api/status/endpoints/{endpointId}' as never, {
			params: { path: { endpointId } },
			body
		} as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').MonitorEndpoint;
	},
	listStatusRollups: async (params?: {
		endpointId?: string;
		intervalMinutes?: number;
		hours?: number;
	}) => {
		const r = await client.GET('/api/status/rollups' as never, {
			params: {
				query: {
					endpointId: params?.endpointId,
					intervalMinutes: params?.intervalMinutes,
					hours: params?.hours
				}
			}
		} as never);
		return (await expectData(r.response, r.error, r.data ?? [])) as import('./types.js').MonitorRollup[];
	},
	listStatusEvents: async (params?: { endpointId?: string; hours?: number }) => {
		const r = await client.GET('/api/status/events' as never, {
			params: {
				query: {
					endpointId: params?.endpointId,
					hours: params?.hours
				}
			}
		} as never);
		return (await expectData(r.response, r.error, r.data ?? [])) as import('./types.js').MonitorEvent[];
	},
	getPublicStatusSummary: async () => {
		const r = await client.GET('/api/public/status/summary');
		return expectData(r.response, r.error, r.data);
	},
	getMonitoringSettings: async () => {
		const r = await client.GET('/api/settings/monitoring');
		return expectData(r.response, r.error, r.data);
	},
	updateMonitoringSettings: async (body: import('./types.js').MonitoringSettingsRequest) => {
		const r = await client.PUT('/api/settings/monitoring', { body });
		return expectData(r.response, r.error, r.data);
	},
	getEdgeSsoSettings: async () => {
		const r = await client.GET('/api/settings/edge-sso/session');
		return expectData(r.response, r.error, r.data);
	},
	updateEdgeSsoSettings: async (body: import('./types.js').EdgeSsoSettingsRequest) => {
		const r = await client.PUT('/api/settings/edge-sso/session', { body });
		return expectData(r.response, r.error, r.data);
	},
	getDashboardSettings: async () => {
		const r = await client.GET('/api/settings/dashboard' as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').DashboardSettings;
	},
	updateDashboardSettings: async (body: import('./types.js').DashboardSettingsRequest) => {
		const r = await client.PUT('/api/settings/dashboard' as never, { body } as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').DashboardSettings;
	},
	getCategorySettings: async (category: string) => {
		const r = await client.GET('/api/settings/categories/{category}' as never, {
			params: { path: { category } }
		} as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').CategorySettings;
	},
	updateCategorySettings: async (
		category: string,
		body: import('./types.js').CategorySettingsRequest
	) => {
		const r = await client.PUT('/api/settings/categories/{category}' as never, {
			params: { path: { category } },
			body
		} as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').CategorySettings;
	},
	getGeoIpSettings: async () => {
		const r = await client.GET('/api/settings/geoip' as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').GeoIpSettings;
	},
	updateGeoIpSettings: async (body: import('./types.js').GeoIpSettingsRequest) => {
		const r = await client.PUT('/api/settings/geoip' as never, { body } as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').GeoIpSettings;
	},
	runGeoIpUpdate: async () => {
		const r = await client.POST('/api/settings/geoip/update' as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').GeoIpUpdateResult;
	},
	getCaptchaSettings: async () => {
		const r = await client.GET('/api/security/captcha/settings' as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').CaptchaSettings;
	},
	updateCaptchaSettings: async (body: import('./types.js').CaptchaSettingsRequest) => {
		const r = await client.PUT('/api/security/captcha/settings' as never, { body } as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').CaptchaSettings;
	},
	testCaptchaToken: async (token: string) => {
		const r = await client.POST('/api/security/captcha/test' as never, { body: { token } } as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as {
			succeeded: boolean;
			status: string;
			error: string | null;
		};
	},
	getCaptchaChallengeStatus: async (returnUrl?: string | null) => {
		const r = await client.GET('/api/edge-challenge/status' as never, {
			params: { query: { returnUrl: returnUrl ?? undefined } }
		} as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').CaptchaChallengeStatus;
	},
	verifyCaptchaChallenge: async (token: string, returnUrl?: string | null) => {
		const r = await client.POST('/api/edge-challenge/verify' as never, {
			body: { token, returnUrl: returnUrl ?? null }
		} as never);
		if (!r.response.ok) throw errorFromResult(r.response.status, r.error);
		return (await readUndocumentedJson(r.response)) as import('./types.js').CaptchaChallengeVerifyResult;
	},
	getInternalAgentDnsSettings: async () => {
		const r = await client.GET('/api/settings/internal-agent-dns/' as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').InternalAgentDnsSettings;
	},
	updateInternalAgentDnsSettings: async (body: import('./types.js').InternalAgentDnsSettingsRequest) => {
		const r = await client.PUT('/api/settings/internal-agent-dns/' as never, { body } as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').InternalAgentDnsSettings;
	},
	previewInternalAgentDnsSync: async () => {
		const r = await client.POST('/api/settings/internal-agent-dns/preview-sync' as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').AdGuardRewritePlan;
	},
	applyInternalAgentDnsSync: async (body: import('./types.js').AdGuardRewriteApplyRequest) => {
		const r = await client.POST('/api/settings/internal-agent-dns/apply-sync' as never, {
			body
		} as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').AdGuardRewriteApply;
	},
	createEdgeSsoProvider: async (body: import('./types.js').CreateOidcProviderRequest) => {
		const result = await postUndocumented('/api/settings/edge-sso/providers', { body });
		return {
			id: String(result.id ?? ''),
			name: String(result.name ?? body.name),
			issuer: String(result.issuer ?? body.issuer),
			clientId: String(result.clientId ?? body.clientId),
			scopes: String(result.scopes ?? body.scopes ?? ''),
			enabled: result.enabled !== false,
			isDefault: result.isDefault === true || body.isDefault === true
		} satisfies import('./types.js').OidcProvider;
	},
	getSecurityDashboard: async (params?: {
		hours?: number;
		resource?: string;
		traefikHost?: string;
		firewallHostId?: string;
	}) => {
		const r = await client.GET('/api/security/dashboard' as never, {
			params: {
				query: {
					hours: params?.hours,
					resource: params?.resource,
					traefikHost: params?.traefikHost,
					firewallHostId: params?.firewallHostId
				}
			}
		} as never);
		return (await expectData(r.response, r.error, r.data)) as unknown as import('./types.js').SecurityDashboard;
	},
	listBlocklistSources: async () => {
		const r = await client.GET('/api/security/blocklists');
		return expectData(r.response, r.error, r.data ?? []) as Promise<BlocklistSourceResponse[]>;
	},
	createBlocklistSource: async (body: BlocklistSourceRequest) => {
		const r = await client.POST('/api/security/blocklists', { body });
		return expectData(r.response, r.error, r.data) as Promise<BlocklistSourceResponse>;
	},
	updateBlocklistSource: async (id: string, body: BlocklistSourceRequest) => {
		const r = await client.PATCH('/api/security/blocklists/{id}', {
			params: { path: { id } },
			body
		});
		return expectData(r.response, r.error, r.data) as Promise<BlocklistSourceResponse>;
	},
	previewBlocklistSource: async (id: string) => {
		const r = await client.POST('/api/security/blocklists/{id}/fetch-preview', {
			params: { path: { id } }
		});
		return expectData(r.response, r.error, r.data) as Promise<BlocklistFetchPreviewResponse>;
	},
	enableBlocklistSource: async (id: string) => {
		const r = await client.POST('/api/security/blocklists/{id}/enable', {
			params: { path: { id } }
		});
		return expectData(r.response, r.error, r.data) as Promise<BlocklistSourceMutationResponse>;
	},
	getPulseInstall: async (agentId: string, token?: string) => {
		const r = await client.GET('/api/pulse/agents/{agentId}/install', {
			params: { path: { agentId } }
		});
		const install = await expectData(r.response, r.error, r.data);
		if (!token) {
			return install;
		}

		return {
			...install,
			linuxInstallScript: install.linuxInstallScript.replaceAll('<PULSE_TOKEN>', token),
			dockerComposeSnippet: install.dockerComposeSnippet.replaceAll('<PULSE_TOKEN>', token)
		};
	},
	revokePulseAgent: async (agentId: string) => {
		const r = await client.POST('/api/pulse/agents/{agentId}/revoke', {
			params: { path: { agentId } }
		});
		await expectOk(r.response, r.error);
	},
	rotatePulseAgentToken: async (agentId: string) => {
		const body = await postUndocumented('/api/pulse/agents/{agentId}/rotate-token', {
			params: { path: { agentId } }
		});
		return body as import('./types.js').RotatePulseAgentResult;
	},
	listPulseAgents: async () => {
		const r = await client.GET('/api/pulse/agents');
		return expectData(r.response, r.error, r.data ?? []);
	},
	getPulseResolvedTargets: async (agentId: string) => {
		const r = await client.GET('/api/pulse/agents/{agentId}/resolved-targets', {
			params: { path: { agentId } }
		});
		return expectData(r.response, r.error, r.data);
	},
	createPulseAgent: async (body: import('./types.js').CreatePulseAgentRequest) => {
		const r = await client.POST('/api/pulse/agents', { body });
		return expectData(r.response, r.error, r.data);
	},
	listScripts: async () => {
		const r = await client.GET('/api/scripts');
		return expectData(r.response, r.error, r.data ?? []);
	},
	updateScript: async (scriptId: string, body: import('./types.js').UpdateScriptRequest) => {
		const r = await client.PUT('/api/scripts/{scriptId}', {
			params: { path: { scriptId } },
			body
		});
		return expectData(r.response, r.error, r.data);
	},
	deleteScript: async (scriptId: string) => {
		const r = await client.DELETE('/api/scripts/{scriptId}', {
			params: { path: { scriptId } }
		});
		await expectOk(r.response, r.error);
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
	testNotificationProvider: async (providerId: string, body: import('./types.js').NotificationTestRequest) => {
		const r = await client.POST('/api/settings/notifications/providers/{providerId}/test', {
			params: { path: { providerId } },
			body
		});
		return expectData(r.response, r.error, r.data);
	},
	discoverTelegramChat: async (botToken: string) => {
		const body = await postUndocumented('/api/settings/notifications/telegram/discover-chat', {
			body: { botToken }
		});
		return {
			found: body.found === true,
			chatId: typeof body.chatId === 'string' ? body.chatId : null,
			chatTitle: typeof body.chatTitle === 'string' ? body.chatTitle : null,
			error: typeof body.error === 'string' ? body.error : null
		} as import('./types.js').TelegramChatDiscoveryResponse;
	},
	deleteNotificationProvider: async (providerId: string) => {
		const r = await client.DELETE('/api/settings/notifications/providers/{providerId}', {
			params: { path: { providerId } }
		});
		await expectOk(r.response, r.error);
	},
	listNotificationRoutes: async () => {
		const r = await client.GET('/api/settings/notifications/routes');
		return expectData(r.response, r.error, r.data ?? []);
	},
	createNotificationRoute: async (body: import('./types.js').CreateNotificationRouteRequest) => {
		const r = await client.POST('/api/settings/notifications/routes', { body });
		return expectData(r.response, r.error, r.data);
	},
	updateNotificationRoute: async (routeId: string, body: import('./types.js').UpdateNotificationRouteRequest) => {
		const r = await client.PUT('/api/settings/notifications/routes/{routeId}', {
			params: { path: { routeId } },
			body
		});
		return expectData(r.response, r.error, r.data);
	},
	deleteNotificationRoute: async (routeId: string) => {
		const r = await client.DELETE('/api/settings/notifications/routes/{routeId}', {
			params: { path: { routeId } }
		});
		await expectOk(r.response, r.error);
	},
	getPublicApps: async () => {
		const r = await client.GET('/api/public/apps');
		return expectData(r.response, r.error, r.data) as Promise<import('./types.js').PublicDashboard>;
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
	testAdGuardConnection: async (connectionId: string) => {
		const r = await client.POST('/api/adguard/connections/{connectionId}/test', {
			params: { path: { connectionId } }
		});
		return expectData(r.response, r.error, r.data);
	},
	listAdGuardRewrites: async (connectionId: string) => {
		const r = await client.GET('/api/adguard/{connectionId}/rewrites', {
			params: { path: { connectionId } }
		});
		const rewrites = await expectData(r.response, r.error, r.data ?? []);
		return rewrites.filter((rewrite): rewrite is NonNullable<typeof rewrite> => rewrite !== null);
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
	},
	deleteAdGuardRewrite: async (connectionId: string, rewriteId: string) => {
		const r = await client.DELETE('/api/adguard/{connectionId}/rewrites/{rewriteId}', {
			params: { path: { connectionId, rewriteId } }
		});
		return expectData(r.response, r.error, r.data);
	},

	listSecurityProfiles: async () => {
		const r = await client.GET('/api/security/profiles');
		return expectData(r.response, r.error, r.data ?? []);
	},
	createSecurityProfile: async (body: import('./types.js').CreateSecurityProfileRequest) => {
		const r = await client.POST('/api/security/profiles', { body });
		return expectData(r.response, r.error, r.data);
	},
	updateSecurityProfile: async (name: string, body: import('./types.js').UpdateSecurityProfileRequest) => {
		const r = await client.PUT('/api/security/profiles/{name}', { params: { path: { name } }, body });
		return expectData(r.response, r.error, r.data);
	},
	deleteSecurityProfile: async (name: string) => {
		const r = await client.DELETE('/api/security/profiles/{name}', { params: { path: { name } } });
		if (!r.response.ok) throw errorFromResult(r.response.status, r.error);
	}
};
