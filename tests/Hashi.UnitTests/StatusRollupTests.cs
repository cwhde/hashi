using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Hashi.Infrastructure.Platform;
using Hashi.Core.Hosting;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class StatusRollupTests
{
    [Fact]
    public async Task PublicStatus_includes_only_enabled_public_selected_endpoints()
    {
        await using var db = CreateDb();
        db.MonitorEndpoints.AddRange(
            new MonitorEndpointEntity
            {
                Name = "Private",
                Url = "https://private.example.com/",
                CheckType = "https",
                Enabled = true,
                PublicStatusEnabled = false,
                Status = "up",
            },
            new MonitorEndpointEntity
            {
                Name = "Public",
                Url = "https://public.example.com/",
                CheckType = "https",
                Enabled = true,
                PublicStatusEnabled = true,
                Status = "up",
                LastLatencyMs = 50,
            },
            new MonitorEndpointEntity
            {
                Name = "Disabled Public",
                Url = "https://disabled.example.com/",
                CheckType = "https",
                Enabled = false,
                PublicStatusEnabled = true,
                Status = "down",
            });
        await db.SaveChangesAsync();

        var service = new MonitoringService(db, new AppSettingsService(db), new HashiInternalUrlResolver(new HashiPortOptions()));
        var status = await service.PublicStatusAsync();

        var item = Assert.Single(status);
        Assert.Equal("Public", item.Name);
        Assert.Equal("Up", item.Status);
        Assert.Equal(50, item.LastLatencyMs);
    }

    [Fact]
    public async Task PublicSummary_counts_by_status_category()
    {
        await using var db = CreateDb();
        db.MonitorEndpoints.AddRange(
            new MonitorEndpointEntity
            {
                Name = "Up 1",
                Url = "https://up1.example.com/",
                CheckType = "https",
                Enabled = true,
                PublicStatusEnabled = true,
                Status = "up",
            },
            new MonitorEndpointEntity
            {
                Name = "Up 2",
                Url = "https://up2.example.com/",
                CheckType = "https",
                Enabled = true,
                PublicStatusEnabled = true,
                Status = "up",
            },
            new MonitorEndpointEntity
            {
                Name = "Degraded",
                Url = "https://degraded.example.com/",
                CheckType = "https",
                Enabled = true,
                PublicStatusEnabled = true,
                Status = "degraded",
            },
            new MonitorEndpointEntity
            {
                Name = "Down",
                Url = "https://down.example.com/",
                CheckType = "https",
                Enabled = true,
                PublicStatusEnabled = true,
                Status = "down",
            },
            new MonitorEndpointEntity
            {
                Name = "Unknown",
                Url = "https://unknown.example.com/",
                CheckType = "https",
                Enabled = true,
                PublicStatusEnabled = true,
                Status = "unknown",
            },
            new MonitorEndpointEntity
            {
                Name = "Private",
                Url = "https://private.example.com/",
                CheckType = "https",
                Enabled = true,
                PublicStatusEnabled = false,
                Status = "up",
            });
        await db.SaveChangesAsync();

        var service = new MonitoringService(db, new AppSettingsService(db), new HashiInternalUrlResolver(new HashiPortOptions()));
        var summary = await service.PublicSummaryAsync();

        Assert.Equal(5, summary.TotalEndpoints);
        Assert.Equal(2, summary.UpCount);
        Assert.Equal(1, summary.DegradedCount);
        Assert.Equal(1, summary.DownCount);
    }

    [Fact]
    public async Task PublicStatusMaps_status_to_display_status()
    {
        await using var db = CreateDb();
        db.MonitorEndpoints.AddRange(
            new MonitorEndpointEntity
            {
                Name = "Up",
                Url = "https://up.example.com/",
                CheckType = "https",
                Enabled = true,
                PublicStatusEnabled = true,
                Status = "up",
            },
            new MonitorEndpointEntity
            {
                Name = "Degraded",
                Url = "https://degraded.example.com/",
                CheckType = "https",
                Enabled = true,
                PublicStatusEnabled = true,
                Status = "degraded",
            },
            new MonitorEndpointEntity
            {
                Name = "Down",
                Url = "https://down.example.com/",
                CheckType = "https",
                Enabled = true,
                PublicStatusEnabled = true,
                Status = "down",
            },
            new MonitorEndpointEntity
            {
                Name = "Unknown",
                Url = "https://unknown.example.com/",
                CheckType = "https",
                Enabled = true,
                PublicStatusEnabled = true,
                Status = "unknown",
            });
        await db.SaveChangesAsync();

        var service = new MonitoringService(db, new AppSettingsService(db), new HashiInternalUrlResolver(new HashiPortOptions()));
        var status = await service.PublicStatusAsync();

        Assert.Contains(status, x => x.Status == "Up");
        Assert.Contains(status, x => x.Status == "Degraded");
        Assert.Contains(status, x => x.Status == "Down");
        Assert.Contains(status, x => x.Status == "Unknown");
    }

    [Fact]
    public async Task PublicStatus_includes_last_checked_timestamp()
    {
        await using var db = CreateDb();
        var checkedAt = DateTimeOffset.UtcNow.AddMinutes(-5);
        db.MonitorEndpoints.Add(new MonitorEndpointEntity
        {
            Name = "Checked",
            Url = "https://checked.example.com/",
            CheckType = "https",
            Enabled = true,
            PublicStatusEnabled = true,
            Status = "up",
            LastCheckedAtUtc = checkedAt,
        });
        await db.SaveChangesAsync();

        var service = new MonitoringService(db, new AppSettingsService(db), new HashiInternalUrlResolver(new HashiPortOptions()));
        var status = await service.PublicStatusAsync();

        var item = Assert.Single(status);
        Assert.NotNull(item.LastCheckedAtUtc);
    }

    [Fact]
    public async Task PublicStatus_excludes_endpoints_without_public_status_flag()
    {
        await using var db = CreateDb();
        db.MonitorEndpoints.AddRange(
            new MonitorEndpointEntity
            {
                Name = "Hidden",
                Url = "https://hidden.example.com/",
                CheckType = "https",
                Enabled = true,
                PublicStatusEnabled = false,
                Status = "up",
            },
            new MonitorEndpointEntity
            {
                Name = "Visible",
                Url = "https://visible.example.com/",
                CheckType = "https",
                Enabled = true,
                PublicStatusEnabled = true,
                Status = "up",
            });
        await db.SaveChangesAsync();

        var service = new MonitoringService(db, new AppSettingsService(db), new HashiInternalUrlResolver(new HashiPortOptions()));
        var status = await service.PublicStatusAsync();

        Assert.Single(status);
        Assert.Contains(status, x => x.Name == "Visible");
        Assert.DoesNotContain(status, x => x.Name == "Hidden");
    }

    [Fact]
    public async Task PublicSummary_returns_zero_counts_when_no_public_endpoints()
    {
        await using var db = CreateDb();
        db.MonitorEndpoints.Add(new MonitorEndpointEntity
        {
            Name = "Private",
            Url = "https://private.example.com/",
            CheckType = "https",
            Enabled = true,
            PublicStatusEnabled = false,
            Status = "up",
        });
        await db.SaveChangesAsync();

        var service = new MonitoringService(db, new AppSettingsService(db), new HashiInternalUrlResolver(new HashiPortOptions()));
        var summary = await service.PublicSummaryAsync();

        Assert.Equal(0, summary.TotalEndpoints);
        Assert.Equal(0, summary.UpCount);
        Assert.Equal(0, summary.DegradedCount);
        Assert.Equal(0, summary.DownCount);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
