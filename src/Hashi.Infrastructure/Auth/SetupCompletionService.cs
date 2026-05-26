using Hashi.Core.Setup;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Auth;

public sealed class SetupCompletionService(
    HashiDbContext db,
    SetupStateService setupState,
    VaultSessionState vaultSession,
    AuditService audit)
{
    public async Task<SetupCompletionResult> TryCompleteAsync(CancellationToken cancellationToken = default)
    {
        var state = await setupState.GetOrCreateAsync(cancellationToken);
        if (state.IsComplete)
        {
            return SetupCompletionResult.Failed("Setup is already complete.");
        }

        var passkeyCount = await db.PasskeyCredentials.CountAsync(cancellationToken);
        if (passkeyCount == 0)
        {
            return SetupCompletionResult.Failed("Register a passkey before completing setup.");
        }

        var vaultConfigured = await db.VaultWrappedKeys.AnyAsync(
            x => x.WrapMethod == Persistence.Entities.VaultWrapMethodNames.RecoveryKey,
            cancellationToken);
        if (!vaultConfigured)
        {
            return SetupCompletionResult.Failed("Configure the vault before completing setup.");
        }

        if (!vaultSession.IsUnlocked)
        {
            return SetupCompletionResult.Failed("Unlock the vault to verify recovery before completing setup.");
        }

        if (state.HttpsDomainVerifiedAtUtc is null)
        {
            return SetupCompletionResult.Failed(
                "Verify HTTPS access on the admin domain (POST /api/setup/verify-https) before completing setup.");
        }

        await setupState.MarkCompleteAsync(cancellationToken);
        await setupState.MarkStepCompleteAsync(SetupStep.PasskeyAndVault, cancellationToken);
        await setupState.MarkStepCompleteAsync(SetupStep.Complete, cancellationToken);
        await audit.WriteAsync("setup", "setup_completed", subjectType: "setup", cancellationToken: cancellationToken);
        vaultSession.Lock();

        return SetupCompletionResult.Ok();
    }
}

public sealed record SetupCompletionResult(bool Succeeded, string? Error)
{
    public static SetupCompletionResult Ok() => new(true, null);

    public static SetupCompletionResult Failed(string error) => new(false, error);
}
