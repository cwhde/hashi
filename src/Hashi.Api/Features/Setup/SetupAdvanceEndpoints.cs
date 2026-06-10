using Hashi.Contracts.Api;
using Hashi.Core.Setup;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Hashi.Infrastructure.Sync;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.HttpResults;
using Microsoft.EntityFrameworkCore;
using System.Text.Json;

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
    private static readonly HashSet<string> AllowedCategories = new(StringComparer.OrdinalIgnoreCase)
    {
        "security",
        "appearance",
        "dashboard",
        "dns",
        "traefik",
        "firewall",
        "notifications",
        "pulse",
    };

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
            IServiceScopeFactory scopeFactory,
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
            try
            {
                using var scope = scopeFactory.CreateScope();
                var syncHost = scope.ServiceProvider.GetRequiredService<SyncOrchestratorHostedService>();
                syncHost.SignalImmediateSync();
            }
            catch
            {
                // Best-effort sync trigger.
            }
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
            return TypedResults.Ok(new EdgeSsoSettingsResponse(
                s.EdgeSsoSessionHours,
                s.EdgeSsoIdleTimeoutMinutes,
                s.EdgeSsoRememberDeviceDays,
                s.UpdatedAtUtc));
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

            if (request.EdgeSsoIdleTimeoutMinutes is int idleMinutes && idleMinutes >= 5)
            {
                s.EdgeSsoIdleTimeoutMinutes = Math.Min(idleMinutes, 10080);
            }

            if (request.EdgeSsoRememberDeviceDays is int rememberDays && rememberDays >= 0)
            {
                s.EdgeSsoRememberDeviceDays = Math.Min(rememberDays, 365);
            }

            s.UpdatedAtUtc = DateTimeOffset.UtcNow;
            await settings.SaveAsync(ct);
            await audit.WriteAsync("settings", "edge_sso_updated", subjectType: "app_settings", cancellationToken: ct);
            return TypedResults.Ok(new EdgeSsoSettingsResponse(
                s.EdgeSsoSessionHours,
                s.EdgeSsoIdleTimeoutMinutes,
                s.EdgeSsoRememberDeviceDays,
                s.UpdatedAtUtc));
        });

        group.MapGet("/dashboard", async (AppSettingsService settings, CancellationToken ct) =>
        {
            var s = await settings.GetOrCreateAsync(ct);
            return TypedResults.Ok(new DashboardSettingsResponse(s.OverviewWidgetsJson, s.UpdatedAtUtc));
        });

        group.MapPut("/dashboard", async Task<Results<Ok<DashboardSettingsResponse>, BadRequest<ApiErrorResponse>>> (
            DashboardSettingsRequest request,
            AppSettingsService settings,
            AuditService audit,
            CancellationToken ct) =>
        {
            var s = await settings.GetOrCreateAsync(ct);
            if (request.OverviewWidgetsJson is not null)
            {
                if (!TryEnsureJsonObject(request.OverviewWidgetsJson, "Overview widget preferences", out var error))
                {
                    return TypedResults.BadRequest(new ApiErrorResponse(error));
                }

                s.OverviewWidgetsJson = request.OverviewWidgetsJson;
            }

            s.UpdatedAtUtc = DateTimeOffset.UtcNow;
            await settings.SaveAsync(ct);
            await audit.WriteAsync("settings", "dashboard_updated", subjectType: "app_settings", cancellationToken: ct);
            return TypedResults.Ok(new DashboardSettingsResponse(s.OverviewWidgetsJson, s.UpdatedAtUtc));
        });

        group.MapGet("/geoip", async (GeoIpSettingsService geoIp, CancellationToken ct) =>
            TypedResults.Ok(await geoIp.GetAsync(ct)))
            .Produces<GeoIpSettingsResponse>(StatusCodes.Status200OK);

        group.MapPut("/geoip", async Task<IResult> (
            GeoIpSettingsRequest request,
            GeoIpSettingsService geoIp,
            AuditService audit,
            CancellationToken ct) =>
        {
            try
            {
                var response = await geoIp.UpdateAsync(request, ct);
                await audit.WriteAsync("settings", "geoip_updated", subjectType: "app_settings", cancellationToken: ct);
                return TypedResults.Ok(response);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<GeoIpSettingsResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);

        group.MapPost("/geoip/update", async Task<IResult> (
            GeoIpUpdateService updater,
            BackgroundJobService jobs,
            AuditService audit,
            CancellationToken ct) =>
        {
            await jobs.BeginRunAsync(BackgroundJobKeys.GeoIpUpdate, ct);
            var result = await updater.RunUpdateAsync(ct);
            await jobs.CompleteRunAsync(
                BackgroundJobKeys.GeoIpUpdate,
                result.Succeeded,
                result.Message,
                result.Succeeded ? null : result.Message,
                259200,
                ct);
            await audit.WriteAsync(
                "settings",
                "geoip_update_requested",
                outcome: result.Succeeded ? "success" : "failure",
                subjectType: "app_settings",
                cancellationToken: ct);
            return result.Succeeded ? TypedResults.Ok(result) : TypedResults.BadRequest(result);
        })
            .Produces<GeoIpUpdateResponse>(StatusCodes.Status200OK)
            .Produces<GeoIpUpdateResponse>(StatusCodes.Status400BadRequest);

        group.MapGet("/categories/{category}", async Task<Results<Ok<CategorySettingsResponse>, NotFound>> (
            string category,
            AppSettingsService settings,
            CancellationToken ct) =>
        {
            if (!AllowedCategories.Contains(category))
            {
                return TypedResults.NotFound();
            }

            var s = await settings.GetOrCreateAsync(ct);
            var map = ReadCategoryMap(s.SettingsCategoriesJson);
            map.TryGetValue(category.ToLowerInvariant(), out var json);
            return TypedResults.Ok(new CategorySettingsResponse(category.ToLowerInvariant(), json ?? "{}", s.UpdatedAtUtc));
        });

        group.MapPut("/categories/{category}", async Task<Results<Ok<CategorySettingsResponse>, NotFound, BadRequest<ApiErrorResponse>>> (
            string category,
            CategorySettingsRequest request,
            AppSettingsService settings,
            AuditService audit,
            CancellationToken ct) =>
        {
            if (!AllowedCategories.Contains(category))
            {
                return TypedResults.NotFound();
            }

            var json = string.IsNullOrWhiteSpace(request.SettingsJson) ? "{}" : request.SettingsJson!;
            if (!TryEnsureJsonObject(json, "Category settings", out var error))
            {
                return TypedResults.BadRequest(new ApiErrorResponse(error));
            }

            var s = await settings.GetOrCreateAsync(ct);
            var map = ReadCategoryMap(s.SettingsCategoriesJson);
            var normalized = category.ToLowerInvariant();
            map[normalized] = json;
            s.SettingsCategoriesJson = JsonSerializer.Serialize(map);
            s.UpdatedAtUtc = DateTimeOffset.UtcNow;
            await settings.SaveAsync(ct);
            await audit.WriteAsync("settings", $"{normalized}_updated", subjectType: "app_settings", cancellationToken: ct);
            return TypedResults.Ok(new CategorySettingsResponse(normalized, json, s.UpdatedAtUtc));
        });

        return app;
    }

    private static Dictionary<string, string> ReadCategoryMap(string json)
    {
        try
        {
            return JsonSerializer.Deserialize<Dictionary<string, string>>(json) ?? [];
        }
        catch
        {
            return [];
        }
    }

    private static bool TryEnsureJsonObject(string json, string label, out string error)
    {
        error = string.Empty;
        try
        {
            using var doc = JsonDocument.Parse(json);
            if (doc.RootElement.ValueKind != JsonValueKind.Object)
            {
                error = $"{label} must be a JSON object.";
                return false;
            }

            return true;
        }
        catch (JsonException)
        {
            error = $"{label} must be valid JSON.";
            return false;
        }
    }
}
