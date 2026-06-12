using System.Security.Cryptography;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Crypto;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Auth;

public sealed class SecretRecordService(
    HashiDbContext db,
    VaultSessionState session,
    ServiceSyncVaultState serviceSync)
{
    public async Task<StoredSecretDescriptor> StoreAsync(
        SecretPurpose purpose,
        string label,
        byte[] plaintext,
        CancellationToken cancellationToken = default,
        bool serviceSyncEligible = false)
    {
        if (!session.IsUnlocked)
        {
            throw new InvalidOperationException("Vault must be unlocked to store secrets.");
        }

        var secretClass = MapPurposeToClass(purpose);
        var purposeKeyTag = MapPurposeToKeyTag(purpose);

        var dek = RandomNumberGenerator.GetBytes(32);
        try
        {
            var ciphertext = AesGcmCipher.Encrypt(plaintext, dek).ToBlob();
            var adminWrapped = AesGcmCipher.Encrypt(dek, session.GetRootKeyOrThrow()).ToBlob();
            byte[]? serviceWrapped = null;
            byte[]? purposeWrapped = null;

            if (serviceSyncEligible && serviceSync.IsReady)
            {
                serviceWrapped = AesGcmCipher.Encrypt(dek, serviceSync.GetWrapKeyOrThrow()).ToBlob();
            }

            if (!string.IsNullOrEmpty(purposeKeyTag) && serviceSync.IsReady)
            {
                var purposeKey = serviceSync.GetPurposeWrapKeyOrThrow(purposeKeyTag);
                purposeWrapped = AesGcmCipher.Encrypt(dek, purposeKey).ToBlob();
            }

            var entity = new SecretRecordEntity
            {
                Purpose = SecretPurposeMapping.ToName(purpose),
                Label = label,
                SecretClass = secretClass,
                PurposeKeyTag = purposeKeyTag,
                AdminWrappedDekBlob = adminWrapped,
                ServiceWrappedDekBlob = serviceWrapped,
                PurposeWrappedDekBlob = purposeWrapped,
                IsServiceSyncEligible = serviceSyncEligible,
                CiphertextBlob = ciphertext,
            };
            db.SecretRecords.Add(entity);
            await db.SaveChangesAsync(cancellationToken);

            return new StoredSecretDescriptor(entity.Id, purpose, label, entity.CreatedAtUtc, entity.UpdatedAtUtc);
        }
        finally
        {
            CryptographicOperations.ZeroMemory(dek);
        }
    }

    public async Task<IReadOnlyList<StoredSecretDescriptor>> ListAsync(CancellationToken cancellationToken = default)
    {
        return await db.SecretRecords
            .AsNoTracking()
            .OrderBy(x => x.Label)
            .Select(x => new StoredSecretDescriptor(
                x.Id,
                SecretPurposeMapping.FromName(x.Purpose),
                x.Label,
                x.CreatedAtUtc,
                x.UpdatedAtUtc))
            .ToListAsync(cancellationToken);
    }

    public async Task<byte[]?> DecryptForAdminAsync(Guid secretId, CancellationToken cancellationToken = default)
    {
        if (!session.IsUnlocked)
        {
            return null;
        }

        return await DecryptWithAdminWrapAsync(secretId, cancellationToken);
    }

    public async Task<byte[]?> DecryptForServiceSyncAsync(Guid secretId, CancellationToken cancellationToken = default)
    {
        if (!serviceSync.IsReady)
        {
            return null;
        }

        var entity = await db.SecretRecords.AsNoTracking().SingleOrDefaultAsync(x => x.Id == secretId, cancellationToken);
        if (entity is null || !entity.IsServiceSyncEligible || entity.ServiceWrappedDekBlob is null)
        {
            return null;
        }

        var dek = AesGcmCipher.Decrypt(entity.ServiceWrappedDekBlob, serviceSync.GetWrapKeyOrThrow());
        try
        {
            return AesGcmCipher.Decrypt(entity.CiphertextBlob, dek);
        }
        finally
        {
            CryptographicOperations.ZeroMemory(dek);
        }
    }

    public async Task<byte[]?> DecryptForPurposeAsync(Guid secretId, CancellationToken cancellationToken = default)
    {
        var entity = await db.SecretRecords.AsNoTracking().SingleOrDefaultAsync(x => x.Id == secretId, cancellationToken);
        if (entity is null)
        {
            return null;
        }

        if (session.IsUnlocked)
        {
            return await DecryptWithAdminWrapAsync(secretId, cancellationToken);
        }

        if (entity.PurposeWrappedDekBlob is not null && serviceSync.IsReady && !string.IsNullOrEmpty(entity.PurposeKeyTag))
        {
            var purposeKey = serviceSync.GetPurposeWrapKeyOrThrow(entity.PurposeKeyTag);
            var dek = AesGcmCipher.Decrypt(entity.PurposeWrappedDekBlob, purposeKey);
            try
            {
                return AesGcmCipher.Decrypt(entity.CiphertextBlob, dek);
            }
            finally
            {
                CryptographicOperations.ZeroMemory(dek);
            }
        }

        return await DecryptForServiceSyncAsync(secretId, cancellationToken);
    }

    private async Task<byte[]?> DecryptWithAdminWrapAsync(Guid secretId, CancellationToken cancellationToken)
    {
        var entity = await db.SecretRecords.AsNoTracking().SingleOrDefaultAsync(x => x.Id == secretId, cancellationToken);
        if (entity is null)
        {
            return null;
        }

        var dek = AesGcmCipher.Decrypt(entity.AdminWrappedDekBlob, session.GetRootKeyOrThrow());
        try
        {
            return AesGcmCipher.Decrypt(entity.CiphertextBlob, dek);
        }
        finally
        {
            CryptographicOperations.ZeroMemory(dek);
        }
    }

    private static string MapPurposeToClass(SecretPurpose purpose) => purpose switch
    {
        SecretPurpose.SshCredential => SecretClassNames.ServiceSync,
        SecretPurpose.DnsProviderToken => SecretClassNames.ServiceSync,
        SecretPurpose.AcmeEab => SecretClassNames.ServiceSync,
        SecretPurpose.AdGuardCredential => SecretClassNames.ServiceSync,
        SecretPurpose.NotificationToken => SecretClassNames.ServiceSync,
        SecretPurpose.OidcClientSecret => SecretClassNames.SessionUnlocked,
        SecretPurpose.MaxMindLicenseKey => SecretClassNames.ServiceSync,
        SecretPurpose.CapSecretKey => SecretClassNames.ServiceSync,
        SecretPurpose.ScriptEnvironment => SecretClassNames.ServiceSync,
        _ => SecretClassNames.SessionUnlocked,
    };

    private static string? MapPurposeToKeyTag(SecretPurpose purpose) => purpose switch
    {
        SecretPurpose.SshCredential => SecretPurposeKeyNames.Ssh,
        SecretPurpose.DnsProviderToken => SecretPurposeKeyNames.Dns,
        SecretPurpose.AcmeEab => SecretPurposeKeyNames.Acme,
        SecretPurpose.AdGuardCredential => SecretPurposeKeyNames.AdGuard,
        SecretPurpose.NotificationToken => SecretPurposeKeyNames.Notification,
        SecretPurpose.OidcClientSecret => SecretPurposeKeyNames.Oidc,
        SecretPurpose.MaxMindLicenseKey => SecretPurposeKeyNames.MaxMind,
        SecretPurpose.CapSecretKey => SecretPurposeKeyNames.Cap,
        SecretPurpose.ScriptEnvironment => SecretPurposeKeyNames.Script,
        _ => null,
    };
}
