using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Hashi.Infrastructure.Sync;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.HttpResults;
using System.Text.Json;

namespace Hashi.Api.Features.Setup;

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
            using var scope = scopeFactory.CreateScope();
            var orchestrator = scope.ServiceProvider.GetRequiredService<SyncOrchestratorService>();
            await orchestrator.TriggerImmediateSyncAsync(ct);
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

        group.MapGet("/admin-session", async (AppSettingsService settings, CancellationToken ct) =>
        {
            var s = await settings.GetOrCreateAsync(ct);
            return TypedResults.Ok(new AdminSessionSettingsResponse(
                s.AdminSessionMinutes,
                s.AdminSessionAbsoluteMinutes,
                s.UpdatedAtUtc));
        });

        group.MapPut("/admin-session", async Task<Results<Ok<AdminSessionSettingsResponse>, BadRequest<ApiErrorResponse>>> (
            AdminSessionSettingsRequest request,
            AppSettingsService settings,
            AuditService audit,
            CancellationToken ct) =>
        {
            var s = await settings.GetOrCreateAsync(ct);
            var idleMinutes = request.IdleTimeoutMinutes is { } requestedIdle
                ? Math.Clamp(requestedIdle, 5, 240)
                : s.AdminSessionMinutes;
            var absoluteMinutes = request.AbsoluteTimeoutMinutes is { } requestedAbsolute
                ? Math.Clamp(requestedAbsolute, 5, 480)
                : s.AdminSessionAbsoluteMinutes;
            if (absoluteMinutes < idleMinutes)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(
                    "Absolute session timeout must be greater than or equal to idle timeout."));
            }

            s.AdminSessionMinutes = idleMinutes;
            s.AdminSessionAbsoluteMinutes = absoluteMinutes;
            s.UpdatedAtUtc = DateTimeOffset.UtcNow;
            await settings.SaveAsync(ct);
            await audit.WriteAsync(
                "settings",
                "admin_session_updated",
                subjectType: "app_settings",
                metadata: new { idleMinutes, absoluteMinutes },
                cancellationToken: ct);
            return TypedResults.Ok(new AdminSessionSettingsResponse(
                s.AdminSessionMinutes,
                s.AdminSessionAbsoluteMinutes,
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

        SettingsGeoIpEndpoints.Map(group);

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
