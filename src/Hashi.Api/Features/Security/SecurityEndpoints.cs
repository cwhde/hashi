using Hashi.Api.Hosting;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.HttpResults;
using Microsoft.AspNetCore.Routing;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Api.Features.Security;

public static class SecurityEndpoints
{
    public static IEndpointRouteBuilder MapSecurityEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/security").WithTags("Security");
        group.MapGet("/dashboard", async (
            int? hours,
            string? resource,
            string? traefikHost,
            Guid? firewallHostId,
            SecurityIngestionService security,
            CancellationToken ct) =>
            TypedResults.Ok(await security.GetDashboardAsync(
                hours ?? 24,
                resource,
                traefikHost,
                firewallHostId,
                ct)))
            .Produces<SecurityDashboardResponse>(StatusCodes.Status200OK);
        group.MapGet("/captcha/settings", async (CaptchaChallengeService captcha, CancellationToken ct) =>
            TypedResults.Ok(await captcha.GetSettingsAsync(ct)))
            .Produces<CaptchaSettingsResponse>(StatusCodes.Status200OK);
        group.MapPut("/captcha/settings", async Task<IResult> (
            CaptchaSettingsRequest request,
            CaptchaChallengeService captcha,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await captcha.UpdateSettingsAsync(request, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<CaptchaSettingsResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);
        group.MapPost("/captcha/test", async Task<IResult> (
            CaptchaTestRequest request,
            CaptchaChallengeService captcha,
            CancellationToken ct) =>
        {
            var result = await captcha.TestAsync(request, ct);
            return result.Status switch
            {
                "unavailable" => TypedResults.Json(result, statusCode: StatusCodes.Status503ServiceUnavailable),
                "failed" => TypedResults.Json(result, statusCode: StatusCodes.Status400BadRequest),
                _ => TypedResults.Ok(result),
            };
        })
            .Produces<CaptchaTestResponse>(StatusCodes.Status200OK)
            .Produces<CaptchaTestResponse>(StatusCodes.Status400BadRequest)
            .Produces<CaptchaTestResponse>(StatusCodes.Status503ServiceUnavailable);
        group.MapPost("/access-log", async Task<IResult> (AccessLogIngestRequest request, SecurityIngestionService security, CancellationToken ct) =>
        {
            await security.IngestAccessLogAsync(request, ct);
            return TypedResults.Ok(new { accepted = true });
        });
        group.MapPost("/waf-events", async Task<IResult> (WafEventIngestRequest request, SecurityIngestionService security, CancellationToken ct) =>
        {
            await security.IngestWafEventAsync(request, ct);
            return TypedResults.Ok(new { accepted = true });
        });
        group.MapPost("/blocklist/sync", async Task<IResult> (SecurityIngestionService security, CancellationToken ct) =>
        {
            var result = await security.SyncBlocklistToAllFirewallsAsync(ct);
            return TypedResults.Ok(result);
        }).Produces<BlocklistSyncResponse>(StatusCodes.Status200OK);

        group.MapGet("/blocklists", async (BlocklistSourceManagementService blocklists, CancellationToken ct) =>
            TypedResults.Ok(await blocklists.ListAsync(ct)))
            .Produces<IEnumerable<BlocklistSourceResponse>>(StatusCodes.Status200OK);
        group.MapPost("/blocklists", async Task<IResult> (
            UpsertBlocklistSourceRequest request,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await blocklists.CreateAsync(request, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<BlocklistSourceResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);
        group.MapGet("/blocklists/{id:guid}", async Task<IResult> (
            Guid id,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
        {
            var source = await blocklists.GetAsync(id, ct);
            return source is null ? TypedResults.NotFound() : TypedResults.Ok(source);
        })
            .Produces<BlocklistSourceResponse>(StatusCodes.Status200OK);
        group.MapPatch("/blocklists/{id:guid}", async Task<IResult> (
            Guid id,
            UpsertBlocklistSourceRequest request,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
        {
            try
            {
                var source = await blocklists.UpdateAsync(id, request, ct);
                return source is null ? TypedResults.NotFound() : TypedResults.Ok(source);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<BlocklistSourceResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);
        group.MapDelete("/blocklists/{id:guid}", async Task<IResult> (
            Guid id,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
            await blocklists.DeleteAsync(id, ct) ? TypedResults.NoContent() : TypedResults.NotFound());
        group.MapPost("/blocklists/{id:guid}/fetch-preview", async Task<IResult> (
            Guid id,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
        {
            try
            {
                var preview = await blocklists.PreviewAsync(id, ct);
                return preview is null ? TypedResults.NotFound() : TypedResults.Ok(preview);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<BlocklistFetchPreviewResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);
        group.MapPost("/blocklists/{id:guid}/enable", async Task<IResult> (
            Guid id,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
        {
            var result = await blocklists.EnableAsync(id, ct);
            return result is null ? TypedResults.NotFound() : TypedResults.Ok(result);
        })
            .Produces<BlocklistSourceMutationResponse>(StatusCodes.Status200OK);
        group.MapPost("/blocklists/{id:guid}/disable", async Task<IResult> (
            Guid id,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
        {
            var result = await blocklists.DisableAsync(id, ct);
            return result is null ? TypedResults.NotFound() : TypedResults.Ok(result);
        })
            .Produces<BlocklistSourceMutationResponse>(StatusCodes.Status200OK);
        group.MapPost("/blocklists/{id:guid}/refresh", async Task<IResult> (
            Guid id,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
        {
            var result = await blocklists.RefreshAsync(id, ct);
            return result is null ? TypedResults.NotFound() : TypedResults.Ok(result);
        })
            .Produces<BlocklistSourceMutationResponse>(StatusCodes.Status200OK);
        group.MapGet("/blocklists/{id:guid}/runs", async (
            Guid id,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
            TypedResults.Ok(await blocklists.ListRunsAsync(id, ct)))
            .Produces<IEnumerable<BlocklistFetchRunResponse>>(StatusCodes.Status200OK);
        group.MapGet("/blocklists/{id:guid}/entries", async (
            Guid id,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
            TypedResults.Ok(await blocklists.ListEntriesAsync(id, ct)))
            .Produces<IEnumerable<BlocklistEntryResponse>>(StatusCodes.Status200OK);

        group.MapGet("/profiles", async (HashiDbContext db, CancellationToken ct) =>
        {
            var profiles = await db.SecurityProfiles.AsNoTracking().ToListAsync(ct);
            return TypedResults.Ok(profiles.Select(p => new SecurityProfileResponse(
                p.Name,
                p.ForwardAuthPolicy,
                p.WafMode,
                p.RateLimitAverage,
                p.RateLimitBurst)));
        }).Produces<IEnumerable<SecurityProfileResponse>>(StatusCodes.Status200OK);

        group.MapPost("/profiles", async Task<IResult> (CreateSecurityProfileRequest request, HashiDbContext db, CancellationToken ct) =>
        {
            if (string.IsNullOrWhiteSpace(request.Name))
            {
                return TypedResults.BadRequest(new ApiErrorResponse("Profile name cannot be empty."));
            }

            var name = request.Name.Trim();
            if (await db.SecurityProfiles.AnyAsync(x => x.Name.ToLower() == name.ToLower(), ct))
            {
                return TypedResults.BadRequest(new ApiErrorResponse($"Profile '{name}' already exists."));
            }

            var profile = new SecurityProfileEntity
            {
                Name = name,
                ForwardAuthPolicy = request.ForwardAuthPolicy ?? "adaptive",
                WafMode = request.WafMode ?? "detect_only",
                RateLimitAverage = request.RateLimitAverage,
                RateLimitBurst = request.RateLimitBurst
            };

            db.SecurityProfiles.Add(profile);
            await db.SaveChangesAsync(ct);

            return TypedResults.Ok(new SecurityProfileResponse(
                profile.Name,
                profile.ForwardAuthPolicy,
                profile.WafMode,
                profile.RateLimitAverage,
                profile.RateLimitBurst));
        })
        .Produces<SecurityProfileResponse>(StatusCodes.Status200OK)
        .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);

        group.MapPut("/profiles/{name}", async Task<IResult> (string name, UpdateSecurityProfileRequest request, HashiDbContext db, CancellationToken ct) =>
        {
            var profile = await db.SecurityProfiles.SingleOrDefaultAsync(x => x.Name.ToLower() == name.ToLower(), ct);
            if (profile is null)
            {
                return TypedResults.NotFound();
            }

            profile.ForwardAuthPolicy = request.ForwardAuthPolicy ?? "adaptive";
            profile.WafMode = request.WafMode ?? "detect_only";
            profile.RateLimitAverage = request.RateLimitAverage;
            profile.RateLimitBurst = request.RateLimitBurst;

            await db.SaveChangesAsync(ct);

            return TypedResults.Ok(new SecurityProfileResponse(
                profile.Name,
                profile.ForwardAuthPolicy,
                profile.WafMode,
                profile.RateLimitAverage,
                profile.RateLimitBurst));
        })
        .Produces<SecurityProfileResponse>(StatusCodes.Status200OK)
        .Produces(StatusCodes.Status404NotFound);

        group.MapDelete("/profiles/{name}", async Task<IResult> (string name, HashiDbContext db, CancellationToken ct) =>
        {
            var profile = await db.SecurityProfiles.SingleOrDefaultAsync(x => x.Name.ToLower() == name.ToLower(), ct);
            if (profile is null)
            {
                return TypedResults.NotFound();
            }

            var nameLower = name.ToLower();
            if (await db.Resources.AnyAsync(r => r.SecurityProfileName != null && r.SecurityProfileName.ToLower() == nameLower, ct))
            {
                return TypedResults.BadRequest(new ApiErrorResponse($"Profile '{profile.Name}' is currently in use by one or more resources."));
            }

            db.SecurityProfiles.Remove(profile);
            await db.SaveChangesAsync(ct);

            return TypedResults.NoContent();
        })
        .Produces(StatusCodes.Status204NoContent)
        .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest)
        .Produces(StatusCodes.Status404NotFound);

        return app;
    }
}
