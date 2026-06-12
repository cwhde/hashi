using System.Collections.Concurrent;
using System.Security.Claims;
using System.Security.Cryptography;
using Hashi.Core.Auth;
using Microsoft.AspNetCore.Http;

namespace Hashi.Infrastructure.Auth;

public sealed class VaultSessionState(IHttpContextAccessor? httpContextAccessor = null)
{
    private const string LocalTestSessionKey = "local-test-session";
    private readonly ConcurrentDictionary<string, byte[]> _adminRootKeys = new();

    public bool IsUnlocked => TryGetCurrentSessionKey(out var sessionKey) && _adminRootKeys.ContainsKey(sessionKey);

    public void Unlock(ReadOnlySpan<byte> adminRootKey)
        => UnlockForSession(GetRequiredCurrentSessionKey(), adminRootKey);

    public void UnlockForSession(string sessionKey, ReadOnlySpan<byte> adminRootKey)
    {
        var key = adminRootKey.ToArray();
        if (_adminRootKeys.TryGetValue(sessionKey, out var existing))
        {
            CryptographicOperations.ZeroMemory(existing);
        }

        _adminRootKeys[sessionKey] = key;
    }

    public ReadOnlySpan<byte> GetRootKeyOrThrow()
    {
        var sessionKey = GetRequiredCurrentSessionKey();
        if (!_adminRootKeys.TryGetValue(sessionKey, out var adminRootKey))
        {
            throw new InvalidOperationException("Vault is locked.");
        }

        return adminRootKey;
    }

    public void Lock()
    {
        if (!TryGetCurrentSessionKey(out var sessionKey))
        {
            return;
        }

        if (_adminRootKeys.TryRemove(sessionKey, out var adminRootKey))
        {
            CryptographicOperations.ZeroMemory(adminRootKey);
        }
    }

    public void LockForSession(string sessionKey)
    {
        if (_adminRootKeys.TryRemove(sessionKey, out var adminRootKey))
        {
            CryptographicOperations.ZeroMemory(adminRootKey);
        }
    }

    private string GetRequiredCurrentSessionKey()
        => TryGetCurrentSessionKey(out var sessionKey)
            ? sessionKey
            : throw new InvalidOperationException("Vault unlock requires an authenticated admin session.");

    private bool TryGetCurrentSessionKey(out string sessionKey)
    {
        var context = httpContextAccessor?.HttpContext;
        sessionKey = context?.User.FindFirst(ClaimTypes.NameIdentifier)?.Value
            ?? context?.User.FindFirst(ClaimTypes.Sid)?.Value
            ?? context?.Request.Cookies["hashi.session"]
            ?? string.Empty;

        if (!string.IsNullOrWhiteSpace(sessionKey))
        {
            return true;
        }

        if (httpContextAccessor is null)
        {
            sessionKey = LocalTestSessionKey;
            return true;
        }

        return false;
    }
}

public sealed class ServiceSyncVaultState
{
    public byte[]? WrapKey { get; private set; }

    private readonly Dictionary<string, byte[]> _purposeKeys = new(StringComparer.OrdinalIgnoreCase);

    public bool IsReady => WrapKey is not null;

    public bool IsUnlocked { get; set; }

    public void Initialize(ReadOnlySpan<byte> wrapKey)
    {
        WrapKey = wrapKey.ToArray();
    }

    public void InitializePurposeKey(string purposeTag, ReadOnlySpan<byte> purposeKey)
    {
        if (_purposeKeys.TryGetValue(purposeTag, out var existing))
        {
            CryptographicOperations.ZeroMemory(existing);
        }

        _purposeKeys[purposeTag] = purposeKey.ToArray();
    }

    public ReadOnlySpan<byte> GetWrapKeyOrThrow()
    {
        if (WrapKey is null)
        {
            throw new InvalidOperationException("Service-sync vault is not configured.");
        }

        return WrapKey;
    }

    public ReadOnlySpan<byte> GetPurposeWrapKeyOrThrow(string purposeTag)
    {
        if (_purposeKeys.TryGetValue(purposeTag, out var key))
        {
            return key;
        }

        return GetWrapKeyOrThrow();
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
        SecretPurpose.MaxMindLicenseKey => Persistence.Entities.SecretPurposeNames.MaxMindLicenseKey,
        SecretPurpose.CapSecretKey => Persistence.Entities.SecretPurposeNames.CapSecretKey,
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
        Persistence.Entities.SecretPurposeNames.MaxMindLicenseKey => SecretPurpose.MaxMindLicenseKey,
        Persistence.Entities.SecretPurposeNames.CapSecretKey => SecretPurpose.CapSecretKey,
        Persistence.Entities.SecretPurposeNames.ScriptEnvironment => SecretPurpose.ScriptEnvironment,
        _ => SecretPurpose.Generic,
    };
}
