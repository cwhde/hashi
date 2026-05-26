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
export type DnsSyncApplyRequest = Schemas['DnsSyncApplyRequest'];

export type CreateSshConnectionRequest = Schemas['CreateSshConnectionRequest'];
export type RemoteWriteRequest = Schemas['RemoteWriteRequest'];

export type Resource = Schemas['ResourceResponse'];
export type CreateResourceRequest = Schemas['CreateResourceRequest'];
export type UpdateResourceRequest = Schemas['UpdateResourceRequest'];

export type TraefikRenderResponse = Schemas['TraefikRenderResponse'];
export type FirewallRenderRequest = Schemas['FirewallRenderRequest'];
export type FirewallRenderResponse = Schemas['FirewallRenderResponse'];

export type MonitorEndpoint = Schemas['MonitorEndpointResponse'];
export type PublicStatusItem = Schemas['PublicStatusItemResponse'];
export type SecurityDashboard = Schemas['SecurityDashboardResponse'];
export type PulseAgent = Schemas['PulseAgentResponse'];
export type Script = Schemas['ScriptResponse'];
export type NotificationProvider = Schemas['NotificationProviderResponse'];

export type ApiError = {
	error?: string;
	message?: string;
};

export type UndocumentedJson = Record<string, unknown>;
