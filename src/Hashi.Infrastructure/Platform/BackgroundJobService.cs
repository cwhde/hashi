using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public static class BackgroundJobKeys
{
    public const string SyncReconcile = "sync-reconcile";
    public const string MonitorCheck = "monitor-check";
    public const string MonitorRollup = "monitor-rollup";
    public const string ScriptCron = "script-cron";
    public const string AccessLogIngest = "access-log-ingest";
    public const string GeoIpUpdate = "geoip-update";
    public const string BlocklistFetch = "blocklist-fetch";
    public const string SecurityBucketAggregation = "security-bucket-aggregation";
    public const string BlockExpiry = "block-expiry";
    public const string InternalAgentDnsSync = "internal-agent-dns-sync";
    public const string ChallengeCleanup = "challenge-cleanup";
}

public sealed class BackgroundJobService(HashiDbContext db)
{
    public async Task EnsureJobsAsync(CancellationToken cancellationToken = default)
    {
        await EnsureJobAsync(BackgroundJobKeys.SyncReconcile, "Passive sync reconcile", 3600, cancellationToken);
        await EnsureJobAsync(BackgroundJobKeys.MonitorCheck, "Monitor checks", 30, cancellationToken);
        await EnsureJobAsync(BackgroundJobKeys.MonitorRollup, "Monitor rollups", 60, cancellationToken);
        await EnsureJobAsync(BackgroundJobKeys.ScriptCron, "Script cron sync", 60, cancellationToken);
        await EnsureJobAsync(BackgroundJobKeys.AccessLogIngest, "Traefik access-log ingest", 60, cancellationToken);
        await EnsureJobAsync(BackgroundJobKeys.GeoIpUpdate, "GeoIP database update", 259200, cancellationToken);
        await EnsureJobAsync(BackgroundJobKeys.BlocklistFetch, "Blocklist fetch", 3600, cancellationToken);
        await EnsureJobAsync(BackgroundJobKeys.SecurityBucketAggregation, "Security bucket aggregation", 300, cancellationToken);
        await EnsureJobAsync(BackgroundJobKeys.BlockExpiry, "Security block expiry", 300, cancellationToken);
        await EnsureJobAsync(BackgroundJobKeys.InternalAgentDnsSync, "Internal agent DNS sync", 300, cancellationToken);
        await EnsureJobAsync(BackgroundJobKeys.ChallengeCleanup, "CAPTCHA challenge cleanup", 300, cancellationToken);
    }

    public async Task<IReadOnlyList<BackgroundJobEntity>> ListAsync(CancellationToken cancellationToken = default)
    {
        await EnsureJobsAsync(cancellationToken);
        return await db.BackgroundJobs.AsNoTracking().OrderBy(x => x.DisplayName).ToListAsync(cancellationToken);
    }

    public async Task BeginRunAsync(string jobKey, CancellationToken cancellationToken = default)
    {
        var job = await GetOrCreateAsync(jobKey, cancellationToken);
        job.Status = BackgroundJobStatusNames.Running;
        job.LastStartedAtUtc = DateTimeOffset.UtcNow;
        job.LastError = null;
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task CompleteRunAsync(
        string jobKey,
        bool succeeded,
        string? diffSummary,
        string? error,
        int intervalSeconds,
        CancellationToken cancellationToken = default)
    {
        var job = await GetOrCreateAsync(jobKey, cancellationToken);
        var completedAt = DateTimeOffset.UtcNow;
        job.Status = succeeded ? BackgroundJobStatusNames.Succeeded : BackgroundJobStatusNames.Failed;
        job.LastCompletedAtUtc = completedAt;
        job.LastDurationMs = job.LastStartedAtUtc is null
            ? null
            : (long)(completedAt - job.LastStartedAtUtc.Value).TotalMilliseconds;
        job.LastDiffSummary = diffSummary;
        job.LastError = error;
        job.IntervalSeconds = intervalSeconds;
        job.NextRunAtUtc = completedAt.AddSeconds(Math.Max(5, intervalSeconds));
        await db.SaveChangesAsync(cancellationToken);
    }

    private async Task EnsureJobAsync(
        string jobKey,
        string displayName,
        int intervalSeconds,
        CancellationToken cancellationToken)
    {
        if (await db.BackgroundJobs.AnyAsync(x => x.JobKey == jobKey, cancellationToken))
        {
            return;
        }

        db.BackgroundJobs.Add(new BackgroundJobEntity
        {
            JobKey = jobKey,
            DisplayName = displayName,
            Status = BackgroundJobStatusNames.Idle,
            IntervalSeconds = intervalSeconds,
        });
        await db.SaveChangesAsync(cancellationToken);
    }

    private async Task<BackgroundJobEntity> GetOrCreateAsync(string jobKey, CancellationToken cancellationToken)
    {
        var job = await db.BackgroundJobs.SingleOrDefaultAsync(x => x.JobKey == jobKey, cancellationToken);
        if (job is not null)
        {
            return job;
        }

        await EnsureJobsAsync(cancellationToken);
        return await db.BackgroundJobs.SingleAsync(x => x.JobKey == jobKey, cancellationToken);
    }
}

public static class BackgroundJobStatusNames
{
    public const string Idle = "idle";
    public const string Running = "running";
    public const string Succeeded = "succeeded";
    public const string Failed = "failed";
}
