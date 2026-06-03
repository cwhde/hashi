import type { components } from './schema.js';

export type Schemas = components['schemas'];

export type HealthResponse = Schemas['HealthResponse'];
export type SetupStatusResponse = Schemas['SetupStatusResponse'];
export type BootstrapAllowedResponse = Schemas['BootstrapAllowedResponse'];
export type GeneralSettings = Schemas['GeneralSettingsResponse'];
export type GeneralSettingsUpdate = Schemas['GeneralSettingsRequest'];
export type GeneralSettingsUpdateResult = Schemas['GeneralSettingsUpdateResponse'];
export type AuditEvent = Schemas['AuditEventResponse'];
export type BackgroundJob = Schemas['BackgroundJobResponse'];
export type DnsSyncPlan = Schemas['DnsSyncPlanResponse'];
export type BootstrapLoginRequest = Schemas['BootstrapLoginRequest'];
export type SessionStatus = Schemas['SessionStatusResponse'];
export type VaultStatus = Schemas['VaultStatusResponse'];
export type VaultSetupRequest = Schemas['VaultSetupRequest'];
export type VaultGenerateRecoveryKeyResponse = Schemas['VaultGenerateRecoveryKeyResponse'];
export type SecretDescriptor = Schemas['SecretDescriptorResponse'];

export type ConnectionSummary = Schemas['ConnectionSummaryResponse'];
export type CreateHetznerDnsConnectionRequest = Schemas['CreateHetznerDnsConnectionRequest'];
export type DnsProviderValidationRequest = Schemas['DnsProviderValidationRequest'];
export type DnsZone = {
	id: string;
	connectionId: string;
	providerZoneId: string;
	name: string;
	defaultTtl: number;
};
export type DnsRecord = Omit<Schemas['DnsRecordResponse'], 'ttl'> & { zoneId: string; ttl: number | null };
export type ManualDnsRecordRequest = {
	zoneId: string;
	name: string;
	type: string;
	value: string;
	ttl: number | null;
	enabled: boolean;
	dashboardEnabled: boolean;
	dashboardDisplayName: string | null;
};
export type DnsImportApplyRequest = Schemas['DnsImportApplyRequest'];
export type DnsImportDecision = Schemas['DnsImportDecisionResponse'];
export type DnsWriteValidationRequest = Schemas['DnsWriteValidationRequest'];
export type DnsSyncApplyRequest = Schemas['DnsSyncApplyRequest'];

export type CreateSshConnectionRequest = Schemas['CreateSshConnectionRequest'];
export type RemoteWriteRequest = Schemas['RemoteWriteRequest'];

export type Resource = Schemas['ResourceResponse'];
export type CreateResourceRequest = Schemas['CreateResourceRequest'];
export type UpdateResourceRequest = Schemas['UpdateResourceRequest'];
export type CertificateSetupRequest = Schemas['CertificateSetupRequest'];
export type TraefikEntryPoint = Schemas['TraefikEntryPointResponse'];

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
export type UpdateMonitorEndpointRequest = Schemas['UpdateMonitorEndpointRequest'];
export type MonitorRollup = Schemas['MonitorRollupResponse'];
export type PublicStatusItem = Schemas['PublicStatusItemResponse'];
export type PublicDashboard = Schemas['PublicDashboardResponse'];
export type PublicDashboardItem = Schemas['PublicDashboardItemResponse'];
export type PublicStatusStripBucket = Schemas['PublicStatusStripBucket'];
export type SecurityRankItem = { label: string; count: number };
export type SecurityResourceEnforcementItem = {
	resource: string;
	blocked: number;
	challenged: number;
};
export type SecurityRecentEventItem = {
	occurredAtUtc: string;
	category: string;
	action: string;
	clientIp: string | null;
	host: string | null;
	path: string | null;
};
export type SecurityFilterOption = {
	value: string;
	label: string;
};
export type SecurityFirewallHostOption = {
	id: string;
	name: string;
	domain: string;
	linkedTraefikHost: string;
};
export type SecurityTopBlockedIpItem = {
	ip: string;
	count: number;
	lastSeenAtUtc: string;
	countryCode: string | null;
	asn: string | null;
	reason: string | null;
	expiresAtUtc: string | null;
	subjectState: string | null;
};
export type SecurityDashboard = {
	allowed: number;
	blocked: number;
	challenged: number;
	wafDetections: number;
	wafBlocks: number;
	hours: number;
	resourceFilter: string | null;
	traefikHostFilter: string | null;
	firewallHostIdFilter: string | null;
	topBlockedIps: SecurityTopBlockedIpItem[];
	topCountries: SecurityRankItem[];
	topAsns: SecurityRankItem[];
	topResourcesBlockedChallenged: SecurityResourceEnforcementItem[];
	recentEvents: SecurityRecentEventItem[];
	resourceFilters: SecurityFilterOption[];
	traefikHostFilters: SecurityFilterOption[];
	firewallHostFilters: SecurityFirewallHostOption[];
	firewallActiveIpBlocks: number;
	blocklistCount: number;
	securityEventCount: number;
};
export type PulseAgent = Schemas['PulseAgentResponse'];
export type PulseInstall = Schemas['PulseInstallResponse'];
export type CreatePulseAgentRequest = Schemas['CreatePulseAgentRequest'];
export type CreatePulseAgentResult = Schemas['CreatePulseAgentResponse'];
export type Script = Schemas['ScriptResponse'];
export type CreateScriptRequest = Schemas['CreateScriptRequest'];
export type UpdateScriptRequest = Schemas['UpdateScriptRequest'];
export type RunScriptRequest = Schemas['RunScriptRequest'];
export type RunScriptResponse = Schemas['RunScriptResponse'];
export type NotificationProvider = Schemas['NotificationProviderResponse'];
export type CreateNotificationProviderRequest = Schemas['CreateNotificationProviderRequest'];
export type NotificationTestRequest = Schemas['NotificationTestRequest'];
export type TelegramChatDiscoveryResponse = {
	found: boolean;
	chatId: string | null;
	chatTitle: string | null;
	error: string | null;
};
export type AdGuardConnection = Schemas['AdGuardConnectionResponse'];
export type CreateAdGuardConnectionRequest = Schemas['CreateAdGuardConnectionRequest'];
export type AdGuardRewrite = NonNullable<Schemas['AdGuardRewriteResponse']>;
export type AdGuardRewriteMutation = Schemas['AdGuardRewriteMutationResponse'];
export type AdGuardRewriteApply = Schemas['AdGuardRewriteApplyResponse'];
export type UpsertAdGuardRewriteRequest = Schemas['UpsertAdGuardRewriteRequest'];

export type MonitoringSettingsRequest = Schemas['MonitoringSettingsRequest'];
export type EdgeSsoSettingsRequest = Schemas['EdgeSsoSettingsRequest'];
export type DashboardSettings = Schemas['DashboardSettingsResponse'];
export type DashboardSettingsRequest = Schemas['DashboardSettingsRequest'];
export type CategorySettings = Schemas['CategorySettingsResponse'];
export type CategorySettingsRequest = Schemas['CategorySettingsRequest'];
export type GeoIpDatabase = Schemas['GeoIpDatabaseResponse'];
export type GeoIpSettings = Schemas['GeoIpSettingsResponse'];
export type GeoIpSettingsRequest = Schemas['GeoIpSettingsRequest'];
export type GeoIpUpdateResult = Schemas['GeoIpUpdateResponse'];
export type OidcProvider = Schemas['OidcProviderResponse'];
export type CreateOidcProviderRequest = Schemas['CreateOidcProviderRequest'];
export type RotatePulseAgentResult = Schemas['CreatePulseAgentResponse'];

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
