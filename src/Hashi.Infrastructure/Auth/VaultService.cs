using System.Security.Cryptography;
using Hashi.Core.Auth;
using Hashi.Core.Setup;
using Hashi.Infrastructure.Crypto;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Auth;

public sealed class VaultService(
    HashiDbContext db,
    VaultSessionState session,
    ServiceSyncVaultState serviceSync,
    SetupStateService setupState,
    AuditService audit,
    ILogger<VaultService> logger)
{
    public async Task<VaultStatus> GetStatusAsync(CancellationToken cancellationToken = default)
    {
        var state = await setupState.GetOrCreateAsync(cancellationToken);
        var hasPasskey = await db.PasskeyCredentials.AnyAsync(cancellationToken);
        var configured = await db.VaultWrappedKeys.AnyAsync(
            x => x.WrapMethod == VaultWrapMethodNames.RecoveryKey,
            cancellationToken);
        var prfWrap = await db.VaultWrappedKeys.AnyAsync(
            x => x.WrapMethod == VaultWrapMethodNames.PasskeyPrf,
            cancellationToken);

        var lockState = !configured
            ? VaultLockState.NotConfigured
            : session.IsUnlocked
                ? VaultLockState.Unlocked
                : VaultLockState.Locked;

        return new VaultStatus(
            lockState,
            configured,
            hasPasskey,
            prfWrap,
            serviceSync.IsReady,
            !state.IsComplete && !string.IsNullOrEmpty(state.BootstrapPasswordHash));
    }

    public async Task<VaultSetupResult> SetupVaultAsync(
        string recoveryKey,
        bool prfWrapAttempted,
        byte[]? prfOutput,
        Guid? passkeyCredentialId,
        CancellationToken cancellationToken = default)
    {
        if (await db.VaultWrappedKeys.AnyAsync(cancellationToken))
        {
            throw new InvalidOperationException("Vault is already configured.");
        }

        if (await db.PasskeyCredentials.CountAsync(cancellationToken) == 0)
        {
            throw new InvalidOperationException("Register a passkey before configuring the vault.");
        }

        var adminRootKey = RandomNumberGenerator.GetBytes(32);
        try
        {
            var recoveryWrapKey = KeyDerivation.DeriveRecoveryWrapKey(recoveryKey);
            var recoveryBlob = AesGcmCipher.Encrypt(adminRootKey, recoveryWrapKey).ToBlob();
            db.VaultWrappedKeys.Add(new VaultWrappedKeyEntity
            {
                WrapMethod = VaultWrapMethodNames.RecoveryKey,
                WrappedKeyBlob = recoveryBlob,
                RecoveryKeyHash = KeyDerivation.HashRecoveryKeyForVerification(recoveryKey),
            });

            if (prfWrapAttempted && prfOutput is { Length: >= 32 } && passkeyCredentialId is Guid credentialId)
            {
                var credential = await db.PasskeyCredentials.SingleOrDefaultAsync(x => x.Id == credentialId, cancellationToken)
                    ?? throw new InvalidOperationException("Passkey credential not found.");
                var prfWrapKey = KeyDerivation.DerivePrfWrapKey(prfOutput, credential.CredentialId);
                var prfBlob = AesGcmCipher.Encrypt(adminRootKey, prfWrapKey).ToBlob();
                db.VaultWrappedKeys.Add(new VaultWrappedKeyEntity
                {
                    WrapMethod = VaultWrapMethodNames.PasskeyPrf,
                    WrappedKeyBlob = prfBlob,
                    PasskeyCredentialId = credentialId,
                });
                credential.PrfSupported = true;
            }

            if (serviceSync.IsReady)
            {
                var serviceBlob = AesGcmCipher.Encrypt(adminRootKey, serviceSync.GetWrapKeyOrThrow()).ToBlob();
                db.VaultWrappedKeys.Add(new VaultWrappedKeyEntity
                {
                    WrapMethod = VaultWrapMethodNames.ServiceSync,
                    WrappedKeyBlob = serviceBlob,
                });
            }

            await db.SaveChangesAsync(cancellationToken);
            session.Unlock(adminRootKey);
            await audit.WriteAsync("vault", "vault_configured", metadata: new { prfWrapAttempted, serviceSync = serviceSync.IsReady }, cancellationToken: cancellationToken);
            logger.LogInformation("Vault configured with recovery key wrap{PrfSuffix}", prfWrapAttempted ? " and PRF wrap" : string.Empty);

            return new VaultSetupResult(true, prfWrapAttempted && prfOutput is { Length: >= 32 }, serviceSync.IsReady);
        }
        finally
        {
            CryptographicOperations.ZeroMemory(adminRootKey);
        }
    }

    public async Task<bool> UnlockWithRecoveryKeyAsync(string recoveryKey, CancellationToken cancellationToken = default)
    {
        var wrapped = await db.VaultWrappedKeys.SingleOrDefaultAsync(
            x => x.WrapMethod == VaultWrapMethodNames.RecoveryKey,
            cancellationToken)
            ?? throw new InvalidOperationException("Vault is not configured.");

        var hash = KeyDerivation.HashRecoveryKeyForVerification(recoveryKey);
        if (!string.Equals(hash, wrapped.RecoveryKeyHash, StringComparison.Ordinal))
        {
            await audit.WriteAsync("vault", "unlock_failed", outcome: "failure", cancellationToken: cancellationToken);
            return false;
        }

        var rootKey = AesGcmCipher.Decrypt(wrapped.WrappedKeyBlob, KeyDerivation.DeriveRecoveryWrapKey(recoveryKey));
        session.Unlock(rootKey);
        CryptographicOperations.ZeroMemory(rootKey);
        await audit.WriteAsync("vault", "unlocked", metadata: new { method = "recovery_key" }, cancellationToken: cancellationToken);
        return true;
    }

    public async Task<bool> UnlockWithPrfAsync(
        Guid passkeyCredentialId,
        byte[] prfOutput,
        string? sessionKey = null,
        CancellationToken cancellationToken = default)
    {
        var wrapped = await db.VaultWrappedKeys.SingleOrDefaultAsync(
            x => x.WrapMethod == VaultWrapMethodNames.PasskeyPrf && x.PasskeyCredentialId == passkeyCredentialId,
            cancellationToken);
        if (wrapped is null)
        {
            return false;
        }

        var credential = await db.PasskeyCredentials.SingleAsync(x => x.Id == passkeyCredentialId, cancellationToken);
        var rootKey = AesGcmCipher.Decrypt(wrapped.WrappedKeyBlob, KeyDerivation.DerivePrfWrapKey(prfOutput, credential.CredentialId));
        if (sessionKey is null)
        {
            session.Unlock(rootKey);
        }
        else
        {
            session.UnlockForSession(sessionKey, rootKey);
        }

        CryptographicOperations.ZeroMemory(rootKey);
        await audit.WriteAsync("vault", "unlocked", metadata: new { method = "passkey_prf" }, cancellationToken: cancellationToken);
        return true;
    }

    public Task LockAsync(CancellationToken cancellationToken = default)
    {
        session.Lock();
        return audit.WriteAsync("vault", "locked", cancellationToken: cancellationToken);
    }

    public async Task EnsureServiceSyncWrapAsync(CancellationToken cancellationToken = default)
    {
        if (!serviceSync.IsReady)
        {
            return;
        }

        var wrapped = await db.VaultWrappedKeys.SingleOrDefaultAsync(
            x => x.WrapMethod == VaultWrapMethodNames.ServiceSync,
            cancellationToken);
        if (wrapped is null)
        {
            return;
        }

        try
        {
            var rootKey = AesGcmCipher.Decrypt(wrapped.WrappedKeyBlob, serviceSync.GetWrapKeyOrThrow());
            CryptographicOperations.ZeroMemory(rootKey);
        }
        catch (CryptographicException ex)
        {
            logger.LogWarning(ex, "Service-sync vault unlock failed.");
        }
    }
}

public sealed record VaultSetupResult(bool Configured, bool PrfWrapStored, bool ServiceSyncWrapStored);
