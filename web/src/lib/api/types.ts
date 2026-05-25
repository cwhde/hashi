export type HealthResponse = {
	status: string;
	version: string;
	timestamp: string;
};

export type SetupStatusResponse = {
	isComplete: boolean;
	currentStep: string;
	completedSteps: string[];
	updatedAtUtc: string | null;
};

export type BootstrapAllowedResponse = {
	allowed: boolean;
	remoteIp: string | null;
};

export type GeneralSettings = {
	rootDomain: string | null;
	adminDomain: string | null;
	internalUrl: string | null;
	defaultSyncIntervalMinutes: number;
	publicDashboardEnabled: boolean;
	publicStatusEnabled: boolean;
	theme: string | null;
	updatedAtUtc: string | null;
};

export type GeneralSettingsUpdate = Partial<{
	rootDomain: string;
	adminDomain: string;
	internalUrl: string;
	defaultSyncIntervalMinutes: number;
	publicDashboardEnabled: boolean;
	publicStatusEnabled: boolean;
	theme: string;
}>;

export type AuditEvent = {
	id: string;
	category: string;
	action: string;
	subjectType: string | null;
	subjectId: string | null;
	outcome: string;
	createdAtUtc: string;
};

export type ApiError = {
	error?: string;
	message?: string;
};

export type BootstrapLoginResponse = {
	succeeded: boolean;
	error: string | null;
};

export type SessionStatus = {
	isAuthenticated: boolean;
	authMethod: string | null;
	vaultUnlocked: boolean;
	setupComplete: boolean;
};

export type PasskeyBeginResponse = {
	options: Record<string, unknown>;
	challengeSessionId: string;
};

export type PasskeyRegistrationCompleteResponse = {
	credentialId: string;
	prfSupported: boolean;
};

export type PasskeyLoginCompleteResponse = {
	succeeded: boolean;
	vaultUnlocked: boolean;
};

export type VaultStatus = {
	lockState: string;
	isVaultConfigured: boolean;
	hasPasskey: boolean;
	prfWrapAvailable: boolean;
	serviceSyncVaultReady: boolean;
	bootstrapCredentialsActive: boolean;
};

export type VaultSetupResponse = {
	configured: boolean;
	prfWrapStored: boolean;
	serviceSyncWrapStored: boolean;
	generatedRecoveryKey: string;
};

export type SetupCompleteResponse = {
	succeeded: boolean;
	error: string | null;
};
