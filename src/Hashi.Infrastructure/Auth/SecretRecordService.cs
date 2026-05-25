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
        CancellationToken cancellationToken = default)
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
            if (serviceSync.IsReady)
            {
                serviceWrapped = AesGcmCipher.Encrypt(dek, serviceSync.GetWrapKeyOrThrow()).ToBlob();
            }

            var entity = new SecretRecordEntity
            {
                Purpose = SecretPurposeMapping.ToName(purpose),
                Label = label,
                AdminWrappedDekBlob = adminWrapped,
                ServiceWrappedDekBlob = serviceWrapped,
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
}
