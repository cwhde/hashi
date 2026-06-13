namespace Hashi.Infrastructure.Persistence.Entities;

public sealed class PasskeyCredentialEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public byte[] CredentialId { get; set; } = [];

    public string CredentialIdBase64 { get; set; } = string.Empty;

    public byte[] PublicKey { get; set; } = [];

    public uint SignCount { get; set; }

    public string Nickname { get; set; } = "Primary passkey";

    public bool PrfSupported { get; set; }

    public byte[]? PrfSalt { get; set; }

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class AdminSessionEntity
{
    public string Id { get; set; } = string.Empty;

    public string AuthMethod { get; set; } = string.Empty;

    public Guid? PasskeyCredentialId { get; set; }

    public PasskeyCredentialEntity? PasskeyCredential { get; set; }

    public string BoundIp { get; set; } = string.Empty;

    public string ScopesJson { get; set; } = "[]";

    public int IdleTimeoutMinutes { get; set; } = 240;

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;

    public DateTimeOffset LastSeenAtUtc { get; set; } = DateTimeOffset.UtcNow;

    public DateTimeOffset IdleExpiresAtUtc { get; set; }

    public DateTimeOffset AbsoluteExpiresAtUtc { get; set; }

    public DateTimeOffset? ReauthenticatedAtUtc { get; set; }

    public DateTimeOffset? RevokedAtUtc { get; set; }

    public string? RevocationReason { get; set; }

    public string? UserAgentHash { get; set; }
}

public sealed class VaultWrappedKeyEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string WrapMethod { get; set; } = string.Empty;

    public string? PurposeTag { get; set; }

    public byte[] WrappedKeyBlob { get; set; } = [];

    public string? RecoveryKeyHash { get; set; }

    public Guid? PasskeyCredentialId { get; set; }

    public PasskeyCredentialEntity? PasskeyCredential { get; set; }

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class SecretRecordEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string Purpose { get; set; } = string.Empty;

    public string Label { get; set; } = string.Empty;

    public string SecretClass { get; set; } = SecretClassNames.ServiceSync;

    public string? PurposeKeyTag { get; set; }

    public byte[] AdminWrappedDekBlob { get; set; } = [];

    public byte[]? ServiceWrappedDekBlob { get; set; }

    public byte[]? PurposeWrappedDekBlob { get; set; }

    public bool IsServiceSyncEligible { get; set; }

    public byte[] CiphertextBlob { get; set; } = [];

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;

    public DateTimeOffset UpdatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public static class SecretClassNames
{
    public const string SessionUnlocked = "session_unlocked";
    public const string ServiceSync = "service_sync";
    public const string ServerOperational = "server_operational";
}

public static class SecretPurposeKeyNames
{
    public const string Ssh = "ssh";
    public const string Dns = "dns";
    public const string Oidc = "oidc";
    public const string Acme = "acme";
    public const string AdGuard = "adguard";
    public const string Notification = "notification";
    public const string Cap = "cap";
    public const string MaxMind = "maxmind";
    public const string Script = "script";
    public const string Generic = "generic";
}

public static class VaultWrapMethodNames
{
    public const string RecoveryKey = "recovery_key";
    public const string PasskeyPrf = "passkey_prf";
    public const string ServiceSync = "service_sync";
}

public static class SecretPurposeNames
{
    public const string SshCredential = "ssh_credential";
    public const string DnsProviderToken = "dns_provider_token";
    public const string AcmeEab = "acme_eab";
    public const string AdGuardCredential = "adguard_credential";
    public const string NotificationToken = "notification_token";
    public const string OidcClientSecret = "oidc_client_secret";
    public const string MaxMindLicenseKey = "maxmind_license_key";
    public const string CapSecretKey = "cap_secret_key";
    public const string ScriptEnvironment = "script_environment";
    public const string Generic = "generic";
}
