using Hashi.Contracts.Api;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Hashi.UnitTests.Fakes;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class TraefikSyncSafetyTests
{
    [Fact]
    public async Task Apply_skips_remote_writes_when_content_hash_unchanged()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        await using var db = new HashiDbContext(options);
        db.Resources.Add(new ResourceEntity
        {
            Name = "App",
            Slug = "app",
            Kind = "https",
            Enabled = true,
            Domain = "app.example.com",
            TargetScheme = "http",
            TargetHost = "10.0.0.2",
            TargetPort = 8080,
        });
        await db.SaveChangesAsync();
        var connectionId = Guid.NewGuid();
        var settings = new AppSettingsService(db);
        await settings.GetOrCreateAsync();
        var traefik = new TraefikPlatformService(db, settings);
        var render = await traefik.RenderAsync();
        db.TraefikHostStates.Add(new TraefikHostStateEntity
        {
            ConnectionId = connectionId,
            LastAppliedContentHash = render.ContentHash,
        });
        await db.SaveChangesAsync();
        var ssh = new FakeSshRemoteExecutor();
        var vault = new VaultSessionState();
        vault.Unlock(new byte[32]);
        var sync = new TraefikSyncService(db, ssh, traefik, new SecretRecordService(db, vault, new ServiceSyncVaultState()), new AuditService(db));
        var result = await sync.ApplyAsync(new TraefikApplyRequest(
            connectionId,
            "10.0.0.1",
            22,
            "root",
            "password",
            "secret",
            null,
            null));

        Assert.True(result.Succeeded);
        Assert.True(result.Skipped);
        Assert.Equal(0, ssh.WriteCount);
        Assert.Contains("unchanged", result.Message ?? string.Empty, StringComparison.OrdinalIgnoreCase);
    }
}
