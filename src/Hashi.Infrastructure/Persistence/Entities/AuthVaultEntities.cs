namespace Hashi.Infrastructure.Persistence.Entities;

public sealed class PasskeyCredentialEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public byte[] CredentialId { get; set; } = [];

    public byte[] PublicKey { get; set; } = [];

    public uint SignCount { get; set; }

    public string Nickname { get; set; } = "Primary passkey";

    public bool PrfSupported { get; set; }

    public byte[]? PrfSalt { get; set; }

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class VaultWrappedKeyEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string WrapMethod { get; set; } = string.Empty;

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

    public byte[] AdminWrappedDekBlob { get; set; } = [];

    public byte[]? ServiceWrappedDekBlob { get; set; }

    public bool IsServiceSyncEligible { get; set; }

    public byte[] CiphertextBlob { get; set; } = [];

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;

    public DateTimeOffset UpdatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
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
    public const string ScriptEnvironment = "script_environment";
    public const string Generic = "generic";
}
