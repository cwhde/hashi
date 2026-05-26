using Hashi.Contracts.Api;
using Hashi.Core.Setup;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.HttpResults;
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
                var passkeyCount = await db.PasskeyCredentials.CountAsync(ct);
                if (passkeyCount == 0)
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
            var completed = await setup.GetCompletedStepsAsync(ct);
            return TypedResults.Ok(new SetupStatusResponse(
                state.IsComplete,
                state.CurrentStep,
                completed,
                state.HttpsDomainVerifiedAtUtc is not null,
                state.UpdatedAtUtc));
        });

        group.MapPost("/system-resource/plan", async Task<IResult> (
            SystemResourceSetupService systemResource,
            CancellationToken ct) =>
        {
            try
            {
                var result = await systemResource.PlanAsync(ct);
                return TypedResults.Ok(result);
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
                return result.Succeeded
                    ? TypedResults.Ok(result)
                    : TypedResults.BadRequest(result);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });

        return app;
    }
}

public static class SettingsEndpoints
{
    public static IEndpointRouteBuilder MapSettingsEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/settings").WithTags("Settings");

        group.MapGet("/general", async (AppSettingsService settings, CancellationToken ct) =>
        {
            var s = await settings.GetOrCreateAsync(ct);
            return TypedResults.Ok(new GeneralSettingsResponse(
                s.RootDomain,
                s.AdminDomain,
                s.InternalUrl,
                s.DefaultSyncIntervalMinutes,
                s.PublicDashboardEnabled,
                s.PublicStatusEnabled,
                s.Theme,
                s.UpdatedAtUtc));
        });

        group.MapPut("/general", async (
            GeneralSettingsRequest request,
            AppSettingsService settings,
            AuditService audit,
            CancellationToken ct) =>
        {
            var s = await settings.GetOrCreateAsync(ct);
            if (request.RootDomain is not null)
            {
                s.RootDomain = request.RootDomain;
            }

            if (request.AdminDomain is not null)
            {
                s.AdminDomain = request.AdminDomain;
            }

            if (request.InternalUrl is not null)
            {
                s.InternalUrl = request.InternalUrl;
            }

            if (request.DefaultSyncIntervalMinutes is int interval && interval > 0)
            {
                s.DefaultSyncIntervalMinutes = interval;
            }

            if (request.PublicDashboardEnabled is bool dashboard)
            {
                s.PublicDashboardEnabled = dashboard;
            }

            if (request.PublicStatusEnabled is bool status)
            {
                s.PublicStatusEnabled = status;
            }

            if (request.Theme is not null)
            {
                s.Theme = request.Theme;
            }

            s.UpdatedAtUtc = DateTimeOffset.UtcNow;
            await settings.SaveAsync(ct);
            await audit.WriteAsync("settings", "general_updated", subjectType: "app_settings", cancellationToken: ct);
            return TypedResults.Ok(new GeneralSettingsUpdateResponse(true, s.UpdatedAtUtc));
        });

        group.MapGet("/monitoring", async (AppSettingsService settings, CancellationToken ct) =>
        {
            var s = await settings.GetOrCreateAsync(ct);
            return TypedResults.Ok(new MonitoringSettingsResponse(
                s.MonitorCheckIntervalSeconds,
                s.MonitorCheckTimeoutSeconds,
                s.MonitorSampleRetentionDays,
                s.MonitorDegradedLatencyMs,
                s.UpdatedAtUtc));
        });

        group.MapPut("/monitoring", async (
            MonitoringSettingsRequest request,
            AppSettingsService settings,
            AuditService audit,
            CancellationToken ct) =>
        {
            var s = await settings.GetOrCreateAsync(ct);
            if (request.MonitorCheckIntervalSeconds is int interval && interval >= 15)
            {
                s.MonitorCheckIntervalSeconds = Math.Min(interval, 300);
            }

            if (request.MonitorCheckTimeoutSeconds is int timeout && timeout >= 5)
            {
                s.MonitorCheckTimeoutSeconds = Math.Min(timeout, 120);
            }

            if (request.MonitorSampleRetentionDays is int retention && retention >= 7)
            {
                s.MonitorSampleRetentionDays = Math.Min(retention, 365);
            }

            if (request.MonitorDegradedLatencyMs is int degraded && degraded >= 100)
            {
                s.MonitorDegradedLatencyMs = degraded;
            }

            s.UpdatedAtUtc = DateTimeOffset.UtcNow;
            await settings.SaveAsync(ct);
            await audit.WriteAsync("settings", "monitoring_updated", subjectType: "app_settings", cancellationToken: ct);
            return TypedResults.Ok(new MonitoringSettingsResponse(
                s.MonitorCheckIntervalSeconds,
                s.MonitorCheckTimeoutSeconds,
                s.MonitorSampleRetentionDays,
                s.MonitorDegradedLatencyMs,
                s.UpdatedAtUtc));
        });

        group.MapGet("/edge-sso/session", async (AppSettingsService settings, CancellationToken ct) =>
        {
            var s = await settings.GetOrCreateAsync(ct);
            return TypedResults.Ok(new EdgeSsoSettingsResponse(s.EdgeSsoSessionHours, s.UpdatedAtUtc));
        });

        group.MapPut("/edge-sso/session", async (
            EdgeSsoSettingsRequest request,
            AppSettingsService settings,
            AuditService audit,
            CancellationToken ct) =>
        {
            var s = await settings.GetOrCreateAsync(ct);
            if (request.EdgeSsoSessionHours is int hours && hours >= 1)
            {
                s.EdgeSsoSessionHours = Math.Min(hours, 168);
            }

            s.UpdatedAtUtc = DateTimeOffset.UtcNow;
            await settings.SaveAsync(ct);
            await audit.WriteAsync("settings", "edge_sso_updated", subjectType: "app_settings", cancellationToken: ct);
            return TypedResults.Ok(new EdgeSsoSettingsResponse(s.EdgeSsoSessionHours, s.UpdatedAtUtc));
        });

        return app;
    }
}
