import type { components } from './schema.js';

export type Schemas = components['schemas'];

export type HealthResponse = Schemas['HealthResponse'];
export type SetupStatusResponse = Schemas['SetupStatusResponse'];
export type BootstrapAllowedResponse = Schemas['BootstrapAllowedResponse'];
export type GeneralSettings = Schemas['GeneralSettingsResponse'];
export type GeneralSettingsUpdate = Schemas['GeneralSettingsRequest'];
export type GeneralSettingsUpdateResult = Schemas['GeneralSettingsUpdateResponse'];
export type AuditEvent = Schemas['AuditEventResponse'];
export type BootstrapLoginRequest = Schemas['BootstrapLoginRequest'];
export type SessionStatus = Schemas['SessionStatusResponse'];
export type VaultStatus = Schemas['VaultStatusResponse'];
export type VaultSetupRequest = Schemas['VaultSetupRequest'];
export type VaultGenerateRecoveryKeyResponse = Schemas['VaultGenerateRecoveryKeyResponse'];
export type SecretDescriptor = Schemas['SecretDescriptorResponse'];

export type ConnectionSummary = Schemas['ConnectionSummaryResponse'];
export type CreateHetznerDnsConnectionRequest = Schemas['CreateHetznerDnsConnectionRequest'];
export type DnsProviderValidationRequest = Schemas['DnsProviderValidationRequest'];
export type DnsRecord = Schemas['DnsRecordResponse'];
export type DnsImportApplyRequest = Schemas['DnsImportApplyRequest'];
export type DnsImportDecision = Schemas['DnsImportDecisionResponse'];
export type DnsWriteValidationRequest = Schemas['DnsWriteValidationRequest'];
export type DnsSyncApplyRequest = Schemas['DnsSyncApplyRequest'];

export type CreateSshConnectionRequest = Schemas['CreateSshConnectionRequest'];
export type RemoteWriteRequest = Schemas['RemoteWriteRequest'];

export type Resource = Schemas['ResourceResponse'];
export type CreateResourceRequest = Schemas['CreateResourceRequest'];
export type UpdateResourceRequest = Schemas['UpdateResourceRequest'];

export type TraefikRenderResponse = Schemas['TraefikRenderResponse'];
export type TraefikApplyRequest = Schemas['TraefikApplyRequest'];
export type TraefikApplyResponse = Schemas['TraefikApplyResponse'];
export type TraefikApplyConnectionRequest = Schemas['TraefikApplyConnectionRequest'];
export type TraefikHostState = Schemas['TraefikHostStateResponse'];
export type TraefikDetectExistingResponse = Schemas['TraefikDetectExistingResponse'];
export type TraefikConfigValidationResponse = Schemas['TraefikConfigValidationResponse'];
export type TraefikUserMiddleware = Schemas['TraefikUserMiddlewareResponse'];
export type UpdateTraefikUserMiddlewareRequest = Schemas['UpdateTraefikUserMiddlewareRequest'];
export type TraefikUserMiddlewareValidationRequest = Schemas['TraefikUserMiddlewareValidationRequest'];
export type TraefikUserMiddlewareValidationResponse = Schemas['TraefikUserMiddlewareValidationResponse'];
export type FirewallRenderRequest = Schemas['FirewallRenderRequest'];
export type CreateFirewallHostRequest = Schemas['CreateFirewallHostRequest'];
export type FirewallRenderResponse = Schemas['FirewallRenderResponse'];
export type FirewallHost = Schemas['FirewallHostResponse'];

export type MonitorEndpoint = Schemas['MonitorEndpointResponse'];
export type MonitorRollup = Schemas['MonitorRollupResponse'];
export type PublicStatusItem = Schemas['PublicStatusItemResponse'];
export type PublicStatusStripBucket = Schemas['PublicStatusStripBucket'];
export type SecurityDashboard = Schemas['SecurityDashboardResponse'];
export type SecurityRankItem = { label: string; count: number };
export type PulseAgent = Schemas['PulseAgentResponse'];
export type PulseInstall = Schemas['PulseInstallResponse'];
export type CreatePulseAgentRequest = Schemas['CreatePulseAgentRequest'];
export type CreatePulseAgentResult = Schemas['CreatePulseAgentResponse'];
export type Script = Schemas['ScriptResponse'];
export type CreateScriptRequest = Schemas['CreateScriptRequest'];
export type RunScriptRequest = Schemas['RunScriptRequest'];
export type RunScriptResponse = Schemas['RunScriptResponse'];
export type NotificationProvider = Schemas['NotificationProviderResponse'];
export type CreateNotificationProviderRequest = Schemas['CreateNotificationProviderRequest'];
export type AdGuardConnection = Schemas['AdGuardConnectionResponse'];
export type CreateAdGuardConnectionRequest = Schemas['CreateAdGuardConnectionRequest'];
export type AdGuardRewrite = Schemas['AdGuardRewriteResponse'];
export type UpsertAdGuardRewriteRequest = Schemas['UpsertAdGuardRewriteRequest'];

export type SyncRun = Schemas['SyncRunResponse'];
export type SyncPlanPreview = Schemas['SyncPlanPreviewResponse'];
export type SyncReconcileResult = Schemas['SyncReconcileResponse'];
export type SyncApplyRequest = Schemas['SyncApplyRequest'];

export type SystemResourceSyncResult = {
	succeeded: boolean;
	runId: string;
	riskLevel?: string | null;
	requiresConfirmation?: boolean;
	previewMarkdown?: string | null;
	message?: string | null;
};

export type ApiError = {
	error?: string;
	message?: string;
};

export type UndocumentedJson = Record<string, unknown>;
