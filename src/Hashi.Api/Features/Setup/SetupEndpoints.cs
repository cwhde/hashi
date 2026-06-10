using Hashi.Contracts.Api;
using Hashi.Core.Setup;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Bootstrap;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Http.HttpResults;

namespace Hashi.Api.Features.Setup;

public static class SetupEndpoints
{
    public static IEndpointRouteBuilder MapSetupEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/setup").WithTags("Setup");

        group.MapGet("/status", async (SetupStateService setup, CancellationToken ct) =>
        {
            var state = await setup.GetOrCreateAsync(ct);
            var completed = await setup.GetCompletedStepsAsync(ct);
            return TypedResults.Ok(new SetupStatusResponse(
                state.IsComplete,
                state.CurrentStep,
                completed,
                state.HttpsDomainVerifiedAtUtc is not null,
                state.UpdatedAtUtc));
        });

        group.MapPost("/verify-https", async Task<IResult> (
            HttpContext httpContext,
            AppSettingsService settings,
            SetupStateService setup,
            AuditService audit,
            CancellationToken ct) =>
        {
            var appSettings = await settings.GetOrCreateAsync(ct);
            if (string.IsNullOrWhiteSpace(appSettings.AdminDomain))
            {
                return TypedResults.BadRequest(new SetupVerifyHttpsResponse(false, "Configure admin domain in base settings first."));
            }

            var host = httpContext.Request.Host.Host;
            if (!HostMatchesAdminDomain(host, appSettings.AdminDomain))
            {
                return TypedResults.BadRequest(new SetupVerifyHttpsResponse(
                    false,
                    $"Request host '{host}' does not match admin domain '{appSettings.AdminDomain}'."));
            }

            if (!IsHttpsRequest(httpContext))
            {
                return TypedResults.BadRequest(new SetupVerifyHttpsResponse(
                    false,
                    "Open Hashi over HTTPS on the admin domain before verifying."));
            }

            await setup.MarkHttpsVerifiedAsync(ct);
            await audit.WriteAsync("setup", "https_domain_verified", subjectType: "setup", cancellationToken: ct);
            return TypedResults.Ok(new SetupVerifyHttpsResponse(true, null));
        });

        group.MapGet("/bootstrap-allowed", (HttpContext httpContext) =>
        {
            var remoteIp = httpContext.Connection.RemoteIpAddress?.ToString();
            return TypedResults.Ok(new BootstrapAllowedResponse(
                BootstrapNetworkPolicy.IsAllowed(remoteIp),
                remoteIp));
        });

        group.MapGet("/certificate", async (CertificateSetupService certificate, CancellationToken ct) =>
            TypedResults.Ok(await certificate.GetAsync(ct)))
            .Produces<CertificateSetupResponse>(StatusCodes.Status200OK);
        group.MapPost("/certificate/validate", async Task<IResult> (
            CertificateSetupRequest request,
            CertificateSetupService certificate,
            CancellationToken ct) =>
            TypedResults.Ok(await certificate.ValidateAsync(request, ct)))
            .Produces<CertificateSetupValidateResponse>(StatusCodes.Status200OK);
        group.MapPost("/certificate/save", async Task<IResult> (
            CertificateSetupRequest request,
            CertificateSetupService certificate,
            CancellationToken ct) =>
        {
            var result = await certificate.SaveAsync(request, ct);
            return result.Saved
                ? TypedResults.Ok(result)
                : TypedResults.BadRequest(result);
        })
            .Produces<CertificateSetupSaveResponse>(StatusCodes.Status200OK);

        return app;
    }

    private static bool IsHttpsRequest(HttpContext httpContext)
    {
        if (httpContext.Request.IsHttps)
        {
            return true;
        }

        var forwarded = httpContext.Request.Headers["X-Forwarded-Proto"].FirstOrDefault();
        return string.Equals(forwarded, "https", StringComparison.OrdinalIgnoreCase);
    }

    private static bool HostMatchesAdminDomain(string host, string adminDomain)
    {
        if (string.Equals(host, adminDomain, StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        return host.EndsWith("." + adminDomain, StringComparison.OrdinalIgnoreCase);
    }
}

public static class ActivityEndpoints
{
    public static IEndpointRouteBuilder MapActivityEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/activity").WithTags("Activity");

        group.MapGet("/audit", async (AuditService audit, CancellationToken ct) =>
        {
            var events = await audit.ListRecentAsync(cancellationToken: ct);
            return TypedResults.Ok(events.Select(x => new AuditEventResponse(
                x.Id,
                x.Category,
                x.Action,
                x.SubjectType,
                x.SubjectId,
                x.Outcome,
                x.CreatedAtUtc)));
        });

        group.MapGet("/jobs", async (BackgroundJobService jobs, CancellationToken ct) =>
        {
            var items = await jobs.ListAsync(ct);
            return TypedResults.Ok(items.Select(x => new BackgroundJobResponse(
                x.JobKey,
                x.DisplayName,
                x.Status,
                x.LastStartedAtUtc,
                x.LastCompletedAtUtc,
                x.NextRunAtUtc,
                x.LastDurationMs,
                x.LastDiffSummary,
                x.LastError,
                x.IntervalSeconds)));
        })
            .Produces<IEnumerable<BackgroundJobResponse>>(StatusCodes.Status200OK);

        return app;
    }
}

public static class HealthEndpoints
{
    public static IEndpointRouteBuilder MapHealthEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/api/health", (VaultSessionState session, ServiceSyncVaultState serviceSync) =>
        {
            var available = session.IsUnlocked || (serviceSync.IsReady && serviceSync.IsUnlocked);
            return TypedResults.Ok(new HealthResponse(
                "healthy",
                "2.0.0-alpha",
                DateTimeOffset.UtcNow,
                serviceSync.IsReady,
                ProviderSyncPaused: !available));
        })
            .WithTags("Health")
            .AllowAnonymous();

        return app;
    }
}
