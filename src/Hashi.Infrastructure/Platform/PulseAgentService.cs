using Hashi.Core.Dns;
using Hashi.Core.Resources;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class PulseAgentService(HashiDbContext db)
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
        var ipChanged = !string.Equals(agent.LastPublicIp, publicIp, StringComparison.OrdinalIgnoreCase);

        agent.LastSeenAtUtc = DateTimeOffset.UtcNow;
        agent.LastPublicIp = publicIp;
        agent.Status = "online";
        await db.SaveChangesAsync(cancellationToken);

        if (ipChanged)
        {
            await QueueDnsSyncForPulseAsync(agent, publicIp, internalIp, cancellationToken);
        }

        return true;
    }

    private async Task QueueDnsSyncForPulseAsync(
        PulseAgentEntity agent,
        string? publicIp,
        string? internalIp,
        CancellationToken cancellationToken)
    {
        var settings = await db.AppSettings.SingleOrDefaultAsync(cancellationToken);
        var rootDomain = settings?.RootDomain ?? "local";
        var hosts = await db.FirewallHosts.AsNoTracking().ToListAsync(cancellationToken);
        var hostTargets = hosts.Select(h => new FirewallHostDnsTarget(
            h.Id,
            h.Name,
            h.PublicIp ?? h.InternalTraefikIp,
            null)).ToList();
        var records = DnsRecordGenerator.GenerateResourceRecords(
            new ResourceDnsTarget(agent.Name, ResourceSlug.Normalize(agent.Name), rootDomain, null, publicIp, new PulseDnsTarget(agent.Id, publicIp, internalIp)),
            hostTargets);

        if (records.Count == 0)
        {
            return;
        }

        db.SyncRuns.Add(new SyncRunEntity
        {
            Subsystem = "dns-pulse",
            Status = SyncRunStatusNames.Pending,
            RiskLevel = nameof(Core.Sync.SyncRiskLevel.Low),
            ErrorSummary = $"Pulse agent {agent.Name} IP change queued DNS sync.",
        });
        await db.SaveChangesAsync(cancellationToken);
    }
}
