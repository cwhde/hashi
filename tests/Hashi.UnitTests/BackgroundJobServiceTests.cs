using System.Net;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;
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

    [Fact]
    public async Task Security_addendum_worker_executes_registered_jobs()
    {
        var services = new ServiceCollection();
        var databaseName = Guid.NewGuid().ToString();
        services.AddDbContext<HashiDbContext>(options =>
            options.UseInMemoryDatabase(databaseName));
        services.AddScoped<BackgroundJobService>();
        services.AddScoped<AuditService>();
        services.AddScoped<BlocklistParser>();
        services.AddScoped<BlocklistSafeHttpFetcher>();
        services.AddSingleton<IBlocklistDnsResolver>(_ =>
        {
            var resolver = new FakeDnsResolver();
            resolver.Add("feed.example", IPAddress.Parse("203.0.113.10"));
            return resolver;
        });
        services.AddScoped<IBlocklistHttpTransport, FakeTransport>();
        services.AddScoped<BlocklistSourceManagementService>(provider => new BlocklistSourceManagementService(
            provider.GetRequiredService<HashiDbContext>(),
            provider.GetRequiredService<BlocklistSafeHttpFetcher>(),
            provider.GetRequiredService<BlocklistParser>(),
            provider.GetRequiredService<AuditService>(),
            NullLogger<BlocklistSourceManagementService>.Instance));
        services.AddScoped<SecurityMaintenanceService>();
        await using var provider = services.BuildServiceProvider();

        using (var scope = provider.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
            await scope.ServiceProvider.GetRequiredService<BackgroundJobService>().EnsureJobsAsync();
            var subject = new SecuritySubjectEntity
            {
                SubjectType = SecuritySubjectTypeNames.Ip,
                SubjectValue = "203.0.113.5",
                NormalizedValue = "203.0.113.5",
                CurrentState = SecuritySubjectStateNames.SoftBlocked,
            };
            db.SecuritySubjects.Add(subject);
            db.SecuritySubjectStates.Add(new SecuritySubjectStateEntity
            {
                SecuritySubjectId = subject.Id,
                SoftBlockedUntilUtc = DateTimeOffset.UtcNow.AddMinutes(-1),
                ChallengeRequired = true,
                ChallengeRequiredSinceUtc = DateTimeOffset.UtcNow.AddHours(-2),
            });
            db.ManualSecurityEntries.Add(new ManualSecurityEntryEntity
            {
                SubjectType = SecuritySubjectTypeNames.Ip,
                SubjectValue = "203.0.113.5",
                NormalizedValue = "203.0.113.5",
                EntryType = ManualSecurityEntryTypeNames.Block,
                Enabled = true,
                IsPermanent = false,
                ExpiresAtUtc = DateTimeOffset.UtcNow.AddMinutes(-1),
            });
            await db.SaveChangesAsync();
        }

        var worker = new SecurityAddendumJobWorker(
            provider.GetRequiredService<IServiceScopeFactory>(),
            NullLogger<SecurityAddendumJobWorker>.Instance);
        var results = await worker.ProcessOnceAsync();

        Assert.Contains(results, x => x.JobKey == BackgroundJobKeys.BlocklistFetch && x.Ran && x.Succeeded);
        Assert.Contains(results, x => x.JobKey == BackgroundJobKeys.BlockExpiry && x.Ran && x.Succeeded);
        Assert.Contains(results, x => x.JobKey == BackgroundJobKeys.ChallengeCleanup && x.Ran && x.Succeeded);

        using var verifyScope = provider.CreateScope();
        var verifyDb = verifyScope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var jobs = await verifyDb.BackgroundJobs.AsNoTracking().ToDictionaryAsync(x => x.JobKey);
        Assert.Equal(BackgroundJobStatusNames.Succeeded, jobs[BackgroundJobKeys.BlockExpiry].Status);
        Assert.NotNull(jobs[BackgroundJobKeys.BlockExpiry].LastCompletedAtUtc);
        Assert.NotNull(jobs[BackgroundJobKeys.BlockExpiry].NextRunAtUtc);
        Assert.False(await verifyDb.ManualSecurityEntries.AnyAsync(x => x.Enabled));
        Assert.Equal(SecuritySubjectStateNames.Observed, (await verifyDb.SecuritySubjects.SingleAsync()).CurrentState);
        Assert.False((await verifyDb.SecuritySubjectStates.SingleAsync()).ChallengeRequired);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private sealed class FakeDnsResolver : IBlocklistDnsResolver
    {
        private readonly Dictionary<string, IReadOnlyList<IPAddress>> addresses = new(StringComparer.OrdinalIgnoreCase);

        public void Add(string host, params IPAddress[] values)
            => addresses[host] = values;

        public Task<IReadOnlyList<IPAddress>> ResolveAsync(string host, CancellationToken cancellationToken)
            => Task.FromResult(addresses.TryGetValue(host, out var values) ? values : [IPAddress.Parse("203.0.113.10")]);
    }

    private sealed class FakeTransport : IBlocklistHttpTransport
    {
        public Task<BlocklistHttpTransportResponse> GetAsync(
            Uri uri,
            IReadOnlyDictionary<string, string> headers,
            int timeoutSeconds,
            int maxBytes,
            CancellationToken cancellationToken)
            => Task.FromResult(new BlocklistHttpTransportResponse(200, new Dictionary<string, string>(), string.Empty));
    }
}
