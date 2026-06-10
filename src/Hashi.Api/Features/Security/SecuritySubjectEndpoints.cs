using Hashi.Api.Hosting;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.HttpResults;
using Microsoft.AspNetCore.Routing;

namespace Hashi.Api.Features.Security;

public static class SecuritySubjectEndpoints
{
    public static IEndpointRouteBuilder MapSecuritySubjectEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/security").WithTags("Security");

        group.MapGet("/subjects/search", async (
            string? q,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
            TypedResults.Ok(await subjects.SearchAsync(q, ct)))
            .Produces<SecuritySubjectSearchResponse>(StatusCodes.Status200OK);

        group.MapGet("/subjects/{id:guid}", async Task<IResult> (
            Guid id,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            var detail = await subjects.GetDetailAsync(id, ct);
            return detail is null ? TypedResults.NotFound() : TypedResults.Ok(detail);
        })
            .Produces<SecuritySubjectDetailResponse>(StatusCodes.Status200OK);

        group.MapGet("/subjects/{id:guid}/events", async (
            Guid id,
            string? eventType,
            Guid? resourceId,
            int? limit,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
            TypedResults.Ok(await subjects.ListEventsAsync(id, eventType, resourceId, limit ?? 100, ct)))
            .Produces<IEnumerable<SecurityEventResponse>>(StatusCodes.Status200OK);

        group.MapGet("/subjects/{id:guid}/buckets", async (
            Guid id,
            int? hours,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
            TypedResults.Ok(await subjects.ListBucketsAsync(id, hours ?? 24, ct)))
            .Produces<IEnumerable<SecurityRequestBucketResponse>>(StatusCodes.Status200OK);

        group.MapGet("/subjects/{id:guid}/effective-decision", async Task<IResult> (
            Guid id,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            var decision = await subjects.GetEffectiveDecisionAsync(id, ct);
            return decision is null ? TypedResults.NotFound() : TypedResults.Ok(decision);
        })
            .Produces<SecurityEffectiveDecisionResponse>(StatusCodes.Status200OK);

        group.MapPost("/manual-entries", async Task<IResult> (
            UpsertManualSecurityEntryRequest request,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await subjects.CreateManualEntryAsync(request, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<ManualSecurityEntryResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);

        group.MapPatch("/manual-entries/{id:guid}", async Task<IResult> (
            Guid id,
            UpdateManualSecurityEntryRequest request,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            try
            {
                var entry = await subjects.UpdateManualEntryAsync(id, request, ct);
                return entry is null ? TypedResults.NotFound() : TypedResults.Ok(entry);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<ManualSecurityEntryResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);

        group.MapDelete("/manual-entries/{id:guid}", async Task<IResult> (
            Guid id,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            var deleted = await subjects.DeleteManualEntryAsync(id, ct);
            return deleted ? TypedResults.NoContent() : TypedResults.NotFound();
        });

        group.MapPost("/manual-entries/{id:guid}/expire", async Task<IResult> (
            Guid id,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            var entry = await subjects.ExpireManualEntryAsync(id, ct);
            return entry is null ? TypedResults.NotFound() : TypedResults.Ok(entry);
        })
            .Produces<ManualSecurityEntryResponse>(StatusCodes.Status200OK);

        group.MapPost("/blocks", async Task<IResult> (
            CreateSecurityBlockRequest request,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await subjects.CreateBlockAsync(request, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<SecurityBlockMutationResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);

        group.MapPatch("/blocks/{id:guid}", async Task<IResult> (
            Guid id,
            UpdateSecurityBlockRequest request,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            try
            {
                var entry = await subjects.UpdateBlockAsync(id, request, ct);
                return entry is null ? TypedResults.NotFound() : TypedResults.Ok(entry);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<SecurityBlockMutationResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);

        group.MapPost("/blocks/{id:guid}/extend", async Task<IResult> (
            Guid id,
            SecurityBlockDurationRequest request,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            var entry = await subjects.ExtendBlockAsync(id, request.DurationSeconds, ct);
            return entry is null ? TypedResults.NotFound() : TypedResults.Ok(entry);
        })
            .Produces<SecurityBlockMutationResponse>(StatusCodes.Status200OK);

        group.MapPost("/blocks/{id:guid}/shorten", async Task<IResult> (
            Guid id,
            SecurityBlockDurationRequest request,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            var entry = await subjects.ShortenBlockAsync(id, request.DurationSeconds, ct);
            return entry is null ? TypedResults.NotFound() : TypedResults.Ok(entry);
        })
            .Produces<SecurityBlockMutationResponse>(StatusCodes.Status200OK);

        group.MapPost("/blocks/{id:guid}/make-permanent", async Task<IResult> (
            Guid id,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            var entry = await subjects.MakePermanentAsync(id, ct);
            return entry is null ? TypedResults.NotFound() : TypedResults.Ok(entry);
        })
            .Produces<SecurityBlockMutationResponse>(StatusCodes.Status200OK);

        group.MapPost("/blocks/{id:guid}/expire", async Task<IResult> (
            Guid id,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            var entry = await subjects.ExpireBlockAsync(id, ct);
            return entry is null ? TypedResults.NotFound() : TypedResults.Ok(entry);
        })
            .Produces<SecurityBlockMutationResponse>(StatusCodes.Status200OK);

        group.MapPost("/blocks/{id:guid}/preview-firewall-sync", async Task<IResult> (
            Guid id,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            try
            {
                var preview = await subjects.PreviewFirewallSyncAsync(id, ct);
                return preview is null ? TypedResults.NotFound() : TypedResults.Ok(preview);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<FirewallPlanPreviewResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);

        return app;
    }
}
