using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Routing;

namespace Hashi.Api.Features.Firewall;

public static class FirewallEndpoints
{
    public static IEndpointRouteBuilder MapFirewallEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/firewall").WithTags("Firewall");
        group.MapGet("/hosts", async (FirewallApplyService firewall, CancellationToken ct) =>
            TypedResults.Ok(await firewall.ListHostsAsync(ct)));
        group.MapPost("/render", (FirewallRenderRequest request, FirewallPlatformService firewall) =>
            TypedResults.Ok(firewall.Render(request)));
        group.MapPost("/hosts", async Task<IResult> (CreateFirewallHostRequest request, FirewallApplyService firewall, CancellationToken ct) =>
        {
            var host = await firewall.UpsertHostAsync(request, ct);
            return TypedResults.Ok(FirewallApplyService.ToResponse(host));
        })
            .Produces<FirewallHostResponse>(StatusCodes.Status200OK);
        group.MapPut("/hosts/{firewallHostId:guid}", async Task<IResult> (
            Guid firewallHostId,
            UpdateFirewallHostRequest request,
            FirewallApplyService firewall,
            CancellationToken ct) =>
        {
            var host = await firewall.UpdateHostAsync(firewallHostId, request, ct);
            return host is null ? TypedResults.NotFound() : TypedResults.Ok(FirewallApplyService.ToResponse(host));
        })
            .Produces<FirewallHostResponse>(StatusCodes.Status200OK);
        group.MapPost("/hosts/{firewallHostId:guid}/plan", async (
            Guid firewallHostId,
            FirewallApplyService firewall,
            CancellationToken ct) =>
            TypedResults.Ok(await firewall.PlanForHostAsync(firewallHostId, ct)))
            .Produces<FirewallPlanPreviewResponse>(StatusCodes.Status200OK);
        group.MapPost("/apply", async Task<IResult> (FirewallApplyRequest request, FirewallApplyService firewall, CancellationToken ct) =>
        {
            var result = await firewall.ApplyAsync(request, ct);
            return TypedResults.Ok(result);
        });
        group.MapPost("/{firewallHostId:guid}/rollback", async Task<IResult> (
            Guid firewallHostId,
            FirewallApplyRequest request,
            FirewallApplyService firewall,
            CancellationToken ct) =>
        {
            var result = await firewall.RollbackAsync(firewallHostId, request, ct);
            return TypedResults.Ok(result);
        });
        return app;
    }
}
