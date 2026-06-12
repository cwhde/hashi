using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Platform;

public sealed class SecurityAddendumJobWorker(
    IServiceScopeFactory scopeFactory,
    ILogger<SecurityAddendumJobWorker> logger) : BackgroundService
{
    private const int BlocklistFetchIntervalSeconds = 3600;
    private const int MaintenanceIntervalSeconds = 300;

    public async Task<IReadOnlyList<SecurityAddendumJobRunResult>> ProcessOnceAsync(CancellationToken cancellationToken = default)
    {
        using var scope = scopeFactory.CreateScope();
        var services = scope.ServiceProvider;
        var results = new List<SecurityAddendumJobRunResult>
        {
            await RunJobAsync(
                services,
                BackgroundJobKeys.BlocklistFetch,
                BlocklistFetchIntervalSeconds,
                async (provider, ct) =>
                {
                    var blocklists = provider.GetRequiredService<BlocklistSourceManagementService>();
                    var result = await blocklists.RefreshDueSourcesAsync(ct);
                    return result.Failed == 0
                        ? new SecurityAddendumJobHandlerResult(true, $"Refreshed {result.Succeeded} due blocklist source(s); skipped {result.SkippedNotModified}; due {result.DueSources}.", null)
                        : new SecurityAddendumJobHandlerResult(false, $"Refreshed {result.Succeeded} due blocklist source(s); failed {result.Failed}; skipped {result.SkippedNotModified}.", $"{result.Failed} blocklist source refresh(es) failed.");
                },
                cancellationToken),
            await RunJobAsync(
                services,
                BackgroundJobKeys.SecurityBucketAggregation,
                MaintenanceIntervalSeconds,
                async (provider, ct) =>
                {
                    var maintenance = provider.GetRequiredService<SecurityMaintenanceService>();
                    var result = await maintenance.AggregateSecurityBucketsAsync(ct);
                    return new SecurityAddendumJobHandlerResult(true, $"Security request buckets available: {result.RecentBucketCount}.", null);
                },
                cancellationToken),
            await RunJobAsync(
                services,
                BackgroundJobKeys.BlockExpiry,
                MaintenanceIntervalSeconds,
                async (provider, ct) =>
                {
                    var maintenance = provider.GetRequiredService<SecurityMaintenanceService>();
                    var result = await maintenance.ExpireBlocksAsync(ct);
                    return new SecurityAddendumJobHandlerResult(
                        true,
                        $"Expired {result.ManualEntriesExpired} manual block(s), {result.BlocklistEntriesExpired} blocklist entry/entries, and updated {result.SubjectStatesUpdated} subject state(s).",
                        null);
                },
                cancellationToken),
            await RunJobAsync(
                services,
                BackgroundJobKeys.InternalAgentDnsSync,
                MaintenanceIntervalSeconds,
                SyncInternalAgentDnsAsync,
                cancellationToken),
            await RunJobAsync(
                services,
                BackgroundJobKeys.ChallengeCleanup,
                MaintenanceIntervalSeconds,
                async (provider, ct) =>
                {
                    var maintenance = provider.GetRequiredService<SecurityMaintenanceService>();
                    var result = await maintenance.CleanupStaleChallengesAsync(ct);
                    return new SecurityAddendumJobHandlerResult(true, $"Cleared {result.ChallengesCleared} stale CAPTCHA challenge(s).", null);
                },
                cancellationToken),
        };

        return results;
    }

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await ProcessOnceAsync(stoppingToken);
            }
            catch (Exception ex) when (ex is not OperationCanceledException)
            {
                logger.LogError(ex, "Security addendum background jobs failed.");
            }

            await Task.Delay(TimeSpan.FromMinutes(1), stoppingToken);
        }
    }

    private static async Task<SecurityAddendumJobRunResult> RunJobAsync(
        IServiceProvider services,
        string jobKey,
        int intervalSeconds,
        Func<IServiceProvider, CancellationToken, Task<SecurityAddendumJobHandlerResult>> handler,
        CancellationToken cancellationToken)
    {
        var db = services.GetRequiredService<HashiDbContext>();
        var jobs = services.GetRequiredService<BackgroundJobService>();
        var job = await db.BackgroundJobs.AsNoTracking().SingleOrDefaultAsync(x => x.JobKey == jobKey, cancellationToken);
        if (job?.NextRunAtUtc is { } nextRun && nextRun > DateTimeOffset.UtcNow)
        {
            return new SecurityAddendumJobRunResult(jobKey, false, true, null, null);
        }

        await jobs.BeginRunAsync(jobKey, cancellationToken);
        try
        {
            var result = await handler(services, cancellationToken);
            await jobs.CompleteRunAsync(jobKey, result.Succeeded, result.DiffSummary, result.Error, intervalSeconds, cancellationToken);
            return new SecurityAddendumJobRunResult(jobKey, true, result.Succeeded, result.DiffSummary, result.Error);
        }
        catch (Exception ex)
        {
            await jobs.CompleteRunAsync(jobKey, false, null, ex.Message, intervalSeconds, cancellationToken);
            return new SecurityAddendumJobRunResult(jobKey, true, false, null, ex.Message);
        }
    }

    private static async Task<SecurityAddendumJobHandlerResult> SyncInternalAgentDnsAsync(
        IServiceProvider services,
        CancellationToken cancellationToken)
    {
        var db = services.GetRequiredService<HashiDbContext>();
        var settings = await db.InternalAgentDnsSettings.AsNoTracking().SingleOrDefaultAsync(cancellationToken);
        if (settings is null || !settings.Enabled || settings.AdGuardConnectionId is not Guid connectionId)
        {
            return new SecurityAddendumJobHandlerResult(true, "Internal agent DNS is disabled or has no AdGuard connection.", null);
        }

        var sync = services.GetRequiredService<AdGuardSyncService>();
        var result = await sync.SyncManagedRewritesAsync(connectionId, confirmDestructive: true, cancellationToken);
        return new SecurityAddendumJobHandlerResult(
            result.Succeeded,
            result.Succeeded ? "Internal agent DNS rewrites synced." : result.Message,
            result.Succeeded ? null : result.Message);
    }
}

public sealed class SecurityMaintenanceService(
    HashiDbContext db,
    AuditService audit)
{
    public async Task<SecurityBucketAggregationResult> AggregateSecurityBucketsAsync(CancellationToken cancellationToken = default)
    {
        var recentSince = DateTimeOffset.UtcNow.AddHours(-24);
        var recentBuckets = await db.SecurityRequestBuckets.AsNoTracking()
            .LongCountAsync(x => x.BucketStartUtc >= recentSince, cancellationToken);
        return new SecurityBucketAggregationResult(recentBuckets);
    }

    public async Task<SecurityBlockExpiryResult> ExpireBlocksAsync(CancellationToken cancellationToken = default)
    {
        var now = DateTimeOffset.UtcNow;
        var manualEntries = await db.ManualSecurityEntries
            .Where(x => x.Enabled)
            .Where(x => !x.IsPermanent)
            .Where(x => x.ExpiresAtUtc != null && x.ExpiresAtUtc <= now)
            .ToListAsync(cancellationToken);
        foreach (var entry in manualEntries)
        {
            entry.Enabled = false;
        }

        var blocklistEntries = await db.BlocklistEntries
            .Where(x => x.Enabled)
            .Where(x => x.ExpiresAtUtc != null && x.ExpiresAtUtc <= now)
            .ToListAsync(cancellationToken);
        foreach (var entry in blocklistEntries)
        {
            entry.Enabled = false;
            entry.SyncedToFirewall = false;
        }

        var states = await db.SecuritySubjectStates
            .Include(x => x.SecuritySubject)
            .Where(x =>
                (x.SoftBlockedUntilUtc != null && x.SoftBlockedUntilUtc <= now) ||
                (x.FirewallBlockedUntilUtc != null && x.FirewallBlockedUntilUtc <= now) ||
                x.ManualBlockActive)
            .ToListAsync(cancellationToken);

        var activeManualBlocks = await db.ManualSecurityEntries.AsNoTracking()
            .Where(x =>
                x.Enabled &&
                x.EntryType == ManualSecurityEntryTypeNames.Block &&
                (x.IsPermanent || x.ExpiresAtUtc == null || x.ExpiresAtUtc > now))
            .ToListAsync(cancellationToken);

        var activeManualBlockLookup = activeManualBlocks
            .ToLookup(x => (x.SubjectType.ToUpperInvariant(), x.NormalizedValue.ToUpperInvariant()));

        var statesUpdated = 0;
        foreach (var state in states)
        {
            var subject = state.SecuritySubject;
            if (subject is null)
            {
                continue;
            }

            var changed = false;
            if (state.SoftBlockedUntilUtc is not null && state.SoftBlockedUntilUtc <= now)
            {
                state.SoftBlockedUntilUtc = null;
                changed = true;
            }

            if (state.FirewallBlockedUntilUtc is not null && state.FirewallBlockedUntilUtc <= now)
            {
                state.FirewallBlockedUntilUtc = null;
                changed = true;
            }

            var hasActiveManualBlock = activeManualBlockLookup.Contains((subject.SubjectType.ToUpperInvariant(), subject.NormalizedValue.ToUpperInvariant()));
            if (state.ManualBlockActive != hasActiveManualBlock)
            {
                state.ManualBlockActive = hasActiveManualBlock;
                changed = true;
            }

            if (!state.ManualBlockActive && state.SoftBlockedUntilUtc is null && state.FirewallBlockedUntilUtc is null)
            {
                var nextState = state.ManualAllowActive
                    ? SecuritySubjectStateNames.ManuallyAllowed
                    : state.ChallengeRequired
                        ? SecuritySubjectStateNames.Challenged
                        : SecuritySubjectStateNames.Observed;
                if (subject.CurrentState != nextState)
                {
                    subject.CurrentState = nextState;
                    changed = true;
                }
            }

            if (!changed)
            {
                continue;
            }

            state.UpdatedAtUtc = now;
            statesUpdated++;
        }

        await db.SaveChangesAsync(cancellationToken);
        if (manualEntries.Count > 0 || blocklistEntries.Count > 0 || statesUpdated > 0)
        {
            await audit.WriteAsync(
                "security",
                "block_expiry_completed",
                metadata: new
                {
                    manualEntries = manualEntries.Count,
                    blocklistEntries = blocklistEntries.Count,
                    states = statesUpdated,
                },
                cancellationToken: cancellationToken);
        }

        return new SecurityBlockExpiryResult(manualEntries.Count, blocklistEntries.Count, statesUpdated);
    }

    public async Task<SecurityChallengeCleanupResult> CleanupStaleChallengesAsync(CancellationToken cancellationToken = default)
    {
        var now = DateTimeOffset.UtcNow;
        var settings = await db.CaptchaSettings.AsNoTracking().SingleOrDefaultAsync(cancellationToken);
        var staleSeconds = Math.Max(3600, settings?.MinimumRepeatChallengeSeconds ?? 3600);
        var staleBefore = now.AddSeconds(-staleSeconds);
        var states = await db.SecuritySubjectStates
            .Where(x => x.ChallengeRequired)
            .Where(x => x.ChallengeRequiredSinceUtc != null && x.ChallengeRequiredSinceUtc <= staleBefore)
            .Where(x => x.SoftBlockedUntilUtc == null || x.SoftBlockedUntilUtc <= now)
            .Where(x => x.FirewallBlockedUntilUtc == null || x.FirewallBlockedUntilUtc <= now)
            .ToListAsync(cancellationToken);

        var cleared = 0;
        foreach (var state in states)
        {
            var subject = await db.SecuritySubjects.SingleOrDefaultAsync(x => x.Id == state.SecuritySubjectId, cancellationToken);
            state.ChallengeRequired = false;
            state.ChallengeRequiredSinceUtc = null;
            state.ChallengeReason = null;
            state.ChallengeResourceId = null;
            state.RequestsWhileChallenged = 0;
            state.UpdatedAtUtc = now;
            if (subject is not null && SecuritySubjectStateNames.Normalize(subject.CurrentState) == SecuritySubjectStateNames.Challenged)
            {
                subject.CurrentState = state.ManualAllowActive
                    ? SecuritySubjectStateNames.ManuallyAllowed
                    : SecuritySubjectStateNames.Observed;
            }

            cleared++;
        }

        await db.SaveChangesAsync(cancellationToken);
        if (cleared > 0)
        {
            await audit.WriteAsync(
                "security",
                "challenge_cleanup_completed",
                metadata: new { cleared },
                cancellationToken: cancellationToken);
        }

        return new SecurityChallengeCleanupResult(cleared);
    }
}

public sealed record SecurityAddendumJobRunResult(
    string JobKey,
    bool Ran,
    bool Succeeded,
    string? DiffSummary,
    string? Error);

internal sealed record SecurityAddendumJobHandlerResult(
    bool Succeeded,
    string? DiffSummary,
    string? Error);

public sealed record SecurityBucketAggregationResult(long RecentBucketCount);

public sealed record SecurityBlockExpiryResult(
    int ManualEntriesExpired,
    int BlocklistEntriesExpired,
    int SubjectStatesUpdated);

public sealed record SecurityChallengeCleanupResult(int ChallengesCleared);
