using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Crypto;
using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Http.HttpResults;

namespace Hashi.Api.Features.Vault;

public static class VaultEndpoints
{
    public static IEndpointRouteBuilder MapVaultEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/vault").WithTags("Vault");

        group.MapGet("/status", async (VaultService vault, CancellationToken ct) =>
        {
            var status = await vault.GetStatusAsync(ct);
            return TypedResults.Ok(new VaultStatusResponse(
                status.LockState.ToString(),
                status.IsVaultConfigured,
                status.HasPasskey,
                status.PrfWrapAvailable,
                status.ServiceSyncVaultReady,
                status.BootstrapCredentialsActive));
        });

        group.MapPost("/recovery-key/generate", () =>
        {
            var key = RecoveryKeyGenerator.Generate();
            return TypedResults.Ok(new VaultGenerateRecoveryKeyResponse(key));
        });

        group.MapPost("/setup", async Task<IResult> (
            VaultSetupRequest request,
            VaultService vault,
            CancellationToken ct) =>
        {
            byte[]? prfOutput = null;
            if (!string.IsNullOrWhiteSpace(request.PrfOutputBase64))
            {
                prfOutput = Convert.FromBase64String(request.PrfOutputBase64);
            }

            var result = await vault.SetupVaultAsync(
                request.RecoveryKey,
                request.PrfWrapAttempted,
                prfOutput,
                request.PasskeyCredentialId,
                ct);

            return TypedResults.Ok(new VaultSetupResponse(
                result.Configured,
                result.PrfWrapStored,
                result.ServiceSyncWrapStored,
                request.RecoveryKey));
        });

        group.MapPost("/unlock", async Task<IResult> (
            VaultUnlockRequest request,
            VaultService vault,
            CancellationToken ct) =>
        {
            var unlocked = await vault.UnlockWithRecoveryKeyAsync(request.RecoveryKey, ct);
            return unlocked
                ? TypedResults.Ok(new VaultUnlockResponse(true))
                : TypedResults.Unauthorized();
        });

        group.MapPost("/lock", async (VaultService vault, CancellationToken ct) =>
        {
            await vault.LockAsync(ct);
            return TypedResults.Ok(new VaultLockResponse(true));
        });

        group.MapGet("/secrets", async (SecretRecordService secrets, CancellationToken ct) =>
        {
            var items = await secrets.ListAsync(ct);
            return TypedResults.Ok(items.Select(x => new SecretDescriptorResponse(
                x.Id,
                x.Purpose.ToString(),
                x.Label,
                x.CreatedAtUtc,
                x.UpdatedAtUtc)));
        });

        group.MapPost("/secrets", async Task<IResult> (
            SecretStoreRequest request,
            SecretRecordService secrets,
            CancellationToken ct) =>
        {
            if (!Enum.TryParse<SecretPurpose>(request.Purpose, ignoreCase: true, out var purpose))
            {
                return TypedResults.BadRequest(new ApiErrorResponse($"Unknown secret purpose: {request.Purpose}"));
            }

            var plaintext = Convert.FromBase64String(request.PlaintextBase64);
            var stored = await secrets.StoreAsync(purpose, request.Label, plaintext, ct);
            return TypedResults.Ok(new SecretDescriptorResponse(
                stored.Id,
                stored.Purpose.ToString(),
                stored.Label,
                stored.CreatedAtUtc,
                stored.UpdatedAtUtc));
        });

        group.MapGet("/secrets/{id:guid}/reveal", async Task<IResult> (
            Guid id,
            SecretRecordService secrets,
            AuditService audit,
            CancellationToken ct) =>
        {
            var plaintext = await secrets.DecryptForAdminAsync(id, ct);
            if (plaintext is null)
            {
                return TypedResults.NotFound();
            }

            await audit.WriteAsync("vault", "secret_revealed", subjectType: "secret", subjectId: id.ToString(), cancellationToken: ct);
            return TypedResults.Ok(new SecretRevealResponse(Convert.ToBase64String(plaintext)));
        });

        group.MapPost("/verify-unlock", async Task<IResult> (
            VaultUnlockRequest request,
            VaultService vault,
            VaultSessionState session,
            CancellationToken ct) =>
        {
            await vault.LockAsync(ct);
            var unlocked = await vault.UnlockWithRecoveryKeyAsync(request.RecoveryKey, ct);
            return TypedResults.Ok(new VaultVerifyUnlockResponse(unlocked, session.IsUnlocked));
        });

        return app;
    }
}

public static class SetupCompletionEndpoints
{
    public static IEndpointRouteBuilder MapSetupCompletionEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/setup").WithTags("Setup");

        group.MapPost("/complete", async Task<IResult> (
            SetupCompletionService completion,
            CancellationToken ct) =>
        {
            var result = await completion.TryCompleteAsync(ct);
            return result.Succeeded
                ? TypedResults.Ok(new SetupCompleteResponse(true, null))
                : TypedResults.BadRequest(new SetupCompleteResponse(false, result.Error));
        });

        return app;
    }
}
