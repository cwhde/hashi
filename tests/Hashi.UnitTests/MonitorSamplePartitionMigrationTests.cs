using Xunit;

namespace Hashi.UnitTests;

public sealed class MonitorSamplePartitionMigrationTests
{
    [Fact]
    public void PartitionMonitorSamplesRaw_migration_uses_native_postgresql_partitioning()
    {
        var migrationPath = Path.GetFullPath(Path.Combine(
            AppContext.BaseDirectory,
            "..", "..", "..", "..", "..",
            "src", "Hashi.Infrastructure", "Persistence", "Migrations",
            "20260526144201_PartitionMonitorSamplesRaw.cs"));

        Assert.True(File.Exists(migrationPath), $"Migration file not found: {migrationPath}");

        var sql = File.ReadAllText(migrationPath);
        Assert.Contains("PARTITION BY RANGE (partition_date)", sql);
        Assert.Contains("CREATE TABLE IF NOT EXISTS %I PARTITION OF monitor_samples_raw", sql);
        Assert.Contains("DROP TABLE monitor_samples_legacy", sql);
    }
}
