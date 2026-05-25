using Hashi.Contracts.Api;
using Hashi.Core.Setup;
using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.HttpResults;

namespace Hashi.Api.Features.Setup;

public static class SetupAdvanceEndpoints
{
    public static IEndpointRouteBuilder MapSetupAdvanceEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/setup").WithTags("Setup");

        group.MapPost("/steps/{stepSlug}/complete", async Task<IResult> (
            string stepSlug,
            SetupStateService setup,
            AuditService audit,
            CancellationToken ct) =>
        {
            var parsed = SetupStepNames.FromSlug(stepSlug);
            if (SetupStepNames.ToSlug(parsed) != stepSlug)
            {
                return TypedResults.BadRequest(new { error = $"Unknown setup step: {stepSlug}" });
            }

            await setup.MarkStepCompleteAsync(parsed, ct);
            await audit.WriteAsync("setup", "step_completed", subjectType: "setup_step", subjectId: stepSlug, cancellationToken: ct);

            var state = await setup.GetOrCreateAsync(ct);
            var completed = await setup.GetCompletedStepsAsync(ct);
            return TypedResults.Ok(new SetupStatusResponse(
                state.IsComplete,
                state.CurrentStep,
                completed,
                state.UpdatedAtUtc));
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

        return app;
    }
}
