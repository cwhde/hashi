using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class SecurityIngestionService(
    HashiDbContext db,
    FirewallApplyService firewallApply,
    AuditService audit,
    NotificationRoutingService notificationRouting)
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
            }
        }

        await db.SaveChangesAsync(cancellationToken);

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
            .Take(5)
            .Select(x => x.Key)
            .ToList();
        var topCountries = events
            .Where(x => !string.IsNullOrWhiteSpace(x.CountryCode))
            .GroupBy(x => x.CountryCode!)
            .OrderByDescending(x => x.Count())
            .Take(5)
            .Select(x => new SecurityRankItem { Label = x.Key, Count = x.LongCount() })
            .ToList();
        var topAsns = events
            .Where(x => !string.IsNullOrWhiteSpace(x.Asn))
            .GroupBy(x => x.Asn!)
            .OrderByDescending(x => x.Count())
            .Take(5)
            .Select(x => new SecurityRankItem { Label = x.Key, Count = x.LongCount() })
            .ToList();
        return new SecurityDashboardResponse(allowed, blocked, challenged, windowHours, topIps, topCountries, topAsns);
    }

    public async Task SyncBlocklistToFirewallAsync(FirewallApplyRequest request, CancellationToken cancellationToken = default)
    {
        var pending = await db.BlocklistEntries.Where(x => !x.SyncedToFirewall).ToListAsync(cancellationToken);
        if (pending.Count == 0)
        {
            return;
        }

        foreach (var entry in pending)
        {
            entry.SyncedToFirewall = true;
        }

        await db.SaveChangesAsync(cancellationToken);
        await firewallApply.ApplyAsync(request, cancellationToken);
        await audit.WriteAsync("security", "blocklist_synced", cancellationToken: cancellationToken);
    }
}
