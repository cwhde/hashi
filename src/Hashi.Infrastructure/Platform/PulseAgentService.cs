using System.Security.Cryptography;
using System.Text;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class PulseAgentService(HashiDbContext db)
{
    public async Task<CreatePulseAgentResponse> CreateAgentAsync(CreatePulseAgentRequest request, CancellationToken cancellationToken = default)
    {
        var token = Convert.ToBase64String(RandomNumberGenerator.GetBytes(32));
        var hash = Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(token))).ToLowerInvariant();
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

    public async Task<bool> AcceptHeartbeatAsync(Guid agentId, PulseHeartbeatAuthRequest request, CancellationToken cancellationToken = default)
    {
        var agent = await db.PulseAgents.SingleOrDefaultAsync(x => x.Id == agentId, cancellationToken);
        if (agent is null)
        {
            return false;
        }

        var hash = Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(request.Token))).ToLowerInvariant();
        if (!string.Equals(agent.TokenHash, hash, StringComparison.Ordinal))
        {
            return false;
        }

        agent.LastSeenAtUtc = DateTimeOffset.UtcNow;
        agent.LastPublicIp = request.PrivateIpv4Candidates.FirstOrDefault();
        agent.Status = "online";
        await db.SaveChangesAsync(cancellationToken);
        return true;
    }
}
