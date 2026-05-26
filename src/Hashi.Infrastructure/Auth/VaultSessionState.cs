using System.Security.Cryptography;
using Hashi.Core.Auth;

namespace Hashi.Infrastructure.Auth;

public sealed class VaultSessionState
{
    public byte[]? AdminRootKey { get; private set; }

    public bool IsUnlocked => AdminRootKey is not null;

    public void Unlock(ReadOnlySpan<byte> adminRootKey)
    {
        AdminRootKey = adminRootKey.ToArray();
    }

    public ReadOnlySpan<byte> GetRootKeyOrThrow()
    {
        if (AdminRootKey is null)
        {
            throw new InvalidOperationException("Vault is locked.");
        }

        return AdminRootKey;
    }

    public void Lock()
    {
        if (AdminRootKey is not null)
        {
            CryptographicOperations.ZeroMemory(AdminRootKey);
            AdminRootKey = null;
        }
    }
}

public sealed class ServiceSyncVaultState
{
    public byte[]? WrapKey { get; private set; }

    public bool IsReady => WrapKey is not null;

    public void Initialize(ReadOnlySpan<byte> wrapKey)
    {
        WrapKey = wrapKey.ToArray();
    }

    public ReadOnlySpan<byte> GetWrapKeyOrThrow()
    {
        if (WrapKey is null)
        {
            throw new InvalidOperationException("Service-sync vault is not configured.");
        }

        return WrapKey;
    }
}

public static class SecretPurposeMapping
{
    public static string ToName(SecretPurpose purpose) => purpose switch
    {
        SecretPurpose.SshCredential => Persistence.Entities.SecretPurposeNames.SshCredential,
        SecretPurpose.DnsProviderToken => Persistence.Entities.SecretPurposeNames.DnsProviderToken,
        SecretPurpose.AcmeEab => Persistence.Entities.SecretPurposeNames.AcmeEab,
        SecretPurpose.AdGuardCredential => Persistence.Entities.SecretPurposeNames.AdGuardCredential,
        SecretPurpose.NotificationToken => Persistence.Entities.SecretPurposeNames.NotificationToken,
        SecretPurpose.OidcClientSecret => Persistence.Entities.SecretPurposeNames.OidcClientSecret,
        SecretPurpose.ScriptEnvironment => Persistence.Entities.SecretPurposeNames.ScriptEnvironment,
        _ => Persistence.Entities.SecretPurposeNames.Generic,
    };

    public static SecretPurpose FromName(string name) => name switch
    {
        Persistence.Entities.SecretPurposeNames.SshCredential => SecretPurpose.SshCredential,
        Persistence.Entities.SecretPurposeNames.DnsProviderToken => SecretPurpose.DnsProviderToken,
        Persistence.Entities.SecretPurposeNames.AcmeEab => SecretPurpose.AcmeEab,
        Persistence.Entities.SecretPurposeNames.AdGuardCredential => SecretPurpose.AdGuardCredential,
        Persistence.Entities.SecretPurposeNames.NotificationToken => SecretPurpose.NotificationToken,
        Persistence.Entities.SecretPurposeNames.OidcClientSecret => SecretPurpose.OidcClientSecret,
        Persistence.Entities.SecretPurposeNames.ScriptEnvironment => SecretPurpose.ScriptEnvironment,
        _ => SecretPurpose.Generic,
    };
}
