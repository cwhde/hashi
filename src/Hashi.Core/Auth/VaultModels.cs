namespace Hashi.Core.Auth;

public enum VaultLockState
{
    NotConfigured,
    Locked,
    Unlocked,
}

public enum SecretPurpose
{
    SshCredential,
    DnsProviderToken,
    AcmeEab,
    AdGuardCredential,
    NotificationToken,
    OidcClientSecret,
    ScriptEnvironment,
    Generic,
}

public sealed record VaultStatus(
    VaultLockState LockState,
    bool IsVaultConfigured,
    bool HasPasskey,
    bool PrfWrapAvailable,
    bool ServiceSyncVaultReady,
    bool BootstrapCredentialsActive);

public sealed record StoredSecretDescriptor(
    Guid Id,
    SecretPurpose Purpose,
    string Label,
    DateTimeOffset CreatedAtUtc,
    DateTimeOffset UpdatedAtUtc);

public static class AdminAuthMethods
{
    public const string Bootstrap = "bootstrap";
    public const string Passkey = "passkey";
}

public static class AdminClaimTypes
{
    public const string AuthMethod = "hashi:auth_method";
    public const string VaultUnlocked = "hashi:vault_unlocked";
}
