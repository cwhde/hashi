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

        var dek = RandomNumberGenerator.GetBytes(32);
        try
        {
            var ciphertext = AesGcmCipher.Encrypt(plaintext, dek).ToBlob();
            var adminWrapped = AesGcmCipher.Encrypt(dek, session.GetRootKeyOrThrow()).ToBlob();
            byte[]? serviceWrapped = null;
            if (serviceSyncEligible && serviceSync.IsReady)
            {
                serviceWrapped = AesGcmCipher.Encrypt(dek, serviceSync.GetWrapKeyOrThrow()).ToBlob();
            }

            var entity = new SecretRecordEntity
            {
                Purpose = SecretPurposeMapping.ToName(purpose),
                Label = label,
                AdminWrappedDekBlob = adminWrapped,
                ServiceWrappedDekBlob = serviceWrapped,
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
        if (session.IsUnlocked)
        {
            return await DecryptWithAdminWrapAsync(secretId, cancellationToken);
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
}
