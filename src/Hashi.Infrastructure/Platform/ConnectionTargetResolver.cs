using System.Net;
using System.Net.Sockets;
using System.Text.Json;
using System.Text.Json.Nodes;
using Hashi.Core.Sync;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed record ResolvedConnectionTarget(
    Guid TargetId,
    string OwnerType,
    Guid OwnerId,
    string TargetMode,
    string Status,
    string? Error,
    string ResolvedHost,
    string? ResolvedIp,
    Uri BaseUri,
    bool IsStale);

public sealed class ConnectionTargetResolver(HashiDbContext db, AuditService audit)
{
    public static readonly TimeSpan MinimumStaleAfter = TimeSpan.FromMinutes(2);

    public static void ValidateTarget(ConnectionTargetEntity target)
    {
        var mode = (target.TargetMode ?? string.Empty).Trim().ToLowerInvariant();
        switch (mode)
        {
            case ConnectionTargetModeNames.PulseAgent:
                if (target.PulseAgentId is null || target.PulseAgentId == Guid.Empty)
                {
                    throw new InvalidOperationException("Pulse agent mode requires a valid PulseAgentId.");
                }
                break;
            case ConnectionTargetModeNames.StaticIp:
                if (string.IsNullOrWhiteSpace(target.StaticIp))
                {
                    throw new InvalidOperationException("Static IP mode requires a valid StaticIp.");
                }
                break;
            case ConnectionTargetModeNames.StaticHost:
                if (string.IsNullOrWhiteSpace(target.StaticHost))
                {
                    throw new InvalidOperationException("Static host mode requires a valid StaticHost.");
                }
                break;
        }

        if (target.Port <= 0 || target.Port > 65535)
        {
            throw new InvalidOperationException($"Port must be between 1 and 65535, got {target.Port}.");
        }
    }

    public async Task<ResolvedConnectionTarget> ResolveAsync(
        ConnectionTargetEntity target,
        bool persistSnapshot = true,
        CancellationToken cancellationToken = default)
    {
        var resolved = await ResolveCoreAsync(target, cancellationToken);
        if (persistSnapshot)
        {
            await PersistResolutionAsync(target, resolved, cancellationToken);
        }

        return resolved;
    }

    public async Task<IReadOnlyList<ResolvedConnectionTarget>> RefreshTargetsForPulseAgentAsync(
        Guid pulseAgentId,
        CancellationToken cancellationToken = default)
    {
        var targets = await db.ConnectionTargets
            .Where(x => x.PulseAgentId == pulseAgentId && x.TargetMode == ConnectionTargetModeNames.PulseAgent)
            .ToListAsync(cancellationToken);
        var results = new List<ResolvedConnectionTarget>();
        foreach (var target in targets)
        {
            var before = target.ResolvedIpSnapshot;
            var resolved = await ResolveAsync(target, persistSnapshot: true, cancellationToken);
            results.Add(resolved);
            if (!string.Equals(before, resolved.ResolvedIp, StringComparison.OrdinalIgnoreCase))
            {
                await RecordDependencyImpactAsync(target, before, resolved, cancellationToken);
            }
        }

        return results;
    }

    public async Task<ConnectionTargetEntity> GetOrCreateAdGuardTargetAsync(
        AdGuardConnectionEntity connection,
        CancellationToken cancellationToken = default)
    {
        var target = await db.ConnectionTargets.SingleOrDefaultAsync(
            x => x.OwnerType == ConnectionTargetOwnerTypeNames.AdGuardConnection && x.OwnerId == connection.Id,
            cancellationToken);
        if (target is not null)
        {
            return target;
        }

        target = FromAdGuardBaseUrl(connection);
        db.ConnectionTargets.Add(target);
        await db.SaveChangesAsync(cancellationToken);
        return target;
    }

    public async Task<ResolvedConnectionTarget> ResolveAdGuardAsync(
        AdGuardConnectionEntity connection,
        bool persistSnapshot = true,
        CancellationToken cancellationToken = default)
    {
        var target = await GetOrCreateAdGuardTargetAsync(connection, cancellationToken);
        return await ResolveAsync(target, persistSnapshot, cancellationToken);
    }

    public async Task<ResolvedConnectionTarget?> ResolveConnectionAsync(
        ConnectionEntity connection,
        bool persistSnapshot = true,
        CancellationToken cancellationToken = default)
    {
        var target = await db.ConnectionTargets.SingleOrDefaultAsync(
            x => x.OwnerType == ConnectionTargetOwnerTypeNames.Connection && x.OwnerId == connection.Id,
            cancellationToken);
        return target is null
            ? null
            : await ResolveAsync(target, persistSnapshot, cancellationToken);
    }

    public static Uri BuildUri(ConnectionTargetEntity target, string host)
    {
        var scheme = NormalizeScheme(target.Scheme);
        var builder = new UriBuilder(scheme, host, NormalizePort(target.Port, scheme));
        var path = NormalizePathPrefix(target.PathPrefix);
        if (!string.IsNullOrEmpty(path))
        {
            builder.Path = path.TrimStart('/');
        }

        return builder.Uri;
    }

    public static ConnectionTargetEntity FromAdGuardBaseUrl(AdGuardConnectionEntity connection)
    {
        var baseUri = Uri.TryCreate(connection.BaseUrl.TrimEnd('/') + "/", UriKind.Absolute, out var parsed)
            ? parsed
            : new Uri("http://127.0.0.1/");
        var host = baseUri.Host;
        var isIp = IPAddress.TryParse(host, out _);
        return new ConnectionTargetEntity
        {
            OwnerType = ConnectionTargetOwnerTypeNames.AdGuardConnection,
            OwnerId = connection.Id,
            TargetMode = isIp ? ConnectionTargetModeNames.StaticIp : ConnectionTargetModeNames.StaticHost,
            StaticHost = isIp ? null : host,
            StaticIp = isIp ? host : null,
            Scheme = NormalizeScheme(baseUri.Scheme),
            Port = baseUri.IsDefaultPort ? DefaultPort(baseUri.Scheme) : baseUri.Port,
            PathPrefix = NormalizePathPrefix(baseUri.AbsolutePath),
            Status = ConnectionTargetStatusNames.Unresolved,
        };
    }

    public static string ToBaseUrl(ConnectionTargetEntity target, string host)
        => BuildUri(target, host).ToString().TrimEnd('/');

    private async Task<ResolvedConnectionTarget> ResolveCoreAsync(
        ConnectionTargetEntity target,
        CancellationToken cancellationToken)
    {
        return NormalizeTargetMode(target.TargetMode) switch
        {
            ConnectionTargetModeNames.StaticHost => ResolveStaticHost(target),
            ConnectionTargetModeNames.StaticIp => ResolveStaticIp(target),
            ConnectionTargetModeNames.PulseAgent => await ResolvePulseAgentAsync(target, cancellationToken),
            _ => Failure(target, "Unknown target mode."),
        };
    }

    private static ResolvedConnectionTarget ResolveStaticHost(ConnectionTargetEntity target)
    {
        var host = target.StaticHost?.Trim();
        if (string.IsNullOrWhiteSpace(host))
        {
            return Failure(target, "Static host is required.");
        }

        return Success(target, host, host, stale: false);
    }

    private static ResolvedConnectionTarget ResolveStaticIp(ConnectionTargetEntity target)
    {
        if (!IPAddress.TryParse(target.StaticIp?.Trim(), out var ip))
        {
            return Failure(target, "Static IP is invalid.");
        }

        var value = ip.ToString();
        return Success(target, value, value, stale: false);
    }

    private async Task<ResolvedConnectionTarget> ResolvePulseAgentAsync(
        ConnectionTargetEntity target,
        CancellationToken cancellationToken)
    {
        if (target.PulseAgentId is not Guid agentId)
        {
            return Failure(target, "Pulse agent is required.");
        }

        var agent = await db.PulseAgents.AsNoTracking().SingleOrDefaultAsync(x => x.Id == agentId, cancellationToken);
        if (agent is null || string.Equals(agent.Status, "revoked", StringComparison.OrdinalIgnoreCase))
        {
            return Failure(target, "Pulse agent is missing or revoked.");
        }

        var ip = SelectPulseIp(target, agent);
        if (string.IsNullOrWhiteSpace(ip))
        {
            return Failure(target, "Pulse agent has no IP for the selected mode.");
        }

        var staleAfter = TimeSpan.FromSeconds(Math.Max(agent.HeartbeatIntervalSeconds * 3, (int)MinimumStaleAfter.TotalSeconds));
        var isStale = agent.LastSeenAtUtc is null || DateTimeOffset.UtcNow - agent.LastSeenAtUtc.Value > staleAfter;
        var status = isStale ? ConnectionTargetStatusNames.Stale : ConnectionTargetStatusNames.Resolved;
        var error = isStale ? "Pulse agent heartbeat is stale; using last known target." : null;
        return new ResolvedConnectionTarget(
            target.Id,
            target.OwnerType,
            target.OwnerId,
            ConnectionTargetModeNames.PulseAgent,
            status,
            error,
            ip,
            ip,
            BuildUri(target, ip),
            isStale);
    }

    private static string? SelectPulseIp(ConnectionTargetEntity target, PulseAgentEntity agent)
    {
        var mode = NormalizePulseIpMode(target.PulseIpMode);
        return mode switch
        {
            PulseTargetIpModeNames.Selected => FirstNonEmpty(agent.LastSelectedIp, agent.LastPrivateIp, agent.LastPublicIp),
            PulseTargetIpModeNames.Public => agent.LastPublicIp,
            PulseTargetIpModeNames.PrivateSelected => FirstNonEmpty(agent.LastSelectedIp, agent.LastPrivateIp),
            PulseTargetIpModeNames.PrivateCandidate => SelectPrivateCandidate(target.PrivateCandidateSelector, agent),
            _ => null,
        };
    }

    private static string? SelectPrivateCandidate(string? selector, PulseAgentEntity agent)
    {
        var normalized = selector?.Trim();
        if (string.IsNullOrWhiteSpace(normalized) || string.Equals(normalized, PulsePrivateCandidateSelectorNames.Selected, StringComparison.OrdinalIgnoreCase))
        {
            return FirstNonEmpty(agent.LastSelectedIp, agent.LastPrivateIp);
        }

        var ipv4 = DeserializeStringList(agent.LastPrivateIpv4CandidatesJson);
        var ipv6 = DeserializeStringList(agent.LastPrivateIpv6CandidatesJson);
        var all = ipv4.Concat(ipv6).ToList();
        var lower = normalized.ToLowerInvariant();
        if (lower == PulsePrivateCandidateSelectorNames.FirstIpv4)
        {
            return ipv4.FirstOrDefault();
        }

        if (lower == PulsePrivateCandidateSelectorNames.FirstIpv6)
        {
            return ipv6.FirstOrDefault();
        }

        if (lower.StartsWith("interface=", StringComparison.Ordinal))
        {
            var interfaceName = normalized["interface=".Length..].Trim();
            return string.Equals(interfaceName, agent.LastSelectedInterface, StringComparison.OrdinalIgnoreCase)
                ? FirstNonEmpty(agent.LastSelectedIp, agent.LastPrivateIp)
                : null;
        }

        if (lower.StartsWith("cidr=", StringComparison.Ordinal))
        {
            return SelectByCidr(all, normalized["cidr=".Length..].Trim());
        }

        var address = lower.StartsWith("address=", StringComparison.Ordinal)
            ? normalized["address=".Length..].Trim()
            : lower.StartsWith("ip=", StringComparison.Ordinal)
                ? normalized["ip=".Length..].Trim()
                : normalized;
        return IPAddress.TryParse(address, out var parsed)
            ? all.FirstOrDefault(x => IPAddress.TryParse(x, out var candidate) && candidate.Equals(parsed))
            : null;
    }

    private static string? SelectByCidr(IReadOnlyList<string> candidates, string cidr)
    {
        var parts = cidr.Split('/', 2, StringSplitOptions.TrimEntries);
        if (parts.Length != 2 || !IPAddress.TryParse(parts[0], out var network) || !int.TryParse(parts[1], out var prefix))
        {
            return null;
        }

        return candidates.FirstOrDefault(x =>
            IPAddress.TryParse(x, out var candidate) && Contains(network, prefix, candidate));
    }

    private static bool Contains(IPAddress network, int prefixLength, IPAddress candidate)
    {
        if (network.AddressFamily != candidate.AddressFamily)
        {
            return false;
        }

        var maxPrefix = network.AddressFamily == AddressFamily.InterNetwork ? 32 : 128;
        if (prefixLength < 0 || prefixLength > maxPrefix)
        {
            return false;
        }

        var networkBytes = network.GetAddressBytes();
        var candidateBytes = candidate.GetAddressBytes();
        var wholeBytes = prefixLength / 8;
        var remainingBits = prefixLength % 8;
        for (var i = 0; i < wholeBytes; i++)
        {
            if (networkBytes[i] != candidateBytes[i])
            {
                return false;
            }
        }

        if (remainingBits == 0)
        {
            return true;
        }

        var mask = (byte)(0xff << (8 - remainingBits));
        return (networkBytes[wholeBytes] & mask) == (candidateBytes[wholeBytes] & mask);
    }

    private async Task PersistResolutionAsync(
        ConnectionTargetEntity target,
        ResolvedConnectionTarget resolved,
        CancellationToken cancellationToken)
    {
        var tracked = db.Entry(target).State == EntityState.Detached
            ? await db.ConnectionTargets.SingleOrDefaultAsync(x => x.Id == target.Id, cancellationToken)
            : target;
        if (tracked is null)
        {
            return;
        }

        tracked.TargetMode = NormalizeTargetMode(tracked.TargetMode);
        tracked.PulseIpMode = NormalizePulseIpMode(tracked.PulseIpMode);
        tracked.Scheme = NormalizeScheme(tracked.Scheme);
        tracked.Port = NormalizePort(tracked.Port, tracked.Scheme);
        tracked.PathPrefix = NormalizePathPrefix(tracked.PathPrefix);
        tracked.ResolvedIpSnapshot = resolved.ResolvedIp;
        tracked.LastResolvedAtUtc = DateTimeOffset.UtcNow;
        tracked.Status = resolved.Status;
        tracked.LastError = resolved.Error;
        tracked.UpdatedAtUtc = DateTimeOffset.UtcNow;
        if (tracked.OwnerType == ConnectionTargetOwnerTypeNames.Connection &&
            resolved.Status != ConnectionTargetStatusNames.Failed)
        {
            var connection = await db.Connections.SingleOrDefaultAsync(x => x.Id == tracked.OwnerId, cancellationToken);
            if (connection is not null)
            {
                connection.SettingsJson = ApplyResolvedSshTarget(
                    connection.SettingsJson,
                    resolved.ResolvedHost,
                    tracked.Port);
                connection.UpdatedAtUtc = DateTimeOffset.UtcNow;
            }
        }

        await db.SaveChangesAsync(cancellationToken);
    }

    private async Task RecordDependencyImpactAsync(
        ConnectionTargetEntity target,
        string? previousSnapshot,
        ResolvedConnectionTarget resolved,
        CancellationToken cancellationToken)
    {
        db.SyncRuns.Add(new SyncRunEntity
        {
            Subsystem = target.OwnerType == ConnectionTargetOwnerTypeNames.AdGuardConnection ? "adguard-pulse-target" : "connection-target",
            Status = SyncRunStatusNames.Pending,
            RiskLevel = nameof(SyncRiskLevel.Low),
            ErrorSummary = $"Pulse agent target changed for {target.OwnerType} {target.OwnerId}.",
        });

        var genericConnection = await db.Connections.SingleOrDefaultAsync(x => x.Id == target.OwnerId, cancellationToken);
        if (genericConnection is not null)
        {
            genericConnection.HealthState = ConnectionHealthStateNames.Unknown;
            genericConnection.LastValidationMessage = "Pulse target changed; validation pending.";
            genericConnection.UpdatedAtUtc = DateTimeOffset.UtcNow;
            db.ConnectionHealth.Add(new ConnectionHealthEntity
            {
                ConnectionId = genericConnection.Id,
                State = ConnectionHealthStateNames.Unknown,
                CheckKind = "pulse_target_change",
                Message = "Pulse target changed; validation pending.",
                DetailsJson = JsonSerializer.Serialize(new { previousSnapshot, resolved.ResolvedIp, resolved.Status }),
            });
        }

        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync(
            "connection_target",
            "agent_bound_target_changed",
            subjectType: target.OwnerType,
            subjectId: target.OwnerId.ToString(),
            metadata: new
            {
                target.Id,
                target.PulseAgentId,
                previousSnapshot,
                resolvedSnapshot = resolved.ResolvedIp,
                resolved.Status,
                resolved.IsStale,
            },
            cancellationToken: cancellationToken);
    }

    private static ResolvedConnectionTarget Success(ConnectionTargetEntity target, string host, string? resolvedIp, bool stale)
        => new(
            target.Id,
            target.OwnerType,
            target.OwnerId,
            NormalizeTargetMode(target.TargetMode),
            stale ? ConnectionTargetStatusNames.Stale : ConnectionTargetStatusNames.Resolved,
            stale ? "Using last known target." : null,
            host,
            resolvedIp,
            BuildUri(target, host),
            stale);

    private static ResolvedConnectionTarget Failure(ConnectionTargetEntity target, string error)
        => new(
            target.Id,
            target.OwnerType,
            target.OwnerId,
            NormalizeTargetMode(target.TargetMode),
            ConnectionTargetStatusNames.Failed,
            error,
            "127.0.0.1",
            null,
            BuildUri(target, "127.0.0.1"),
            false);

    private static string NormalizeTargetMode(string? mode)
        => (mode ?? string.Empty).Trim().ToLowerInvariant() switch
        {
            ConnectionTargetModeNames.StaticIp => ConnectionTargetModeNames.StaticIp,
            ConnectionTargetModeNames.PulseAgent => ConnectionTargetModeNames.PulseAgent,
            _ => ConnectionTargetModeNames.StaticHost,
        };

    private static string NormalizePulseIpMode(string? mode)
        => (mode ?? string.Empty).Trim().ToLowerInvariant() switch
        {
            PulseTargetIpModeNames.Public => PulseTargetIpModeNames.Public,
            PulseTargetIpModeNames.Private => PulseTargetIpModeNames.PrivateSelected,
            PulseTargetIpModeNames.PrivateSelected => PulseTargetIpModeNames.PrivateSelected,
            PulseTargetIpModeNames.PrivateCandidate => PulseTargetIpModeNames.PrivateCandidate,
            _ => PulseTargetIpModeNames.Selected,
        };

    private static string NormalizeScheme(string? scheme)
        => string.Equals(scheme?.Trim(), "https", StringComparison.OrdinalIgnoreCase) ? "https" : "http";

    private static int NormalizePort(int port, string scheme)
        => port is >= 1 and <= 65535 ? port : DefaultPort(scheme);

    private static int DefaultPort(string? scheme)
        => string.Equals(scheme, "https", StringComparison.OrdinalIgnoreCase) ? 443 : 80;

    private static string? NormalizePathPrefix(string? path)
    {
        if (string.IsNullOrWhiteSpace(path) || path.Trim() == "/")
        {
            return null;
        }

        return "/" + path.Trim().Trim('/');
    }

    private static string? FirstNonEmpty(params string?[] values)
        => values.FirstOrDefault(x => !string.IsNullOrWhiteSpace(x))?.Trim();

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

    private static string ApplyResolvedSshTarget(string settingsJson, string host, int port)
    {
        JsonObject settings;
        try
        {
            settings = JsonNode.Parse(settingsJson) as JsonObject ?? new JsonObject();
        }
        catch (JsonException)
        {
            settings = new JsonObject();
        }

        settings["Host"] = host;
        settings["Port"] = port;
        return settings.ToJsonString();
    }
}
