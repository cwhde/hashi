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
    MaxMindLicenseKey,
    CapSecretKey,
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

public static class AdminSessionScopes
{
    public const string Read = "admin.read";
    public const string Write = "admin.write";
    public const string SettingsManage = "settings.manage";
    public const string SecretsManage = "secrets.manage";
    public const string SyncApply = "sync.apply";
    public const string FirewallApply = "firewall.apply";
    public const string ScriptsManage = "scripts.manage";
    public const string SecurityManage = "security.manage";

    public static readonly IReadOnlyList<string> All =
    [
        Read,
        Write,
        SettingsManage,
        SecretsManage,
        SyncApply,
        FirewallApply,
        ScriptsManage,
        SecurityManage,
    ];

    public static readonly IReadOnlyList<string> Bootstrap =
    [
        Read,
        Write,
        SettingsManage,
        SecretsManage,
        SyncApply,
        FirewallApply,
        SecurityManage,
    ];
}
