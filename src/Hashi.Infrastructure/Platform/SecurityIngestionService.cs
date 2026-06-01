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
    ILogger<SecurityIngestionService> logger,
    TimeProvider? timeProvider = null)
{
    public async Task IngestAccessLogAsync(AccessLogIngestRequest request, CancellationToken cancellationToken = default)
    {
        var now = timeProvider?.GetUtcNow() ?? DateTimeOffset.UtcNow;
        var bucketStartUtc = TruncateToMinuteUtc(now);
        var bucket = await db.AbuseBuckets.SingleOrDefaultAsync(x => x.ClientIp == request.ClientIp, cancellationToken);
        if (bucket is null)
        {
            bucket = new AbuseBucketEntity { ClientIp = request.ClientIp };
            db.AbuseBuckets.Add(bucket);
        }

        bucket.Score += request.StatusCode >= 400 ? 2 : 1;
        bucket.UpdatedAtUtc = now;
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
        requestBucket.UpdatedAtUtc = now;
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

    public Task IngestWafEventAsync(
        WafEventIngestRequest request,
        CancellationToken cancellationToken = default)
        => RecordWafEventAsync(
            request.ClientIp,
            request.Host,
            string.IsNullOrWhiteSpace(request.Path) ? "/" : request.Path,
            NormalizeWafAction(request.Action),
            cancellationToken);

    public async Task<SecurityDashboardResponse> GetDashboardAsync(
        int hours = 24,
        string? resourceFilter = null,
        string? traefikHostFilter = null,
        Guid? firewallHostIdFilter = null,
        CancellationToken cancellationToken = default)
    {
        var windowHours = Math.Clamp(hours, 1, 720);
        var since = DateTimeOffset.UtcNow.AddHours(-windowHours);
        var normalizedResourceFilter = string.IsNullOrWhiteSpace(resourceFilter) ? null : resourceFilter.Trim();
        var normalizedTraefikHostFilter = string.IsNullOrWhiteSpace(traefikHostFilter) ? null : traefikHostFilter.Trim();

        var resourceFilters = await db.Resources.AsNoTracking()
            .Where(x => !string.IsNullOrWhiteSpace(x.Domain))
            .Select(x => x.Domain!)
            .Distinct()
            .OrderBy(x => x)
            .Select(x => new SecurityFilterOption { Value = x, Label = x })
            .ToListAsync(cancellationToken);

        var traefikHostFilters = await db.FirewallHosts.AsNoTracking()
            .Where(x => !string.IsNullOrWhiteSpace(x.LinkedTraefikHost))
            .Select(x => x.LinkedTraefikHost)
            .Distinct()
            .OrderBy(x => x)
            .Select(x => new SecurityFilterOption { Value = x, Label = x })
            .ToListAsync(cancellationToken);

        var firewallHostFilters = await db.FirewallHosts.AsNoTracking()
            .Select(x => new SecurityFirewallHostOption
            {
                Id = x.Id,
                Name = x.Name,
                Domain = x.Domain,
                LinkedTraefikHost = x.LinkedTraefikHost,
            })
            .OrderBy(x => x.Name)
            .ToListAsync(cancellationToken);

        var selectedFirewallHost = firewallHostIdFilter.HasValue
            ? firewallHostFilters.FirstOrDefault(x => x.Id == firewallHostIdFilter.Value)
            : null;

        var hostFilters = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        if (normalizedResourceFilter is not null)
        {
            hostFilters.Add(normalizedResourceFilter);
        }

        if (normalizedTraefikHostFilter is not null)
        {
            hostFilters.Add(normalizedTraefikHostFilter);
        }

        if (selectedFirewallHost is not null)
        {
            if (!string.IsNullOrWhiteSpace(selectedFirewallHost.Domain))
            {
                hostFilters.Add(selectedFirewallHost.Domain);
            }

            if (!string.IsNullOrWhiteSpace(selectedFirewallHost.LinkedTraefikHost))
            {
                hostFilters.Add(selectedFirewallHost.LinkedTraefikHost);
            }
        }

        var accessEventsQuery = db.AccessLogEvents.AsNoTracking()
            .Where(x => x.ReceivedAtUtc >= since);
        var securityEventsQuery = db.SecurityEvents.AsNoTracking()
            .Where(x => x.OccurredAtUtc >= since);

        if (hostFilters.Count > 0)
        {
            accessEventsQuery = accessEventsQuery.Where(x => hostFilters.Contains(x.Host));
            securityEventsQuery = securityEventsQuery.Where(x => x.Host != null && hostFilters.Contains(x.Host));
        }

        var allowed = await accessEventsQuery.Where(x => x.Decision == "allowed").LongCountAsync(cancellationToken);
        var blocked = await accessEventsQuery.Where(x => x.Decision == "blocked").LongCountAsync(cancellationToken);
        var challenged = await accessEventsQuery.Where(x => x.Decision == "challenged").LongCountAsync(cancellationToken);

        var topIps = await accessEventsQuery
            .Where(x => x.Decision == "blocked")
            .GroupBy(x => x.ClientIp)
            .Select(x => new { Ip = x.Key, Count = x.LongCount() })
            .OrderByDescending(x => x.Count)
            .ThenBy(x => x.Ip)
            .Take(10)
            .Select(x => x.Ip)
            .ToListAsync(cancellationToken);

        var topCountries = await accessEventsQuery
            .Where(x => !string.IsNullOrWhiteSpace(x.CountryCode))
            .GroupBy(x => x.CountryCode!)
            .Select(x => new SecurityRankItem { Label = x.Key, Count = x.LongCount() })
            .OrderByDescending(x => x.Count)
            .ThenBy(x => x.Label)
            .Take(10)
            .ToListAsync(cancellationToken);

        var topAsns = await accessEventsQuery
            .Where(x => !string.IsNullOrWhiteSpace(x.Asn))
            .GroupBy(x => x.Asn!)
            .Select(x => new SecurityRankItem { Label = x.Key, Count = x.LongCount() })
            .OrderByDescending(x => x.Count)
            .ThenBy(x => x.Label)
            .Take(10)
            .ToListAsync(cancellationToken);

        var topResourcesRaw = await accessEventsQuery
            .Where(x => x.Decision == "blocked" || x.Decision == "challenged")
            .Where(x => !string.IsNullOrWhiteSpace(x.Host))
            .GroupBy(x => x.Host)
            .Select(x => new
            {
                Resource = x.Key,
                Blocked = x.LongCount(y => y.Decision == "blocked"),
                Challenged = x.LongCount(y => y.Decision == "challenged"),
            })
            .OrderByDescending(x => x.Blocked + x.Challenged)
            .ThenBy(x => x.Resource)
            .Take(10)
            .ToListAsync(cancellationToken);
        var topResources = topResourcesRaw
            .Select(x => new SecurityResourceEnforcementItem
            {
                Resource = x.Resource,
                Blocked = x.Blocked,
                Challenged = x.Challenged,
            })
            .ToList();

        var recentEvents = await securityEventsQuery
            .OrderByDescending(x => x.OccurredAtUtc)
            .Take(20)
            .Select(x => new SecurityRecentEventItem
            {
                OccurredAtUtc = x.OccurredAtUtc,
                Category = x.Category,
                Action = x.Action,
                ClientIp = x.ClientIp,
                Host = x.Host,
                Path = x.Path,
            })
            .ToListAsync(cancellationToken);

        var wafDetections = await securityEventsQuery
            .Where(x => x.Category == "waf")
            .LongCountAsync(cancellationToken);
        var wafBlocks = await securityEventsQuery
            .Where(x => x.Category == "waf")
            .Where(x => x.Action == "blocked" || x.Action == "block" || x.Action == "deny")
            .LongCountAsync(cancellationToken);

        var firewallActiveIpBlocks = await db.BlocklistEntries.AsNoTracking().LongCountAsync(cancellationToken);
        var securityEventCount = await securityEventsQuery.LongCountAsync(cancellationToken);

        return new SecurityDashboardResponse(
            allowed,
            blocked,
            challenged,
            wafDetections,
            wafBlocks,
            windowHours,
            normalizedResourceFilter,
            normalizedTraefikHostFilter,
            selectedFirewallHost?.Id,
            topIps,
            topCountries,
            topAsns,
            topResources,
            recentEvents,
            resourceFilters,
            traefikHostFilters,
            firewallHostFilters,
            firewallActiveIpBlocks,
            firewallActiveIpBlocks,
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

    private static string NormalizeWafAction(string? action)
        => (action ?? string.Empty).Trim().ToLowerInvariant() switch
        {
            "block" or "blocked" or "deny" or "denied" => "blocked",
            "detect" or "detected" or "match" or "matched" => "detected",
            "" => "detected",
            var normalized => normalized,
        };

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
