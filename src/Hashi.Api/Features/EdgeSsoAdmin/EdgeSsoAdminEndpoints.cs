using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Routing;

namespace Hashi.Api.Features.EdgeSsoAdmin;

public static class EdgeSsoAdminEndpoints
{
    public static IEndpointRouteBuilder MapEdgeSsoAdminEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/settings/edge-sso").WithTags("Settings");
        group.MapGet("/providers", async (OidcProviderAdminService admin, CancellationToken ct) =>
            TypedResults.Ok(await admin.ListProvidersAsync(ct)));
        group.MapPost("/providers", async Task<IResult> (
            CreateOidcProviderRequest request,
            OidcProviderAdminService admin,
            CancellationToken ct) =>
            TypedResults.Ok(await admin.CreateProviderAsync(request, ct)));
        group.MapPut("/providers/{providerId:guid}", async Task<IResult> (
            Guid providerId,
            UpdateOidcProviderRequest request,
            OidcProviderAdminService admin,
            CancellationToken ct) =>
        {
            var updated = await admin.UpdateProviderAsync(providerId, request, ct);
            return updated is null ? TypedResults.NotFound() : TypedResults.Ok(updated);
        });
        group.MapDelete("/providers/{providerId:guid}", async Task<IResult> (
            Guid providerId,
            OidcProviderAdminService admin,
            CancellationToken ct) =>
        {
            var deleted = await admin.DeleteProviderAsync(providerId, ct);
            return deleted ? TypedResults.NoContent() : TypedResults.NotFound();
        });
        group.MapGet("/rules", async (OidcProviderAdminService admin, CancellationToken ct) =>
            TypedResults.Ok(await admin.ListRulesAsync(ct)));
        group.MapPost("/rules", async Task<IResult> (
            CreateEdgeAuthRuleRequest request,
            OidcProviderAdminService admin,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await admin.CreateRuleAsync(request, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });
        group.MapPut("/rules/{ruleId:guid}", async Task<IResult> (
            Guid ruleId,
            UpdateEdgeAuthRuleRequest request,
            OidcProviderAdminService admin,
            CancellationToken ct) =>
        {
            try
            {
                var updated = await admin.UpdateRuleAsync(ruleId, request, ct);
                return updated is null ? TypedResults.NotFound() : TypedResults.Ok(updated);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });
        group.MapDelete("/rules/{ruleId:guid}", async Task<IResult> (
            Guid ruleId,
            OidcProviderAdminService admin,
            CancellationToken ct) =>
        {
            var deleted = await admin.DeleteRuleAsync(ruleId, ct);
            return deleted ? TypedResults.NoContent() : TypedResults.NotFound();
        });
        return app;
    }
}
