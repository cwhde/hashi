using System.Net;
using System.Security.Cryptography;
using System.Text;
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
    TimeProvider? timeProvider = null,
    SecuritySubjectService? subjectService = null)
{
    public async Task IngestAccessLogAsync(AccessLogIngestRequest request, CancellationToken cancellationToken = default)
    {
        var now = timeProvider?.GetUtcNow() ?? DateTimeOffset.UtcNow;
        var bucketStartUtc = TruncateToMinuteUtc(now);
        var normalizedSubject = SecuritySubjectNormalizer.TryNormalize(SecuritySubjectTypeNames.Ip, request.ClientIp, out var parsedSubject)
            ? parsedSubject
            : new NormalizedSecuritySubject(SecuritySubjectTypeNames.Ip, request.ClientIp, request.ClientIp);
        SecuritySubjectEntity? securitySubject = null;
        SecuritySubjectStateEntity? securitySubjectState = null;
        if (IPAddress.TryParse(request.ClientIp, out var parsedIp))
        {
            (securitySubject, securitySubjectState) = await (subjectService ?? new SecuritySubjectService(db, timeProvider))
                .ResolveOrCreateIpAsync(parsedIp, request.CountryCode, request.RegionCode, request.Asn, cancellationToken);
        }

        var bucket = await db.AbuseBuckets.SingleOrDefaultAsync(x => x.ClientIp == request.ClientIp, cancellationToken);
        if (bucket is null)
        {
            bucket = new AbuseBucketEntity { ClientIp = request.ClientIp };
            db.AbuseBuckets.Add(bucket);
        }

        var manualBlock = await FindActiveBlocklistEntryAsync(
            request.ClientIp,
            BlocklistSourceNames.Manual,
            cancellationToken);
        var manualAllow = manualBlock is null
            && await IsManuallyAllowedAsync(request.ClientIp, request.CountryCode, request.RegionCode, request.Asn, cancellationToken);

        if (manualBlock is not null)
        {
            bucket.State = SecuritySubjectStateNames.ManuallyBlocked;
        }
        else if (manualAllow)
        {
            bucket.State = SecuritySubjectStateNames.ManuallyAllowed;
        }
        else
        {
            bucket.Score += request.StatusCode >= 400 ? 2 : 1;
            var activeFirewallBlock = await FindActiveBlocklistEntryAsync(
                request.ClientIp,
                BlocklistSourceNames.Automatic,
                cancellationToken);
            bucket.State = activeFirewallBlock?.SyncedToFirewall == true
                ? SecuritySubjectStateNames.FirewallBlocked
                : StateForScore(bucket.Score);
        }

        bucket.UpdatedAtUtc = now;
        if (securitySubject is not null && securitySubjectState is not null)
        {
            securitySubject.CurrentState = SecuritySubjectStateNames.Normalize(bucket.State);
            securitySubjectState.ManualAllowActive = manualAllow;
            securitySubjectState.ManualBlockActive = manualBlock is not null;
            securitySubjectState.ChallengeRequired = securitySubject.CurrentState == SecuritySubjectStateNames.Challenged;
            securitySubjectState.ChallengeReason = securitySubjectState.ChallengeRequired ? "abuse_score_threshold" : securitySubjectState.ChallengeReason;
            securitySubjectState.UpdatedAtUtc = now;
        }

        var normalizedState = SecuritySubjectStateNames.Normalize(bucket.State);
        var decision = DecisionForState(normalizedState);
        var requestId = NormalizeCorrelationId(request.RequestId);
        var userAgentHash = ResolveUserAgentHash(request.UserAgent, request.UserAgentHash);

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
                SubjectType = normalizedSubject.SubjectType,
                NormalizedSubjectValue = normalizedSubject.NormalizedValue,
                RegionCode = request.RegionCode,
                Asn = request.Asn,
                StatusClass = statusClass,
                Method = method,
                PathPrefix = pathPrefix,
            };
            db.SecurityRequestBuckets.Add(requestBucket);
        }

        requestBucket.TotalCount++;
        requestBucket.RequestCount++;
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
            SubjectType = normalizedSubject.SubjectType,
            SubjectValue = normalizedSubject.SubjectValue,
            NormalizedSubjectValue = normalizedSubject.NormalizedValue,
            EventType = "access_log",
            Decision = decision,
            Source = "traefik_access_log",
            RequestMethod = method,
            RequestPath = request.Path,
            StatusCode = request.StatusCode,
            RequestId = requestId,
            UserAgentHash = userAgentHash,
        });

        var shouldSyncFirewallBlock = false;
        if (manualBlock is null && !manualAllow && bucket.Score >= 20)
        {
            var exists = await db.BlocklistEntries.AnyAsync(
                x => x.Scope == BlocklistScopeNames.Global
                    && (x.Type == BlocklistTypeNames.Ip || x.Type == string.Empty)
                    && (x.Value == request.ClientIp || x.ClientIp == request.ClientIp)
                    && (x.ExpiresAtUtc == null || x.ExpiresAtUtc > now),
                cancellationToken);
            if (!exists)
            {
                db.BlocklistEntries.Add(new BlocklistEntryEntity
                {
                    ClientIp = request.ClientIp,
                    Scope = BlocklistScopeNames.Global,
                    Type = BlocklistTypeNames.Ip,
                    Value = request.ClientIp,
                    Reason = "abuse_score_threshold",
                    Source = BlocklistSourceNames.Automatic,
                    CreatedBy = "hashi",
                    CreatedAtUtc = now,
                });
            }

            shouldSyncFirewallBlock = true;
            if (normalizedState != SecuritySubjectStateNames.FirewallBlocked)
            {
                bucket.State = SecuritySubjectStateNames.SoftBlocked;
            }
        }

        await db.SaveChangesAsync(cancellationToken);

        if (shouldSyncFirewallBlock)
        {
            var syncResult = await TrySyncBlocklistToAllFirewallsAsync(cancellationToken);
            if (syncResult?.Synced == true)
            {
                var syncedBucket = await db.AbuseBuckets.SingleAsync(x => x.ClientIp == request.ClientIp, cancellationToken);
                if (SecuritySubjectStateNames.Normalize(syncedBucket.State) == SecuritySubjectStateNames.SoftBlocked)
                {
                    syncedBucket.State = SecuritySubjectStateNames.FirewallBlocked;
                    syncedBucket.UpdatedAtUtc = timeProvider?.GetUtcNow() ?? DateTimeOffset.UtcNow;
                    await db.SaveChangesAsync(cancellationToken);
                }
            }
        }

        var notificationState = SecuritySubjectStateNames.Normalize(bucket.State);
        if (notificationState is SecuritySubjectStateNames.FirewallBlocked
            or SecuritySubjectStateNames.SoftBlocked
            or SecuritySubjectStateNames.Challenged
            or SecuritySubjectStateNames.ManuallyBlocked)
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
        var now = timeProvider?.GetUtcNow() ?? DateTimeOffset.UtcNow;
        var bucketStartUtc = TruncateToMinuteUtc(now);
        var normalizedSubject = SecuritySubjectNormalizer.TryNormalize(SecuritySubjectTypeNames.Ip, request.ClientIp, out var parsedSubject)
            ? parsedSubject
            : new NormalizedSecuritySubject(SecuritySubjectTypeNames.Ip, request.ClientIp, request.ClientIp);
        SecuritySubjectStateEntity? subjectState = null;
        if (IPAddress.TryParse(request.ClientIp, out var parsedIp))
        {
            var resolved = await (subjectService ?? new SecuritySubjectService(db, timeProvider))
                .ResolveOrCreateIpAsync(parsedIp, request.CountryCode, request.RegionCode, request.Asn, cancellationToken);
            subjectState = resolved.State;
        }

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
        var statusClass = statusCode / 100;
        var method = NormalizeMethod(request.Method);
        var pathPrefix = NormalizePathPrefix(request.PathPrefix, request.Path);
        var resource = NormalizeResource(null, request.Host);
        var requestId = NormalizeCorrelationId(request.RequestId);
        var userAgentHash = ResolveUserAgentHash(request.UserAgent, request.UserAgentHash);
        var requestBucket = await db.SecurityRequestBuckets.SingleOrDefaultAsync(
            x => x.BucketStartUtc == bucketStartUtc
                && x.ClientIp == request.ClientIp
                && x.Resource == resource
                && x.TraefikInstance == "forward-auth"
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
                TraefikInstance = "forward-auth",
                CountryCode = request.CountryCode,
                SubjectType = normalizedSubject.SubjectType,
                NormalizedSubjectValue = normalizedSubject.NormalizedValue,
                RegionCode = request.RegionCode,
                Asn = request.Asn,
                StatusClass = statusClass,
                Method = method,
                PathPrefix = pathPrefix,
            };
            db.SecurityRequestBuckets.Add(requestBucket);
        }

        requestBucket.TotalCount++;
        requestBucket.RequestCount++;
        requestBucket.UpdatedAtUtc = now;
        switch (decision)
        {
            case "blocked":
                requestBucket.BlockedCount++;
                break;
            case "challenged":
                requestBucket.ChallengedCount++;
                if (subjectState?.ChallengeRequired == true)
                {
                    requestBucket.ChallengeIgnoredCount++;
                }
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
            SubjectType = normalizedSubject.SubjectType,
            SubjectValue = normalizedSubject.SubjectValue,
            NormalizedSubjectValue = normalizedSubject.NormalizedValue,
            EventType = "forward_auth_decision",
            Decision = request.Decision,
            Source = "hashi_forward_auth",
            RequestMethod = method,
            RequestPath = request.Path,
            StatusCode = statusCode,
            RequestId = requestId,
            UserAgentHash = userAgentHash,
        });
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task RecordWafEventAsync(
        string clientIp,
        string host,
        string path,
        string action,
        string? requestId = null,
        string? userAgent = null,
        string? userAgentHash = null,
        CancellationToken cancellationToken = default)
    {
        db.SecurityEvents.Add(new SecurityEventEntity
        {
            Category = "waf",
            Action = action,
            ClientIp = clientIp,
            Host = host,
            Path = path,
            EventType = "waf_event",
            Decision = action,
            Source = "waf",
            RequestPath = path,
            RequestId = NormalizeCorrelationId(requestId),
            UserAgentHash = ResolveUserAgentHash(userAgent, userAgentHash),
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
            request.RequestId,
            request.UserAgent,
            request.UserAgentHash,
            cancellationToken);

    public async Task<SecurityDashboardResponse> GetDashboardAsync(
        int hours = 24,
        string? resourceFilter = null,
        string? traefikHostFilter = null,
        Guid? firewallHostIdFilter = null,
        CancellationToken cancellationToken = default)
    {
        var windowHours = Math.Clamp(hours, 1, 720);
        var dashboardNow = timeProvider?.GetUtcNow() ?? DateTimeOffset.UtcNow;
        var since = dashboardNow.AddHours(-windowHours);
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

        var topIpStats = await accessEventsQuery
            .Where(x => x.Decision == "blocked")
            .GroupBy(x => x.ClientIp)
            .Select(x => new { Ip = x.Key, Count = x.LongCount(), LastSeenAtUtc = x.Max(y => y.ReceivedAtUtc) })
            .OrderByDescending(x => x.Count)
            .ThenBy(x => x.Ip)
            .Take(10)
            .ToListAsync(cancellationToken);
        var topIpValues = topIpStats.Select(x => x.Ip).ToList();
        var latestTopIpEvents = await accessEventsQuery
            .Where(x => x.Decision == "blocked")
            .Where(x => topIpValues.Contains(x.ClientIp))
            .OrderByDescending(x => x.ReceivedAtUtc)
            .Select(x => new { x.ClientIp, x.CountryCode, x.Asn, x.ReceivedAtUtc })
            .ToListAsync(cancellationToken);
        var latestTopIpContext = latestTopIpEvents
            .GroupBy(x => x.ClientIp)
            .ToDictionary(x => x.Key, x => x.First());
        var activeIpBlocks = await db.BlocklistEntries.AsNoTracking()
            .Where(x => x.Enabled)
            .Where(x => x.ExpiresAtUtc == null || x.ExpiresAtUtc > dashboardNow)
            .Where(x => x.Type == BlocklistTypeNames.Ip || x.Type == BlocklistTypeNames.Cidr || x.Type == string.Empty)
            .Where(x => topIpValues.Contains(x.Value) || topIpValues.Contains(x.ClientIp))
            .ToListAsync(cancellationToken);
        var activeIpBlockByValue = activeIpBlocks
            .GroupBy(x => string.IsNullOrWhiteSpace(x.Value) ? x.ClientIp : x.Value, StringComparer.OrdinalIgnoreCase)
            .ToDictionary(x => x.Key, x => x.OrderByDescending(y => y.CreatedAtUtc).First(), StringComparer.OrdinalIgnoreCase);
        var activeBucketStates = await db.AbuseBuckets.AsNoTracking()
            .Where(x => topIpValues.Contains(x.ClientIp))
            .ToDictionaryAsync(
                x => x.ClientIp,
                x => SecuritySubjectStateNames.Normalize(x.State),
                StringComparer.OrdinalIgnoreCase,
                cancellationToken);
        var topIps = topIpStats
            .Select(x =>
            {
                latestTopIpContext.TryGetValue(x.Ip, out var latest);
                activeIpBlockByValue.TryGetValue(x.Ip, out var block);
                activeBucketStates.TryGetValue(x.Ip, out var subjectState);
                return new SecurityTopBlockedIpItem
                {
                    Ip = x.Ip,
                    Count = x.Count,
                    LastSeenAtUtc = x.LastSeenAtUtc,
                    CountryCode = latest?.CountryCode,
                    Asn = latest?.Asn,
                    Reason = block?.Reason,
                    ExpiresAtUtc = block?.ExpiresAtUtc,
                    SubjectState = subjectState,
                };
            })
            .ToList();

        var topChallengedIpStats = await accessEventsQuery
            .Where(x => x.Decision == "challenged")
            .GroupBy(x => x.ClientIp)
            .Select(x => new { Ip = x.Key, Count = x.LongCount(), LastSeenAtUtc = x.Max(y => y.ReceivedAtUtc) })
            .OrderByDescending(x => x.Count)
            .ThenBy(x => x.Ip)
            .Take(10)
            .ToListAsync(cancellationToken);
        var topChallengedIpValues = topChallengedIpStats.Select(x => x.Ip).ToList();
        var latestTopChallengedIpEvents = await accessEventsQuery
            .Where(x => x.Decision == "challenged")
            .Where(x => topChallengedIpValues.Contains(x.ClientIp))
            .OrderByDescending(x => x.ReceivedAtUtc)
            .Select(x => new { x.ClientIp, x.CountryCode, x.Asn, x.ReceivedAtUtc })
            .ToListAsync(cancellationToken);
        var latestTopChallengedIpContext = latestTopChallengedIpEvents
            .GroupBy(x => x.ClientIp)
            .ToDictionary(x => x.Key, x => x.First());
        var challengedBucketStates = await db.AbuseBuckets.AsNoTracking()
            .Where(x => topChallengedIpValues.Contains(x.ClientIp))
            .ToDictionaryAsync(
                x => x.ClientIp,
                x => SecuritySubjectStateNames.Normalize(x.State),
                StringComparer.OrdinalIgnoreCase,
                cancellationToken);
        var topChallengedIps = topChallengedIpStats
            .Select(x =>
            {
                latestTopChallengedIpContext.TryGetValue(x.Ip, out var latest);
                challengedBucketStates.TryGetValue(x.Ip, out var subjectState);
                return new SecurityTopBlockedIpItem
                {
                    Ip = x.Ip,
                    Count = x.Count,
                    LastSeenAtUtc = x.LastSeenAtUtc,
                    CountryCode = latest?.CountryCode,
                    Asn = latest?.Asn,
                    SubjectState = subjectState,
                };
            })
            .ToList();

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

        var recentManualActions = await securityEventsQuery
            .Where(x => x.Category == "manual_action" || (x.EventType != null && x.EventType.StartsWith("manual_")))
            .OrderByDescending(x => x.OccurredAtUtc)
            .Take(10)
            .Select(x => new SecurityRecentEventItem
            {
                OccurredAtUtc = x.OccurredAtUtc,
                Category = x.Category,
                Action = x.Action,
                ClientIp = x.ClientIp ?? x.NormalizedSubjectValue,
                Host = x.Host,
                Path = x.Path ?? x.RequestPath,
            })
            .ToListAsync(cancellationToken);

        var blocklistMatchTimes = await db.BlocklistEntries.AsNoTracking()
            .Where(x => x.LastHitAtUtc != null && x.LastHitAtUtc >= since)
            .Select(x => x.LastHitAtUtc!.Value)
            .ToListAsync(cancellationToken);
        var blocklistMatchesOverTime = blocklistMatchTimes
            .Select(TruncateToHourUtc)
            .GroupBy(x => x)
            .OrderBy(x => x.Key)
            .Select(x => new SecurityBlocklistMatchBucket
            {
                BucketStartUtc = x.Key,
                Count = x.LongCount(),
            })
            .ToList();

        var captchaSolved = await securityEventsQuery
            .Where(x => x.Category == "captcha" && x.Action == "challenge_solved")
            .LongCountAsync(cancellationToken);
        var captchaFailed = await securityEventsQuery
            .Where(x => x.Category == "captcha" && (x.Action == "challenge_failed" || x.Action == "challenge_unavailable"))
            .LongCountAsync(cancellationToken);
        var captchaIgnored = await securityEventsQuery
            .Where(x => x.Category == "captcha" && x.Action == "challenge_ignored")
            .LongCountAsync(cancellationToken);
        var captchaOutcomes = new SecurityCaptchaOutcomeSummary
        {
            Solved = captchaSolved,
            Failed = captchaFailed,
            Ignored = captchaIgnored,
        };

        var wafDetections = await securityEventsQuery
            .Where(x => x.Category == "waf")
            .LongCountAsync(cancellationToken);
        var wafBlocks = await securityEventsQuery
            .Where(x => x.Category == "waf")
            .Where(x => x.Action == "blocked" || x.Action == "block" || x.Action == "deny")
            .LongCountAsync(cancellationToken);

        var activeSoftBlocks = await db.SecuritySubjectStates.AsNoTracking()
            .Join(
                db.SecuritySubjects.AsNoTracking(),
                state => state.SecuritySubjectId,
                subject => subject.Id,
                (state, subject) => new { state, subject })
            .Where(x => x.subject.CurrentState == SecuritySubjectStateNames.SoftBlocked || x.state.SoftBlockedUntilUtc > dashboardNow)
            .Where(x => x.state.SoftBlockedUntilUtc == null || x.state.SoftBlockedUntilUtc > dashboardNow)
            .OrderByDescending(x => x.subject.LastSeenAtUtc)
            .Take(10)
            .Select(x => new SecurityActiveBlockItem
            {
                SubjectType = x.subject.SubjectType,
                SubjectValue = x.subject.NormalizedValue,
                BlockType = "soft",
                Reason = x.state.LastEscalationReason,
                ExpiresAtUtc = x.state.SoftBlockedUntilUtc,
                LastSeenAtUtc = x.subject.LastSeenAtUtc,
                FirewallSynced = false,
            })
            .ToListAsync(cancellationToken);

        var activeFirewallBlocks = await db.BlocklistEntries.AsNoTracking()
            .Where(x => x.Enabled)
            .Where(x => x.ExpiresAtUtc == null || x.ExpiresAtUtc > dashboardNow)
            .Where(x => x.Type == BlocklistTypeNames.Ip || x.Type == BlocklistTypeNames.Cidr || x.Type == string.Empty)
            .Where(x => x.SyncedToFirewall || x.EnforcementMode == BlocklistEnforcementModeNames.Firewall)
            .OrderByDescending(x => x.LastHitAtUtc ?? x.CreatedAtUtc)
            .Take(10)
            .Select(x => new SecurityActiveBlockItem
            {
                SubjectType = string.IsNullOrWhiteSpace(x.SubjectType) ? x.Type : x.SubjectType,
                SubjectValue = string.IsNullOrWhiteSpace(x.NormalizedValue)
                    ? string.IsNullOrWhiteSpace(x.Value) ? x.ClientIp : x.Value
                    : x.NormalizedValue,
                BlockType = "firewall",
                Reason = x.Reason,
                ExpiresAtUtc = x.ExpiresAtUtc,
                LastSeenAtUtc = x.LastHitAtUtc ?? x.CreatedAtUtc,
                FirewallSynced = x.SyncedToFirewall,
            })
            .ToListAsync(cancellationToken);

        var enabledBlocklistSources = await db.BlocklistSources.AsNoTracking()
            .Where(x => x.Enabled)
            .ToListAsync(cancellationToken);
        var staleBlocklistSources = enabledBlocklistSources
            .Where(x => x.LastFetchedAtUtc == null
                || x.LastFetchStatus != BlocklistFetchStatusNames.Succeeded
                || x.LastFetchedAtUtc <= dashboardNow.AddHours(-Math.Clamp(x.RefreshIntervalHours, 1, 8760)))
            .OrderBy(x => x.LastFetchedAtUtc ?? DateTimeOffset.MinValue)
            .Take(10)
            .Select(x => new SecurityStaleBlocklistSourceItem
            {
                Id = x.Id,
                Name = x.Name,
                LastFetchStatus = x.LastFetchStatus,
                LastFetchError = x.LastFetchError,
                LastFetchedAtUtc = x.LastFetchedAtUtc,
                StaleSinceUtc = x.LastFetchedAtUtc ?? x.CreatedAtUtc,
            })
            .ToList();

        var geoIpStatus = await BuildGeoIpStatusAsync(dashboardNow, cancellationToken);

        var firewallActiveIpBlocks = await db.BlocklistEntries.AsNoTracking()
            .Where(x => x.Enabled)
            .Where(x => x.ExpiresAtUtc == null || x.ExpiresAtUtc > dashboardNow)
            .Where(x => x.Type == BlocklistTypeNames.Ip || x.Type == BlocklistTypeNames.Cidr || x.Type == string.Empty)
            .Where(x => x.SyncedToFirewall || x.EnforcementMode == BlocklistEnforcementModeNames.Firewall)
            .LongCountAsync(cancellationToken);
        var blocklistCount = await db.BlocklistEntries.AsNoTracking()
            .Where(x => x.Enabled)
            .Where(x => x.ExpiresAtUtc == null || x.ExpiresAtUtc > dashboardNow)
            .LongCountAsync(cancellationToken);
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
            topChallengedIps,
            topCountries,
            topAsns,
            topResources,
            recentEvents,
            recentManualActions,
            blocklistMatchesOverTime,
            captchaOutcomes,
            activeSoftBlocks,
            activeFirewallBlocks,
            staleBlocklistSources,
            geoIpStatus,
            resourceFilters,
            traefikHostFilters,
            firewallHostFilters,
            firewallActiveIpBlocks,
            blocklistCount,
            securityEventCount);
    }

    public async Task<BlocklistSyncResponse> SyncBlocklistToAllFirewallsAsync(CancellationToken cancellationToken = default)
    {
        var now = timeProvider?.GetUtcNow() ?? DateTimeOffset.UtcNow;
        var hosts = await db.FirewallHosts.ToListAsync(cancellationToken);
        var activeIpEntries = await db.BlocklistEntries
            .Where(x => x.Enabled)
            .Where(x => x.ExpiresAtUtc == null || x.ExpiresAtUtc > now)
            .Where(x => x.Type == BlocklistTypeNames.Ip || x.Type == BlocklistTypeNames.Cidr || x.Type == string.Empty)
            .ToListAsync(cancellationToken);
        if (hosts.Count == 0 && activeIpEntries.Count > 0)
        {
            return new BlocklistSyncResponse(false, activeIpEntries.Count, 0, ["No firewall hosts configured."]);
        }

        var entryIds = activeIpEntries.Select(x => x.Id).ToList();
        var appliedStates = await db.BlocklistAppliedHosts
            .Where(x => entryIds.Contains(x.BlocklistEntryId))
            .ToListAsync(cancellationToken);
        var pending = activeIpEntries
            .Where(entry => hosts.Any(host => !appliedStates.Any(
                state => state.BlocklistEntryId == entry.Id
                    && state.FirewallHostId == host.Id
                    && state.Status == BlocklistApplyStatusNames.Applied)))
            .ToList();
        if (pending.Count == 0)
        {
            var reconciledAt = timeProvider?.GetUtcNow() ?? DateTimeOffset.UtcNow;
            var reconciled = false;
            foreach (var entry in activeIpEntries)
            {
                if (!entry.SyncedToFirewall)
                {
                    entry.SyncedToFirewall = true;
                    reconciled = true;
                }
            }

            reconciled |= await MarkSyncedAutomaticBucketsAsync(activeIpEntries, reconciledAt, cancellationToken);
            if (reconciled)
            {
                await db.SaveChangesAsync(cancellationToken);
            }

            return new BlocklistSyncResponse(true, 0, 0, []);
        }

        var failures = new List<string>();
        var appliedHosts = 0;
        var appliedAt = timeProvider?.GetUtcNow() ?? DateTimeOffset.UtcNow;
        foreach (var host in hosts)
        {
            try
            {
                var result = await firewallApply.ApplyForHostAsync(host.Id, cancellationToken);
                if (!result.Succeeded)
                {
                    failures.Add($"{host.Name}: {result.Message}");
                    UpsertApplyStates(appliedStates, pending, host.Id, BlocklistApplyStatusNames.Failed, null, result.Message);
                    continue;
                }

                appliedHosts++;
                UpsertApplyStates(appliedStates, pending, host.Id, BlocklistApplyStatusNames.Applied, appliedAt, null);
            }
            catch (Exception ex)
            {
                failures.Add($"{host.Name}: {ex.Message}");
                UpsertApplyStates(appliedStates, pending, host.Id, BlocklistApplyStatusNames.Failed, null, ex.Message);
            }
        }

        if (failures.Count > 0)
        {
            await db.SaveChangesAsync(cancellationToken);
            logger.LogWarning("Blocklist sync failed on {Count} firewall host(s): {Errors}", failures.Count, string.Join("; ", failures));
            return new BlocklistSyncResponse(false, pending.Count, appliedHosts, failures);
        }

        foreach (var entry in pending)
        {
            entry.SyncedToFirewall = true;
        }

        await MarkSyncedAutomaticBucketsAsync(pending, appliedAt, cancellationToken);

        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync(
            "security",
            "blocklist_synced",
            metadata: new { hosts = appliedHosts, entries = pending.Count },
            cancellationToken: cancellationToken);
        return new BlocklistSyncResponse(true, pending.Count, appliedHosts, []);
    }

    private void UpsertApplyStates(
        List<BlocklistAppliedHostEntity> trackedStates,
        IReadOnlyList<BlocklistEntryEntity> entries,
        Guid firewallHostId,
        string status,
        DateTimeOffset? appliedAtUtc,
        string? error)
    {
        foreach (var entry in entries)
        {
            var state = trackedStates.FirstOrDefault(x => x.BlocklistEntryId == entry.Id && x.FirewallHostId == firewallHostId);
            if (state is null)
            {
                state = new BlocklistAppliedHostEntity
                {
                    BlocklistEntryId = entry.Id,
                    FirewallHostId = firewallHostId,
                };
                trackedStates.Add(state);
                db.BlocklistAppliedHosts.Add(state);
            }

            state.Status = status;
            state.AppliedAtUtc = appliedAtUtc;
            state.LastError = error;
        }
    }

    private async Task<bool> MarkSyncedAutomaticBucketsAsync(
        IReadOnlyList<BlocklistEntryEntity> syncedEntries,
        DateTimeOffset syncedAt,
        CancellationToken cancellationToken)
    {
        var syncedIpValues = syncedEntries
            .Where(x => x.Source != BlocklistSourceNames.Manual)
            .Select(x => string.IsNullOrWhiteSpace(x.Value) ? x.ClientIp : x.Value)
            .Where(x => !string.IsNullOrWhiteSpace(x))
            .Distinct(StringComparer.OrdinalIgnoreCase)
            .ToList();
        if (syncedIpValues.Count == 0)
        {
            return false;
        }

        var changed = false;
        var syncedBuckets = await db.AbuseBuckets
            .Where(x => syncedIpValues.Contains(x.ClientIp))
            .ToListAsync(cancellationToken);
        foreach (var bucket in syncedBuckets)
        {
            var state = SecuritySubjectStateNames.Normalize(bucket.State);
            if (state is SecuritySubjectStateNames.ManuallyAllowed or SecuritySubjectStateNames.FirewallBlocked)
            {
                continue;
            }

            bucket.State = SecuritySubjectStateNames.FirewallBlocked;
            bucket.UpdatedAtUtc = syncedAt;
            changed = true;
        }

        return changed;
    }

    private static DateTimeOffset TruncateToMinuteUtc(DateTimeOffset value)
    {
        var utc = value.ToUniversalTime();
        return new DateTimeOffset(utc.Year, utc.Month, utc.Day, utc.Hour, utc.Minute, 0, TimeSpan.Zero);
    }

    private static DateTimeOffset TruncateToHourUtc(DateTimeOffset value)
    {
        var utc = value.ToUniversalTime();
        return new DateTimeOffset(utc.Year, utc.Month, utc.Day, utc.Hour, 0, 0, TimeSpan.Zero);
    }

    private async Task<SecurityGeoIpStatusSummary> BuildGeoIpStatusAsync(DateTimeOffset now, CancellationToken cancellationToken)
    {
        var settings = await db.AppSettings.AsNoTracking().SingleOrDefaultAsync(cancellationToken);
        var expected = new[] { "GeoLite2-City", "GeoLite2-Country", "GeoLite2-ASN" };
        var databaseRows = await db.GeoIpDatabases.AsNoTracking().ToListAsync(cancellationToken);
        var databases = databaseRows.ToDictionary(x => x.EditionId, StringComparer.OrdinalIgnoreCase);
        var missing = expected
            .Where(x => !databases.TryGetValue(x, out var database)
                || database.Status != GeoIpUpdateStatusNames.Succeeded
                || database.LastDownloadedAtUtc is null)
            .ToList();
        var intervalHours = Math.Clamp(settings?.GeoIpUpdateIntervalHours ?? 72, 12, 168);
        var staleCutoff = now.AddHours(-intervalHours * 2);
        var stale = databases.Values
            .Where(x => x.Status == GeoIpUpdateStatusNames.Failed
                || x.Status == GeoIpUpdateStatusNames.NeverRun
                || x.LastDownloadedAtUtc <= staleCutoff)
            .Select(x => x.EditionId)
            .Distinct(StringComparer.OrdinalIgnoreCase)
            .OrderBy(x => x)
            .ToList();

        var enabled = settings?.GeoIpEnabled ?? false;
        var lastUpdateStatus = settings?.GeoIpLastUpdateStatus ?? GeoIpUpdateStatusNames.NeverRun;
        var nextUpdateAtUtc = settings?.GeoIpNextUpdateAtUtc;
        var isStale = enabled
            && (lastUpdateStatus != GeoIpUpdateStatusNames.Succeeded
                || (nextUpdateAtUtc is not null && nextUpdateAtUtc <= now)
                || missing.Count > 0
                || stale.Count > 0);

        return new SecurityGeoIpStatusSummary
        {
            Enabled = enabled,
            DatabaseAvailable = enabled && missing.Count == 0,
            IsStale = isStale,
            LastUpdateStatus = lastUpdateStatus,
            LastUpdateMessage = settings?.GeoIpLastUpdateMessage,
            LastUpdateAtUtc = settings?.GeoIpLastUpdateAtUtc,
            NextUpdateAtUtc = nextUpdateAtUtc,
            MissingDatabases = missing,
            StaleDatabases = stale,
        };
    }

    private static string? NormalizeCorrelationId(string? value)
    {
        if (string.IsNullOrWhiteSpace(value))
        {
            return null;
        }

        var trimmed = value.Trim();
        return trimmed.Length <= 128 ? trimmed : trimmed[..128];
    }

    private static string? ResolveUserAgentHash(string? userAgent, string? userAgentHash)
    {
        if (!string.IsNullOrWhiteSpace(userAgent))
        {
            return HashUserAgent(userAgent);
        }

        if (string.IsNullOrWhiteSpace(userAgentHash))
        {
            return null;
        }

        var normalized = userAgentHash.Trim();
        return normalized.Length <= 128 ? normalized : normalized[..128];
    }

    private static string HashUserAgent(string userAgent)
        => Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(userAgent.Trim()))).ToLowerInvariant();

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

    private async Task<BlocklistSyncResponse?> TrySyncBlocklistToAllFirewallsAsync(CancellationToken cancellationToken)
    {
        try
        {
            return await SyncBlocklistToAllFirewallsAsync(cancellationToken);
        }
        catch (Exception ex)
        {
            logger.LogWarning(ex, "Automatic blocklist sync failed after abuse block.");
            return null;
        }
    }

    private static string StateForScore(int score)
        => score switch
        {
            >= 20 => SecuritySubjectStateNames.SoftBlocked,
            >= 15 => SecuritySubjectStateNames.SoftBlocked,
            >= 10 => SecuritySubjectStateNames.Challenged,
            >= 8 => SecuritySubjectStateNames.Suspect,
            >= 4 => SecuritySubjectStateNames.Warm,
            _ => SecuritySubjectStateNames.Observed,
        };

    private static string DecisionForState(string state)
        => state switch
        {
            SecuritySubjectStateNames.ManuallyBlocked => "blocked",
            SecuritySubjectStateNames.FirewallBlocked => "blocked",
            SecuritySubjectStateNames.SoftBlocked => "blocked",
            SecuritySubjectStateNames.Challenged => "challenged",
            _ => "allowed",
        };

    private async Task<BlocklistEntryEntity?> FindActiveBlocklistEntryAsync(
        string clientIp,
        string source,
        CancellationToken cancellationToken)
    {
        var now = timeProvider?.GetUtcNow() ?? DateTimeOffset.UtcNow;
        return await db.BlocklistEntries
            .Where(x => x.Enabled)
            .Where(x => x.ExpiresAtUtc == null || x.ExpiresAtUtc > now)
            .Where(x => x.Scope == BlocklistScopeNames.Global)
            .Where(x => x.Source == source)
            .Where(x => x.Type == BlocklistTypeNames.Ip || x.Type == string.Empty)
            .Where(x => x.Value == clientIp || x.ClientIp == clientIp)
            .OrderByDescending(x => x.CreatedAtUtc)
            .FirstOrDefaultAsync(cancellationToken);
    }

    private async Task<bool> IsManuallyAllowedAsync(
        string clientIp,
        string? countryCode,
        string? regionCode,
        string? asn,
        CancellationToken cancellationToken)
    {
        var subjects = await db.FirewallAllowedSubjects.AsNoTracking()
            .Where(x => x.Enabled)
            .ToListAsync(cancellationToken);

        return subjects.Any(x => MatchesAllowedSubject(x, clientIp, countryCode, regionCode, asn));
    }

    private static bool MatchesAllowedSubject(
        FirewallAllowedSubjectEntity subject,
        string clientIp,
        string? countryCode,
        string? regionCode,
        string? asn)
        => subject.SubjectKind.Trim().ToLowerInvariant() switch
        {
            FirewallSubjectKindNames.Ip => string.Equals(subject.SubjectValue, clientIp, StringComparison.OrdinalIgnoreCase),
            FirewallSubjectKindNames.Cidr => IsInCidr(clientIp, subject.SubjectValue),
            FirewallSubjectKindNames.Country => string.Equals(subject.SubjectValue, countryCode, StringComparison.OrdinalIgnoreCase),
            FirewallSubjectKindNames.Asn => string.Equals(subject.SubjectValue, asn, StringComparison.OrdinalIgnoreCase),
            _ => false,
        };

    private static bool IsInCidr(string ipText, string cidr)
    {
        if (!IPAddress.TryParse(ipText, out var ip) || string.IsNullOrWhiteSpace(cidr) || !cidr.Contains('/'))
        {
            return false;
        }

        var parts = cidr.Split('/');
        if (!IPAddress.TryParse(parts[0], out var network) || !int.TryParse(parts[1], out var prefixLength))
        {
            return false;
        }

        var ipBytes = ip.GetAddressBytes();
        var networkBytes = network.GetAddressBytes();
        if (ipBytes.Length != networkBytes.Length)
        {
            return false;
        }

        var fullBytes = prefixLength / 8;
        var remainingBits = prefixLength % 8;
        for (var i = 0; i < fullBytes; i++)
        {
            if (ipBytes[i] != networkBytes[i])
            {
                return false;
            }
        }

        if (remainingBits == 0)
        {
            return true;
        }

        var mask = (byte)(0xFF << (8 - remainingBits));
        return (ipBytes[fullBytes] & mask) == (networkBytes[fullBytes] & mask);
    }
}
