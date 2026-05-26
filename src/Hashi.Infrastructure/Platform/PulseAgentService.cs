using Hashi.Core.Dns;
using Hashi.Core.Resources;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Platform;

public sealed class PulseAgentService(
    HashiDbContext db,
    DnsConnectionService dns,
    ILogger<PulseAgentService> logger)
{
    public async Task<CreatePulseAgentResponse> CreateAgentAsync(CreatePulseAgentRequest request, CancellationToken cancellationToken = default)
    {
        var token = Convert.ToBase64String(System.Security.Cryptography.RandomNumberGenerator.GetBytes(32));
        var hash = Convert.ToHexString(System.Security.Cryptography.SHA256.HashData(System.Text.Encoding.UTF8.GetBytes(token))).ToLowerInvariant();
        var agent = new PulseAgentEntity
        {
            Name = request.Name,
            TokenHash = hash,
            Status = "pending",
        };
        db.PulseAgents.Add(agent);
        await db.SaveChangesAsync(cancellationToken);
        return new CreatePulseAgentResponse(agent.Id, agent.Name, token);
    }

    public async Task<bool> AcceptHeartbeatAsync(
        Guid agentId,
        PulseHeartbeatAuthRequest request,
        string? remotePublicIp,
        CancellationToken cancellationToken = default)
    {
        var agent = await db.PulseAgents.SingleOrDefaultAsync(x => x.Id == agentId, cancellationToken);
        if (agent is null)
        {
            return false;
        }

        var hash = Convert.ToHexString(System.Security.Cryptography.SHA256.HashData(System.Text.Encoding.UTF8.GetBytes(request.Token))).ToLowerInvariant();
        if (!string.Equals(agent.TokenHash, hash, StringComparison.Ordinal))
        {
            return false;
        }

        var publicIp = remotePublicIp ?? request.PrivateIpv4Candidates.FirstOrDefault();
        var internalIp = request.PrivateIpv4Candidates.FirstOrDefault();
        var ipChanged = !string.Equals(agent.LastPublicIp, publicIp, StringComparison.OrdinalIgnoreCase)
            || !string.Equals(agent.LastPrivateIp, internalIp, StringComparison.OrdinalIgnoreCase);

        agent.LastSeenAtUtc = DateTimeOffset.UtcNow;
        agent.LastPublicIp = publicIp;
        agent.LastPrivateIp = internalIp;
        agent.Status = "online";
        await db.SaveChangesAsync(cancellationToken);

        if (ipChanged)
        {
            await ApplyDnsForPulseChangeAsync(agent, cancellationToken);
        }

        return true;
    }

    public async Task<bool> RevokeAgentAsync(Guid agentId, CancellationToken cancellationToken = default)
    {
        var agent = await db.PulseAgents.SingleOrDefaultAsync(x => x.Id == agentId, cancellationToken);
        if (agent is null)
        {
            return false;
        }

        agent.TokenHash = Convert.ToHexString(System.Security.Cryptography.SHA256.HashData(
            System.Text.Encoding.UTF8.GetBytes($"revoked:{agentId}:{Guid.NewGuid()}"))).ToLowerInvariant();
        agent.Status = "revoked";
        await db.SaveChangesAsync(cancellationToken);
        return true;
    }

    private async Task ApplyDnsForPulseChangeAsync(PulseAgentEntity agent, CancellationToken cancellationToken)
    {
        var syncRun = new SyncRunEntity
        {
            Subsystem = "dns-pulse",
            Status = SyncRunStatusNames.Applying,
            RiskLevel = nameof(Core.Sync.SyncRiskLevel.Low),
            ErrorSummary = $"Pulse agent {agent.Name} IP change applying DNS sync.",
        };
        db.SyncRuns.Add(syncRun);
        await db.SaveChangesAsync(cancellationToken);

        var errors = new List<string>();
        var appliedConnections = 0;
        var connections = await db.Connections.AsNoTracking()
            .Where(x => x.Type == ConnectionTypeNames.DnsProvider && x.Enabled)
            .ToListAsync(cancellationToken);

        foreach (var connection in connections)
        {
            try
            {
                var plan = await dns.PlanSyncAsync(connection.Id, cancellationToken);
                if (plan.RequiresConfirmation)
                {
                    await dns.ApplySafePlanAsync(plan, cancellationToken);
                }
                else if (plan.Changes.Any(x => x.Kind != DnsChangeKind.NoOp))
                {
                    await dns.ApplyPlanAsync(plan, confirmDestructive: true, cancellationToken);
                }

                appliedConnections++;
            }
            catch (Exception ex)
            {
                errors.Add($"{connection.Name}: {ex.Message}");
                logger.LogWarning(ex, "Pulse DNS sync failed for connection {ConnectionName}", connection.Name);
            }
        }

        syncRun.CompletedAtUtc = DateTimeOffset.UtcNow;
        syncRun.Status = errors.Count == 0 ? SyncRunStatusNames.Succeeded : SyncRunStatusNames.Failed;
        syncRun.ErrorSummary = errors.Count == 0
            ? $"Pulse agent {agent.Name} DNS sync applied to {appliedConnections} connection(s)."
            : string.Join("; ", errors);
        await db.SaveChangesAsync(cancellationToken);
    }
}
