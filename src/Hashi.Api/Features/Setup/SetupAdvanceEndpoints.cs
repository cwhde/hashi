using Hashi.Contracts.Api;
using Hashi.Core.Setup;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Api.Features.Setup;

public static class SetupAdvanceEndpoints
{
    public static IEndpointRouteBuilder MapSetupAdvanceEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/setup").WithTags("Setup");

        group.MapPost("/steps/{stepSlug}/complete", async Task<IResult> (
            string stepSlug,
            SetupStateService setup,
            VaultSessionState vaultSession,
            HashiDbContext db,
            AuditService audit,
            CancellationToken ct) =>
        {
            var parsed = SetupStepNames.FromSlug(stepSlug);
            if (SetupStepNames.ToSlug(parsed) != stepSlug)
            {
                return TypedResults.BadRequest(new { error = $"Unknown setup step: {stepSlug}" });
            }
            if (parsed is SetupStep.Complete)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(
                    "Final setup completion must use POST /api/setup/complete."));
            }
            if (parsed is SetupStep.PasskeyAndVault)
            {
                if (await db.PasskeyCredentials.CountAsync(ct) == 0)
                {
                    return TypedResults.BadRequest(new ApiErrorResponse("Register a passkey before advancing."));
                }
                var vaultConfigured = await db.VaultWrappedKeys.AnyAsync(
                    x => x.WrapMethod == Hashi.Infrastructure.Persistence.Entities.VaultWrapMethodNames.RecoveryKey,
                    ct);
                if (!vaultConfigured || !vaultSession.IsUnlocked)
                {
                    return TypedResults.BadRequest(new ApiErrorResponse("Configure and unlock the vault before advancing."));
                }
            }

            await setup.MarkStepCompleteAsync(parsed, ct);
            await audit.WriteAsync("setup", "step_completed", subjectType: "setup_step", subjectId: stepSlug, cancellationToken: ct);
            var state = await setup.GetOrCreateAsync(ct);
            return TypedResults.Ok(new SetupStatusResponse(
                state.IsComplete,
                state.CurrentStep,
                await setup.GetCompletedStepsAsync(ct),
                state.HttpsDomainVerifiedAtUtc is not null,
                state.UpdatedAtUtc));
        });

        group.MapPost("/system-resource/plan", async Task<IResult> (
            SystemResourceSetupService systemResource,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await systemResource.PlanAsync(ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });

        group.MapPost("/system-resource/sync", async Task<IResult> (
            SystemResourceSetupService systemResource,
            AuditService audit,
            CancellationToken ct) =>
        {
            try
            {
                var result = await systemResource.SyncAsync(ct);
                await audit.WriteAsync("setup", "system_resource_sync", subjectType: "sync_run", subjectId: result.RunId.ToString(), cancellationToken: ct);
                return result.Succeeded ? TypedResults.Ok(result) : TypedResults.BadRequest(result);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });

        return app;
    }
}
