using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Npgsql;
using Xunit;

namespace Hashi.IntegrationTests;

[Collection(PostgresIntegrationCollection.Name)]
public sealed class MonitorSamplePartitionTests(PostgresIntegrationFixture fixture) : IAsyncLifetime
{
    private string? _connectionString;

    public async Task InitializeAsync()
    {
        if (!fixture.IsAvailable)
        {
            return;
        }

        _connectionString = await fixture.CreateDatabaseAsync();
    }

    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task MonitorSamples_partition_drop_removes_only_target_range()
    {
        if (!fixture.IsAvailable || _connectionString is null)
        {
            return;
        }

        await using var factory = IntegrationTestApp.CreateFactory(_connectionString);
        using var scope = factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();

        var endpointId = Guid.NewGuid();
        db.MonitorEndpoints.Add(new MonitorEndpointEntity
        {
            Id = endpointId,
            Name = "partition-test",
            Url = "http://127.0.0.1:1",
            CheckType = "http",
            Status = "unknown",
            Enabled = true,
        });
        await db.SaveChangesAsync();

        var oldWeek = MonitorSamplePartitionService.GetWeekStart(DateOnly.FromDateTime(DateTime.UtcNow.AddDays(-21)));
        var keepWeek = MonitorSamplePartitionService.GetWeekStart(DateOnly.FromDateTime(DateTime.UtcNow.AddDays(-7)));
        var recentWeek = MonitorSamplePartitionService.GetWeekStart(DateOnly.FromDateTime(DateTime.UtcNow));

        await MonitorSamplePartitionService.EnsurePartitionAsync(db, oldWeek);
        await MonitorSamplePartitionService.EnsurePartitionAsync(db, keepWeek);
        await MonitorSamplePartitionService.EnsurePartitionAsync(db, recentWeek);

        var oldSampleId = Guid.NewGuid();
        var keepSampleId = Guid.NewGuid();
        var recentSampleId = Guid.NewGuid();

        db.MonitorSamples.AddRange(
            CreateSample(oldSampleId, endpointId, oldWeek.AddDays(1)),
            CreateSample(keepSampleId, endpointId, keepWeek.AddDays(2)),
            CreateSample(recentSampleId, endpointId, recentWeek.AddDays(1)));
        await db.SaveChangesAsync();

        var retentionDays = 14;
        var dropped = await MonitorSamplePartitionService.DropExpiredPartitionsAsync(db, retentionDays);

        Assert.True(dropped >= 1);
        Assert.False(await db.MonitorSamples.AnyAsync(x => x.Id == oldSampleId));
        Assert.True(await db.MonitorSamples.AnyAsync(x => x.Id == keepSampleId));
        Assert.True(await db.MonitorSamples.AnyAsync(x => x.Id == recentSampleId));

        await using var connection = new NpgsqlConnection(_connectionString);
        await connection.OpenAsync();
        await using var partitionCheck = new NpgsqlCommand(
            """
            SELECT c.relname
            FROM pg_inherits i
            JOIN pg_class c ON c.oid = i.inhrelid
            JOIN pg_class p ON p.oid = i.inhparent
            WHERE p.relname = 'monitor_samples_raw'
              AND c.relname = @partitionName
            """,
            connection);
        partitionCheck.Parameters.AddWithValue(
            "partitionName",
            MonitorSamplePartitionService.GetPartitionName(oldWeek));
        var oldPartitionExists = await partitionCheck.ExecuteScalarAsync();
        Assert.Null(oldPartitionExists);

        partitionCheck.Parameters["partitionName"].Value =
            MonitorSamplePartitionService.GetPartitionName(keepWeek);
        var keepPartitionExists = await partitionCheck.ExecuteScalarAsync();
        Assert.NotNull(keepPartitionExists);
    }

    private static MonitorSampleEntity CreateSample(Guid id, Guid endpointId, DateOnly partitionDate)
        => new()
        {
            Id = id,
            MonitorEndpointId = endpointId,
            PartitionDate = partitionDate,
            CheckedAtUtc = partitionDate.ToDateTime(TimeOnly.MinValue, DateTimeKind.Utc),
            Status = "up",
            LatencyMs = 10,
        };
}
