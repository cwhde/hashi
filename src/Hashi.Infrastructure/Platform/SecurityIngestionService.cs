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
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task<SecurityDashboardResponse> GetDashboardAsync(int hours = 24, CancellationToken cancellationToken = default)
    {
        var windowHours = Math.Clamp(hours, 1, 168);
        var since = DateTimeOffset.UtcNow.AddHours(-windowHours);
        var events = await db.AccessLogEvents.AsNoTracking().Where(x => x.ReceivedAtUtc >= since).ToListAsync(cancellationToken);
        var allowed = events.Count(x => x.Decision == "allowed");
        var blocked = events.Count(x => x.Decision == "blocked");
        var challenged = events.Count(x => x.Decision == "challenged");
        var topIps = events.Where(x => x.Decision == "blocked")
            .GroupBy(x => x.ClientIp)
            .OrderByDescending(x => x.Count())
            .Take(10)
            .Select(x => x.Key)
            .ToList();
        var topCountries = events
            .Where(x => !string.IsNullOrWhiteSpace(x.CountryCode))
            .GroupBy(x => x.CountryCode!)
            .OrderByDescending(x => x.Count())
            .Take(10)
            .Select(x => new SecurityRankItem { Label = x.Key, Count = x.LongCount() })
            .ToList();
        var topAsns = events
            .Where(x => !string.IsNullOrWhiteSpace(x.Asn))
            .GroupBy(x => x.Asn!)
            .OrderByDescending(x => x.Count())
            .Take(10)
            .Select(x => new SecurityRankItem { Label = x.Key, Count = x.LongCount() })
            .ToList();
        return new SecurityDashboardResponse(allowed, blocked, challenged, windowHours, topIps, topCountries, topAsns);
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
