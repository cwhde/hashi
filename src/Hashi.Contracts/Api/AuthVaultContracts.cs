namespace Hashi.Contracts.Api;

public sealed record BootstrapLoginRequest(string Username, string Password);

public sealed record BootstrapLoginResponse(bool Succeeded, string? Error);

public sealed record SessionStatusResponse(
    bool IsAuthenticated,
    string? AuthMethod,
    bool VaultUnlocked,
    bool SetupComplete,
    IReadOnlyList<string>? Scopes = null,
    string? BoundIp = null,
    DateTimeOffset? IdleExpiresAtUtc = null,
    DateTimeOffset? AbsoluteExpiresAtUtc = null,
    DateTimeOffset? ReauthenticatedAtUtc = null);

public sealed record AdminSessionSummaryResponse(
    string SessionId,
    string AuthMethod,
    string BoundIp,
    IReadOnlyList<string> Scopes,
    DateTimeOffset CreatedAtUtc,
    DateTimeOffset LastSeenAtUtc,
    DateTimeOffset IdleExpiresAtUtc,
    DateTimeOffset AbsoluteExpiresAtUtc,
    DateTimeOffset? ReauthenticatedAtUtc,
    bool IsCurrent);

public sealed record RevokeOtherSessionsResponse(int RevokedCount);

public sealed record AdminSessionSettingsResponse(
    int IdleTimeoutMinutes,
    int AbsoluteTimeoutMinutes,
    DateTimeOffset? UpdatedAtUtc);

public sealed record AdminSessionSettingsRequest(
    int? IdleTimeoutMinutes,
    int? AbsoluteTimeoutMinutes);

public sealed record PasskeyRegistrationBeginResponse(object Options, string ChallengeSessionId);

public sealed record PasskeyRegistrationCompleteRequest(
    object Attestation,
    string ChallengeSessionId,
    string Nickname,
    bool ClientReportsPrfSupported);

public sealed record PasskeyRegistrationCompleteResponse(Guid CredentialId, bool PrfSupported);

public sealed record PasskeyLoginBeginResponse(object Options, string ChallengeSessionId);

public sealed record PasskeyLoginCompleteRequest(
    object Assertion,
    string ChallengeSessionId,
    string? PrfOutputBase64);

public sealed record PasskeyLoginCompleteResponse(bool Succeeded, bool VaultUnlocked);

public sealed record VaultStatusResponse(
    string LockState,
    bool IsVaultConfigured,
    bool HasPasskey,
    bool PrfWrapAvailable,
    bool ServiceSyncVaultReady,
    bool BootstrapCredentialsActive);

public sealed record VaultSetupRequest(
    string RecoveryKey,
    bool PrfWrapAttempted,
    string? PrfOutputBase64,
    Guid? PasskeyCredentialId);

public sealed record VaultSetupResponse(
    bool Configured,
    bool PrfWrapStored,
    bool ServiceSyncWrapStored,
    string GeneratedRecoveryKey);

public sealed record VaultUnlockRequest(string RecoveryKey);

public sealed record VaultUnlockResponse(bool Unlocked);

public sealed record VaultGenerateRecoveryKeyResponse(string RecoveryKey);

public sealed record SecretStoreRequest(string Label, string Purpose, string PlaintextBase64);

public sealed record SecretDescriptorResponse(
    Guid Id,
    string Purpose,
    string Label,
    DateTimeOffset CreatedAtUtc,
    DateTimeOffset UpdatedAtUtc);

public sealed record SecretRevealResponse(string PlaintextBase64);

public sealed record SetupCompleteResponse(bool Succeeded, string? Error);

public sealed record LogoutResponse(bool LoggedOut);

public sealed record VaultLockResponse(bool Locked);

public sealed record VaultVerifyUnlockResponse(bool Verified, bool VaultUnlocked);
