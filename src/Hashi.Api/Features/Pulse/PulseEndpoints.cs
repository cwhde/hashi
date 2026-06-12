using Hashi.Api.Hosting;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.HttpResults;
using Microsoft.AspNetCore.Routing;
using Microsoft.EntityFrameworkCore;
using System.Text.Json;

namespace Hashi.Api.Features.Pulse;

public static class PulseEndpoints
{
    public static IEndpointRouteBuilder MapPulseEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/pulse").WithTags("Pulse");
        group.MapGet("/agents", async (HashiDbContext db, CancellationToken ct) =>
        {
            var agents = await db.PulseAgents.AsNoTracking().ToListAsync(ct);
            return TypedResults.Ok(agents.Select(PulseAgentService.ToResponse));
        });
        group.MapGet("/agents/{agentId:guid}/resolved-targets", async Task<IResult> (
            Guid agentId,
            HashiDbContext db,
            ConnectionTargetResolver targets,
            CancellationToken ct) =>
        {
            var agent = await db.PulseAgents.AsNoTracking().SingleOrDefaultAsync(x => x.Id == agentId, ct);
            if (agent is null)
            {
                return TypedResults.NotFound();
            }

            var target = new ConnectionTargetEntity
            {
                OwnerType = "pulse_agent_preview",
                OwnerId = agentId,
                TargetMode = ConnectionTargetModeNames.PulseAgent,
                PulseAgentId = agentId,
                PulseIpMode = PulseTargetIpModeNames.Selected,
                PrivateCandidateSelector = PulsePrivateCandidateSelectorNames.Selected,
                Scheme = "http",
                Port = 80,
            };
            var resolved = await targets.ResolveAsync(target, persistSnapshot: false, cancellationToken: ct);
            return TypedResults.Ok(new PulseResolvedTargetResponse(
                agent.Id,
                agent.Name,
                PulseTargetIpModeNames.Selected,
                agent.LastSelectedIp,
                agent.LastPublicIp,
                DeserializeStringList(agent.LastPrivateIpv4CandidatesJson),
                DeserializeStringList(agent.LastPrivateIpv6CandidatesJson),
                agent.LastSeenAtUtc,
                resolved.Status,
                resolved.ResolvedIp,
                resolved.Error));
        })
            .Produces<PulseResolvedTargetResponse>(StatusCodes.Status200OK);
        group.MapPost("/agents", async Task<Ok<CreatePulseAgentResponse>> (CreatePulseAgentRequest request, PulseAgentService pulse, CancellationToken ct) =>
        {
            var created = await pulse.CreateAgentAsync(request, ct);
            return TypedResults.Ok(created);
        });
        group.MapPost("/agents/{agentId:guid}/revoke", async Task<IResult> (Guid agentId, PulseAgentService pulse, CancellationToken ct) =>
        {
            var revoked = await pulse.RevokeAgentAsync(agentId, ct);
            return revoked ? TypedResults.Ok(new { revoked = true }) : TypedResults.NotFound();
        });
        group.MapPost("/agents/{agentId:guid}/rotate-token", async Task<IResult> (Guid agentId, PulseAgentService pulse, CancellationToken ct) =>
        {
            var rotated = await pulse.RotateTokenAsync(agentId, ct);
            return rotated is null ? TypedResults.NotFound() : TypedResults.Ok(rotated);
        })
            .Produces<RotatePulseAgentTokenResponse>(StatusCodes.Status200OK);
        group.MapGet("/agents/{agentId:guid}/install", (HttpContext ctx, Guid agentId) =>
        {
            if (ctx.Request.Query.ContainsKey("token"))
            {
                return Results.BadRequest(new ApiErrorResponse("Pulse tokens must not be sent in URLs."));
            }

            var apiBase = $"{ctx.Request.Scheme}://{ctx.Request.Host}";
            return TypedResults.Ok(PulseInstallRenderer.Render(apiBase, agentId));
        })
            .Produces<PulseInstallResponse>(StatusCodes.Status200OK);
        group.MapGet("/install/linux.sh", () =>
        {
            var candidates = new[]
            {
                Path.Combine(AppContext.BaseDirectory, "content", "pulse", "install.sh"),
                Path.GetFullPath(Path.Combine(AppContext.BaseDirectory, "..", "..", "..", "..", "agents", "pulse", "install.sh")),
                Path.GetFullPath(Path.Combine(Directory.GetCurrentDirectory(), "agents", "pulse", "install.sh")),
            };
            var path = candidates.FirstOrDefault(File.Exists);
            if (path is null)
            {
                return Results.NotFound();
            }

            return Results.Text(File.ReadAllText(path), "text/x-shellscript");
        }).AllowAnonymous();
        group.MapPost("/{agentId:guid}/heartbeat", async Task<IResult> (
            Guid agentId,
            PulseHeartbeatAuthRequest request,
            HttpContext ctx,
            PulseAgentService pulse,
            CancellationToken ct) =>
        {
            var result = await pulse.AcceptHeartbeatAsync(
                agentId,
                request,
                ctx.Connection.RemoteIpAddress?.ToString(),
                ct);
            return result switch
            {
                PulseHeartbeatAcceptResult.Accepted => TypedResults.Ok(new { accepted = true }),
                PulseHeartbeatAcceptResult.InvalidTimestamp => TypedResults.BadRequest(new ApiErrorResponse("Heartbeat timestamp is outside the accepted clock skew.")),
                PulseHeartbeatAcceptResult.InvalidScope => TypedResults.StatusCode(StatusCodes.Status403Forbidden),
                _ => TypedResults.Unauthorized(),
            };
        }).AllowAnonymous();
        return app;
    }

    private static IReadOnlyList<string> DeserializeStringList(string? json)
    {
        if (string.IsNullOrWhiteSpace(json))
        {
            return [];
        }

        try
        {
            return JsonSerializer.Deserialize<IReadOnlyList<string>>(json) ?? [];
        }
        catch (JsonException)
        {
            return [];
        }
    }
}
