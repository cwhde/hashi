using System.Text.Json;
using Hashi.Core.Dns;
using Hashi.Core.Sync;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Platform;

public sealed class PulseAgentService(
    HashiDbContext db,
    DnsConnectionService dns,
    AuditService audit,
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
        await audit.WriteAsync("pulse", "agent_created", subjectType: "pulse_agent", subjectId: agent.Id.ToString(), cancellationToken: cancellationToken);
        return new CreatePulseAgentResponse(agent.Id, agent.Name, token);
    }

    public async Task<RotatePulseAgentTokenResponse?> RotateTokenAsync(Guid agentId, CancellationToken cancellationToken = default)
    {
        var agent = await db.PulseAgents.SingleOrDefaultAsync(x => x.Id == agentId, cancellationToken);
        if (agent is null || agent.Status == "revoked")
        {
            return null;
        }

        var token = Convert.ToBase64String(System.Security.Cryptography.RandomNumberGenerator.GetBytes(32));
        agent.TokenHash = Convert.ToHexString(System.Security.Cryptography.SHA256.HashData(System.Text.Encoding.UTF8.GetBytes(token))).ToLowerInvariant();
        agent.Status = "pending";
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("pulse", "token_rotated", subjectType: "pulse_agent", subjectId: agent.Id.ToString(), cancellationToken: cancellationToken);
        return new RotatePulseAgentTokenResponse(agent.Id, agent.Name, token);
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
        agent.LastHostname = request.Hostname;
        agent.LastAgentVersion = request.Version;
        agent.Status = "online";

        if (ipChanged)
        {
            agent.DnsPendingAtUtc = DateTimeOffset.UtcNow;
        }

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
        agent.DnsPendingAtUtc = null;
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("pulse", "token_revoked", subjectType: "pulse_agent", subjectId: agent.Id.ToString(), cancellationToken: cancellationToken);
        return true;
    }

    public static PulseAgentResponse ToResponse(PulseAgentEntity agent) => new(
        agent.Id,
        agent.Name,
        agent.Status,
        agent.LastSeenAtUtc,
        agent.LastPublicIp,
        agent.LastHostname,
        agent.LastAgentVersion,
        agent.DnsPendingAtUtc);

    private async Task ApplyDnsForPulseChangeAsync(PulseAgentEntity agent, CancellationToken cancellationToken)
    {
        var syncRun = new SyncRunEntity
        {
            Subsystem = "dns-pulse",
            Status = SyncRunStatusNames.Applying,
            RiskLevel = nameof(SyncRiskLevel.Low),
            ErrorSummary = $"Pulse agent {agent.Name} IP change applying DNS sync.",
        };
        db.SyncRuns.Add(syncRun);
        await db.SaveChangesAsync(cancellationToken);

        var errors = new List<string>();
        var appliedConnections = 0;
        var pendingConnections = 0;
        var connections = await db.Connections.AsNoTracking()
            .Where(x => x.Type == ConnectionTypeNames.DnsProvider && x.Enabled)
            .ToListAsync(cancellationToken);

        foreach (var connection in connections)
        {
            try
            {
                var plan = await dns.PlanSyncAsync(connection.Id, cancellationToken);
                var changes = plan.Changes.Where(x => x.Kind != DnsChangeKind.NoOp).ToList();
                foreach (var change in changes)
                {
                    db.SyncDiffs.Add(new SyncDiffEntity
                    {
                        SyncRunId = syncRun.Id,
                        ResourceType = "dns",
                        ResourceKey = $"{change.Name}/{DnsRecordTypeMapping.ToApiName(change.Type)}",
                        ChangeKind = MapDnsKind(change.Kind).ToString(),
                        Summary = $"Pulse agent {agent.Name}: {change.RiskReason}",
                        BeforeJson = JsonSerializer.Serialize(new { value = change.CurrentValue, ttl = change.Ttl }),
                        AfterJson = JsonSerializer.Serialize(new { value = change.DesiredValue, ttl = change.Ttl }),
                    });
                }

                if (plan.RequiresConfirmation)
                {
                    pendingConnections++;
                    await dns.ApplySafePlanAsync(plan, cancellationToken);
                    db.SyncSteps.Add(new SyncStepEntity
                    {
                        SyncRunId = syncRun.Id,
                        Name = $"dns-pulse-{connection.Name}",
                        Status = SyncRunStatusNames.Succeeded,
                        CompletedAtUtc = DateTimeOffset.UtcNow,
                        Message = "Safe changes applied; destructive changes pending confirmation.",
                    });
                }
                else if (changes.Count > 0)
                {
                    await dns.ApplyPlanAsync(plan, confirmDestructive: true, cancellationToken);
                    db.SyncSteps.Add(new SyncStepEntity
                    {
                        SyncRunId = syncRun.Id,
                        Name = $"dns-pulse-{connection.Name}",
                        Status = SyncRunStatusNames.Succeeded,
                        CompletedAtUtc = DateTimeOffset.UtcNow,
                        Message = "Applied",
                    });
                }
                else
                {
                    db.SyncSteps.Add(new SyncStepEntity
                    {
                        SyncRunId = syncRun.Id,
                        Name = $"dns-pulse-{connection.Name}",
                        Status = SyncRunStatusNames.Succeeded,
                        CompletedAtUtc = DateTimeOffset.UtcNow,
                        Message = "No changes",
                    });
                }

                appliedConnections++;
            }
            catch (Exception ex)
            {
                errors.Add($"{connection.Name}: {ex.Message}");
                db.SyncSteps.Add(new SyncStepEntity
                {
                    SyncRunId = syncRun.Id,
                    Name = $"dns-pulse-{connection.Name}",
                    Status = SyncRunStatusNames.Failed,
                    CompletedAtUtc = DateTimeOffset.UtcNow,
                    Message = ex.Message,
                });
                logger.LogWarning(ex, "Pulse DNS sync failed for connection {ConnectionName}", connection.Name);
            }
        }

        syncRun.CompletedAtUtc = DateTimeOffset.UtcNow;
        syncRun.Status = errors.Count > 0
            ? SyncRunStatusNames.Failed
            : pendingConnections > 0
                ? SyncRunStatusNames.AwaitingConfirmation
                : SyncRunStatusNames.Succeeded;
        syncRun.RiskLevel = pendingConnections > 0
            ? nameof(SyncRiskLevel.Destructive)
            : nameof(SyncRiskLevel.Low);
        syncRun.ErrorSummary = errors.Count == 0
            ? pendingConnections > 0
                ? $"Pulse agent {agent.Name} DNS sync applied safe changes to {appliedConnections} connection(s); destructive changes require confirmation."
                : $"Pulse agent {agent.Name} DNS sync applied to {appliedConnections} connection(s)."
            : string.Join("; ", errors);
        if (errors.Count == 0 && pendingConnections == 0)
        {
            agent.DnsPendingAtUtc = null;
        }

        await db.SaveChangesAsync(cancellationToken);
    }

    private static ProviderResultKind MapDnsKind(DnsChangeKind kind) => kind switch
    {
        DnsChangeKind.Create => ProviderResultKind.Created,
        DnsChangeKind.Update => ProviderResultKind.Updated,
        DnsChangeKind.Delete => ProviderResultKind.Deleted,
        _ => ProviderResultKind.NoOp,
    };
}
