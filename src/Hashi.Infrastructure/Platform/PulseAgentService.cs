using System.Text.Json;
using System.Net;
using System.Net.Sockets;
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

public enum PulseHeartbeatAcceptResult
{
    Accepted,
    Unauthorized,
    InvalidTimestamp,
}

public sealed class PulseAgentService(
    HashiDbContext db,
    DnsConnectionService dns,
    AuditService audit,
    ConnectionTargetResolver targetResolver,
    ILogger<PulseAgentService> logger)
{
    private static readonly TimeSpan HeartbeatTimestampSkew = TimeSpan.FromMinutes(5);

    public async Task<CreatePulseAgentResponse> CreateAgentAsync(CreatePulseAgentRequest request, CancellationToken cancellationToken = default)
    {
        var token = Convert.ToBase64String(System.Security.Cryptography.RandomNumberGenerator.GetBytes(32));
        var hash = Convert.ToHexString(System.Security.Cryptography.SHA256.HashData(System.Text.Encoding.UTF8.GetBytes(token))).ToLowerInvariant();
        var allowedScopes = NormalizeAllowedScopes(request.AllowedScopes);
        var agent = new PulseAgentEntity
        {
            Name = request.Name,
            TokenHash = hash,
            InstallType = NormalizeInstallType(request.InstallType),
            AllowedScopesJson = JsonSerializer.Serialize(allowedScopes),
            HeartbeatIntervalSeconds = NormalizeHeartbeatInterval(request.HeartbeatIntervalSeconds),
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

    public async Task<PulseHeartbeatAcceptResult> AcceptHeartbeatAsync(
        Guid agentId,
        PulseHeartbeatAuthRequest request,
        string? remotePublicIp,
        CancellationToken cancellationToken = default)
    {
        var agent = await db.PulseAgents.SingleOrDefaultAsync(x => x.Id == agentId, cancellationToken);
        if (agent is null)
        {
            return PulseHeartbeatAcceptResult.Unauthorized;
        }

        var hash = Convert.ToHexString(System.Security.Cryptography.SHA256.HashData(System.Text.Encoding.UTF8.GetBytes(request.Token))).ToLowerInvariant();
        if (!string.Equals(agent.TokenHash, hash, StringComparison.Ordinal))
        {
            return PulseHeartbeatAcceptResult.Unauthorized;
        }

        var now = DateTimeOffset.UtcNow;
        if (request.Timestamp == default || (now - request.Timestamp.ToUniversalTime()).Duration() > HeartbeatTimestampSkew)
        {
            return PulseHeartbeatAcceptResult.InvalidTimestamp;
        }

        var ipv4Candidates = ValidatePrivateCandidates(request.PrivateIpv4Candidates, AddressFamily.InterNetwork);
        var ipv6Candidates = ValidatePrivateCandidates(request.PrivateIpv6Candidates, AddressFamily.InterNetworkV6);
        var selectedIp = NormalizeSelectedIp(request.SelectedIp, ipv4Candidates, ipv6Candidates);
        var selectedInterface = string.IsNullOrWhiteSpace(request.SelectedInterface)
            ? null
            : request.SelectedInterface.Trim();
        var publicIp = NormalizeIp(remotePublicIp) ?? ipv4Candidates.FirstOrDefault() ?? ipv6Candidates.FirstOrDefault();
        var internalIp = selectedIp ?? ipv4Candidates.FirstOrDefault() ?? ipv6Candidates.FirstOrDefault();
        var dockerMetadataJson = request.Docker is null ? null : JsonSerializer.Serialize(request.Docker);
        var ipChanged = !string.Equals(agent.LastPublicIp, publicIp, StringComparison.OrdinalIgnoreCase)
            || !string.Equals(agent.LastPrivateIp, internalIp, StringComparison.OrdinalIgnoreCase)
            || !string.Equals(agent.LastSelectedIp, selectedIp, StringComparison.OrdinalIgnoreCase);

        agent.LastSeenAtUtc = now;
        agent.LastPublicIp = publicIp;
        agent.LastPrivateIp = internalIp;
        agent.LastPrivateIpv4CandidatesJson = JsonSerializer.Serialize(ipv4Candidates);
        agent.LastPrivateIpv6CandidatesJson = JsonSerializer.Serialize(ipv6Candidates);
        agent.LastSelectedIp = selectedIp;
        agent.LastSelectedInterface = selectedInterface;
        agent.LastHostname = request.Hostname;
        agent.LastAgentVersion = request.Version;
        agent.LastDockerMetadataJson = dockerMetadataJson;
        if (request.Docker is not null)
        {
            agent.InstallType = "docker";
        }

        agent.Status = "online";

        if (ipChanged)
        {
            agent.DnsPendingAtUtc = now;
        }

        db.PulseHeartbeats.Add(new PulseHeartbeatEntity
        {
            PulseAgentId = agent.Id,
            ReceivedAtUtc = now,
            AgentTimestampUtc = request.Timestamp.ToUniversalTime(),
            RemotePublicIp = publicIp,
            Version = request.Version,
            Hostname = request.Hostname,
            PrivateIpv4CandidatesJson = agent.LastPrivateIpv4CandidatesJson,
            PrivateIpv6CandidatesJson = agent.LastPrivateIpv6CandidatesJson,
            SelectedIp = selectedIp,
            SelectedInterface = selectedInterface,
            DockerMetadataJson = dockerMetadataJson,
        });

        await db.SaveChangesAsync(cancellationToken);

        if (ipChanged)
        {
            await targetResolver.RefreshTargetsForPulseAgentAsync(agent.Id, cancellationToken);
            await ApplyDnsForPulseChangeAsync(agent, cancellationToken);
        }

        return PulseHeartbeatAcceptResult.Accepted;
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
        agent.InstallType,
        DeserializeStringList(agent.AllowedScopesJson),
        agent.HeartbeatIntervalSeconds,
        agent.Status,
        agent.LastSeenAtUtc,
        agent.LastPublicIp,
        agent.LastPrivateIp,
        DeserializeStringList(agent.LastPrivateIpv4CandidatesJson),
        DeserializeStringList(agent.LastPrivateIpv6CandidatesJson),
        agent.LastSelectedIp,
        agent.LastSelectedInterface,
        agent.LastHostname,
        agent.LastAgentVersion,
        agent.DnsPendingAtUtc);

    private static string NormalizeInstallType(string? installType)
    {
        var normalized = installType?.Trim().ToLowerInvariant().Replace("-", "_");
        return normalized is "docker" or "linux_service" ? normalized : "linux_service";
    }

    private static IReadOnlyList<string> NormalizeAllowedScopes(IReadOnlyList<string>? scopes)
    {
        var normalized = (scopes is { Count: > 0 } ? scopes : ["heartbeat"])
            .Select(x => x.Trim().ToLowerInvariant())
            .Where(x => x.Length > 0)
            .Distinct(StringComparer.Ordinal)
            .ToList();
        return normalized.Count == 0 ? ["heartbeat"] : normalized;
    }

    private static int NormalizeHeartbeatInterval(int? seconds)
        => Math.Clamp(seconds.GetValueOrDefault(60), 10, 86_400);

    private static IReadOnlyList<string> ValidatePrivateCandidates(IReadOnlyList<string>? candidates, AddressFamily family)
    {
        if (candidates is null)
        {
            return [];
        }

        var result = new List<string>();
        foreach (var candidate in candidates)
        {
            if (!IPAddress.TryParse(candidate.Trim(), out var ip) || ip.AddressFamily != family || !IsPrivateAddress(ip))
            {
                continue;
            }

            var value = ip.ToString();
            if (!result.Contains(value, StringComparer.OrdinalIgnoreCase))
            {
                result.Add(value);
            }
        }

        return result;
    }

    private static string? NormalizeSelectedIp(
        string? selectedIp,
        IReadOnlyList<string> ipv4Candidates,
        IReadOnlyList<string> ipv6Candidates)
    {
        if (string.IsNullOrWhiteSpace(selectedIp) || !IPAddress.TryParse(selectedIp.Trim(), out var ip) || !IsPrivateAddress(ip))
        {
            return null;
        }

        var value = ip.ToString();
        return ipv4Candidates.Contains(value, StringComparer.OrdinalIgnoreCase)
            || ipv6Candidates.Contains(value, StringComparer.OrdinalIgnoreCase)
            ? value
            : null;
    }

    private static string? NormalizeIp(string? ipText)
        => IPAddress.TryParse(ipText, out var ip) ? ip.ToString() : null;

    private static bool IsPrivateAddress(IPAddress ip)
    {
        if (ip.AddressFamily == AddressFamily.InterNetwork)
        {
            var bytes = ip.GetAddressBytes();
            return bytes[0] == 10
                || bytes[0] == 172 && bytes[1] is >= 16 and <= 31
                || bytes[0] == 192 && bytes[1] == 168;
        }

        if (ip.AddressFamily == AddressFamily.InterNetworkV6)
        {
            var bytes = ip.GetAddressBytes();
            return (bytes[0] & 0xfe) == 0xfc;
        }

        return false;
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
