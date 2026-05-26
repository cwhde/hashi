using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Platform;

public sealed class SecurityIngestionService(
    HashiDbContext db,
    FirewallApplyService firewallApply,
    AuditService audit,
    NotificationRoutingService notificationRouting,
    ILogger<SecurityIngestionService> logger)
{
    public async Task IngestAccessLogAsync(AccessLogIngestRequest request, CancellationToken cancellationToken = default)
    {
        var bucketStartUtc = TruncateToMinuteUtc(DateTimeOffset.UtcNow);
        var bucket = await db.AbuseBuckets.SingleOrDefaultAsync(x => x.ClientIp == request.ClientIp, cancellationToken);
        if (bucket is null)
        {
            bucket = new AbuseBucketEntity { ClientIp = request.ClientIp };
            db.AbuseBuckets.Add(bucket);
        }

        bucket.Score += request.StatusCode >= 400 ? 2 : 1;
        bucket.UpdatedAtUtc = DateTimeOffset.UtcNow;
        bucket.State = bucket.Score switch
        {
            >= 20 => "block",
            >= 10 => "challenge",
            _ => "watch",
        };

        var decision = bucket.State switch
        {
            "block" => "blocked",
            "challenge" => "challenged",
            _ => "allowed",
        };

        var statusClass = request.StatusCode is >= 100 and < 600 ? request.StatusCode / 100 : 0;
        var resource = NormalizeResource(request.Resource, request.Host);
        var traefikInstance = NormalizeTraefikInstance(request.TraefikInstance);
        var method = NormalizeMethod(request.Method);
        var pathPrefix = NormalizePathPrefix(request.PathPrefix, request.Path);
        var requestBucket = await db.SecurityRequestBuckets.SingleOrDefaultAsync(
            x => x.BucketStartUtc == bucketStartUtc
                && x.ClientIp == request.ClientIp
                && x.Resource == resource
                && x.TraefikInstance == traefikInstance
                && x.CountryCode == request.CountryCode
                && x.RegionCode == request.RegionCode
                && x.Asn == request.Asn
                && x.StatusClass == statusClass
                && x.Method == method
                && x.PathPrefix == pathPrefix,
            cancellationToken);
        if (requestBucket is null)
        {
            requestBucket = new SecurityRequestBucketEntity
            {
                BucketStartUtc = bucketStartUtc,
                ClientIp = request.ClientIp,
                Resource = resource,
                TraefikInstance = traefikInstance,
                CountryCode = request.CountryCode,
                RegionCode = request.RegionCode,
                Asn = request.Asn,
                StatusClass = statusClass,
                Method = method,
                PathPrefix = pathPrefix,
            };
            db.SecurityRequestBuckets.Add(requestBucket);
        }

        requestBucket.TotalCount++;
        requestBucket.UpdatedAtUtc = DateTimeOffset.UtcNow;
        switch (decision)
        {
            case "blocked":
                requestBucket.BlockedCount++;
                break;
            case "challenged":
                requestBucket.ChallengedCount++;
                break;
            default:
                requestBucket.AllowedCount++;
                break;
        }

        db.AccessLogEvents.Add(new AccessLogEventEntity
        {
            ClientIp = request.ClientIp,
            Host = request.Host,
            Path = request.Path,
            StatusCode = request.StatusCode,
            CountryCode = request.CountryCode,
            Asn = request.Asn,
            Decision = decision,
        });

        db.SecurityEvents.Add(new SecurityEventEntity
        {
            Category = "access",
            Action = decision,
            ClientIp = request.ClientIp,
            Host = request.Host,
            Path = request.Path,
        });

        var createdBlocklistEntry = false;
        if (bucket.State == "block")
        {
            var exists = await db.BlocklistEntries.AnyAsync(x => x.ClientIp == request.ClientIp, cancellationToken);
            if (!exists)
            {
                db.BlocklistEntries.Add(new BlocklistEntryEntity
                {
                    ClientIp = request.ClientIp,
                    Reason = "abuse_score_threshold",
                });
                createdBlocklistEntry = true;
            }
        }

        await db.SaveChangesAsync(cancellationToken);

        if (createdBlocklistEntry)
        {
            await TrySyncBlocklistToAllFirewallsAsync(cancellationToken);
        }

        if (bucket.State is "block" or "challenge")
        {
            await notificationRouting.RouteSecurityEventAsync(
                $"Security {bucket.State}: {request.ClientIp}",
                $"{request.Host}{request.Path} from {request.ClientIp} scored {bucket.Score}.",
                cancellationToken);
        }
    }

    public async Task IngestForwardAuthDecisionAsync(
        ForwardAuthDecisionIngestRequest request,
        CancellationToken cancellationToken = default)
    {
        var decision = request.Decision.ToLowerInvariant() switch
        {
            "deny" => "blocked",
            "challenge" or "redirect" => "challenged",
            _ => "allowed",
        };
        var statusCode = request.Decision.ToLowerInvariant() switch
        {
            "deny" => 403,
            "challenge" or "redirect" => 401,
            _ => 204,
        };

        db.AccessLogEvents.Add(new AccessLogEventEntity
        {
            ClientIp = request.ClientIp,
            Host = request.Host,
            Path = request.Path,
            StatusCode = statusCode,
            CountryCode = request.CountryCode,
            Asn = request.Asn,
            Decision = decision,
        });

        db.SecurityEvents.Add(new SecurityEventEntity
        {
            Category = "forward_auth",
            Action = request.Decision,
            ClientIp = request.ClientIp,
            Host = request.Host,
            Path = request.Path,
        });
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task RecordWafEventAsync(
        string clientIp,
        string host,
        string path,
        string action,
        CancellationToken cancellationToken = default)
    {
        db.SecurityEvents.Add(new SecurityEventEntity
        {
            Category = "waf",
            Action = action,
            ClientIp = clientIp,
            Host = host,
            Path = path,
        });
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task<SecurityDashboardResponse> GetDashboardAsync(int hours = 24, CancellationToken cancellationToken = default)
    {
        var windowHours = Math.Clamp(hours, 1, 168);
        var since = DateTimeOffset.UtcNow.AddHours(-windowHours);
        var bucketSince = TruncateToMinuteUtc(since);
        var bucketWindow = db.SecurityRequestBuckets.AsNoTracking()
            .Where(x => x.BucketStartUtc >= bucketSince);
        var totals = await bucketWindow
            .GroupBy(_ => 1)
            .Select(group => new
            {
                Allowed = group.Sum(x => x.AllowedCount),
                Blocked = group.Sum(x => x.BlockedCount),
                Challenged = group.Sum(x => x.ChallengedCount),
            })
            .SingleOrDefaultAsync(cancellationToken);
        var allowed = totals?.Allowed ?? 0;
        var blocked = totals?.Blocked ?? 0;
        var challenged = totals?.Challenged ?? 0;
        var topIps = await bucketWindow
            .Where(x => x.BlockedCount > 0)
            .GroupBy(x => x.ClientIp)
            .OrderByDescending(x => x.Sum(y => y.BlockedCount))
            .Take(10)
            .Select(x => x.Key)
            .ToListAsync(cancellationToken);
        var topCountries = await bucketWindow
            .Where(x => !string.IsNullOrWhiteSpace(x.CountryCode))
            .GroupBy(x => x.CountryCode!)
            .OrderByDescending(x => x.Sum(y => y.TotalCount))
            .Take(10)
            .Select(x => new SecurityRankItem { Label = x.Key, Count = x.Sum(y => y.TotalCount) })
            .ToListAsync(cancellationToken);
        var topAsns = await bucketWindow
            .Where(x => !string.IsNullOrWhiteSpace(x.Asn))
            .GroupBy(x => x.Asn!)
            .OrderByDescending(x => x.Sum(y => y.TotalCount))
            .Take(10)
            .Select(x => new SecurityRankItem { Label = x.Key, Count = x.Sum(y => y.TotalCount) })
            .ToListAsync(cancellationToken);
        var blocklistCount = await db.BlocklistEntries.AsNoTracking().LongCountAsync(cancellationToken);
        var securityEventCount = await db.SecurityEvents.AsNoTracking()
            .Where(x => x.OccurredAtUtc >= since)
            .LongCountAsync(cancellationToken);
        return new SecurityDashboardResponse(
            allowed,
            blocked,
            challenged,
            windowHours,
            topIps,
            topCountries,
            topAsns,
            blocklistCount,
            securityEventCount);
    }

    public async Task<BlocklistSyncResponse> SyncBlocklistToAllFirewallsAsync(CancellationToken cancellationToken = default)
    {
        var pending = await db.BlocklistEntries.Where(x => !x.SyncedToFirewall).ToListAsync(cancellationToken);
        if (pending.Count == 0)
        {
            return new BlocklistSyncResponse(true, 0, 0, []);
        }

        var hosts = await db.FirewallHosts.ToListAsync(cancellationToken);
        if (hosts.Count == 0)
        {
            return new BlocklistSyncResponse(false, pending.Count, 0, ["No firewall hosts configured."]);
        }

        var failures = new List<string>();
        var appliedHosts = 0;
        foreach (var host in hosts)
        {
            try
            {
                var result = await firewallApply.ApplyForHostAsync(host.Id, cancellationToken);
                if (!result.Succeeded)
                {
                    failures.Add($"{host.Name}: {result.Message}");
                    continue;
                }

                appliedHosts++;
            }
            catch (Exception ex)
            {
                failures.Add($"{host.Name}: {ex.Message}");
            }
        }

        if (failures.Count > 0)
        {
            logger.LogWarning("Blocklist sync failed on {Count} firewall host(s): {Errors}", failures.Count, string.Join("; ", failures));
            return new BlocklistSyncResponse(false, pending.Count, appliedHosts, failures);
        }

        foreach (var entry in pending)
        {
            entry.SyncedToFirewall = true;
        }

        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync(
            "security",
            "blocklist_synced",
            metadata: new { hosts = appliedHosts, entries = pending.Count },
            cancellationToken: cancellationToken);
        return new BlocklistSyncResponse(true, pending.Count, appliedHosts, []);
    }

    private static DateTimeOffset TruncateToMinuteUtc(DateTimeOffset value)
    {
        var utc = value.ToUniversalTime();
        return new DateTimeOffset(utc.Year, utc.Month, utc.Day, utc.Hour, utc.Minute, 0, TimeSpan.Zero);
    }

    private static string NormalizeResource(string? resource, string host)
        => string.IsNullOrWhiteSpace(resource) ? host : resource.Trim();

    private static string NormalizeTraefikInstance(string? traefikInstance)
        => string.IsNullOrWhiteSpace(traefikInstance) ? "default" : traefikInstance.Trim();

    private static string NormalizeMethod(string? method)
        => string.IsNullOrWhiteSpace(method) ? "GET" : method.Trim().ToUpperInvariant();

    private static string NormalizePathPrefix(string? pathPrefix, string path)
    {
        if (!string.IsNullOrWhiteSpace(pathPrefix))
        {
            return EnsureLeadingSlash(pathPrefix.Trim());
        }

        if (string.IsNullOrWhiteSpace(path))
        {
            return "/";
        }

        var normalizedPath = EnsureLeadingSlash(path.Trim());
        var slash = normalizedPath.IndexOf('/', 1);
        return slash < 0 ? normalizedPath : normalizedPath[..slash];
    }

    private static string EnsureLeadingSlash(string value)
        => value.StartsWith('/') ? value : "/" + value;

    private async Task TrySyncBlocklistToAllFirewallsAsync(CancellationToken cancellationToken)
    {
        try
        {
            await SyncBlocklistToAllFirewallsAsync(cancellationToken);
        }
        catch (Exception ex)
        {
            logger.LogWarning(ex, "Automatic blocklist sync failed after abuse block.");
        }
    }
}
