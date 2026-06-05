using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Platform;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class BackgroundJobServiceTests
{
    [Fact]
    public async Task EnsureJobs_registers_addendum_background_jobs()
    {
        await using var db = CreateDb();
        var service = new BackgroundJobService(db);

        await service.EnsureJobsAsync();

        var jobs = await db.BackgroundJobs.AsNoTracking().ToDictionaryAsync(x => x.JobKey);
        foreach (var key in new[]
        {
            BackgroundJobKeys.BlocklistFetch,
            BackgroundJobKeys.SecurityBucketAggregation,
            BackgroundJobKeys.BlockExpiry,
            BackgroundJobKeys.InternalAgentDnsSync,
            BackgroundJobKeys.ChallengeCleanup,
        })
        {
            Assert.True(jobs.ContainsKey(key), $"Missing background job '{key}'.");
            Assert.True(jobs[key].IntervalSeconds >= 300);
            Assert.Equal(BackgroundJobStatusNames.Idle, jobs[key].Status);
        }
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
