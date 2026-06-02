using Hashi.Contracts.Api;
using Hashi.Core.Connections;
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
    private static readonly (string FileName, Func<Hashi.Core.Traefik.TraefikDynamicFiles, string> Selector)[] DynamicFileMap =
    [
        ("00-hashi-core.yml", f => f.CoreYaml),
        ("10-hashi-http-resources.yml", f => f.HttpResourcesYaml),
        ("20-hashi-stream-resources.yml", f => f.StreamResourcesYaml),
        ("30-user-middlewares.yml", f => f.UserMiddlewaresYaml),
        ("40-hashi-security.yml", f => f.SecurityYaml),
        ("90-hashi-health.yml", f => f.HealthYaml),
    ];

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
        var traefik = TestPlatformHelpers.CreateTraefikPlatform(db);
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
        var sync = TestPlatformHelpers.CreateTraefikSync(db, ssh, vault);
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

    [Fact]
    public async Task Apply_writes_selected_dns_provider_credentials_for_unattended_acme()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        await using var db = new HashiDbContext(options);
        var serviceSync = ReadyServiceSyncVault();
        var dnsConnectionId = await SeedDnsConnectionAsync(db, serviceSync);
        db.AppSettings.Add(new AppSettingsEntity
        {
            AcmeEmail = "admin@example.com",
            AcmeDnsProviderConnectionId = dnsConnectionId,
        });
        await db.SaveChangesAsync();
        var ssh = new FakeSshRemoteExecutor();
        var sync = TestPlatformHelpers.CreateTraefikSync(db, ssh, serviceSync: serviceSync);

        var result = await sync.ApplyAsync(new TraefikApplyRequest(
            Guid.NewGuid(),
            "10.0.0.1",
            22,
            "root",
            "password",
            "secret",
            null,
            null));

        Assert.True(result.Succeeded, result.Message);
        Assert.True(ssh.WrittenFiles.TryGetValue("/etc/hashi/traefik/acme.env", out var envBytes));
        Assert.Equal("HETZNER_API_KEY='dns-token'\n", System.Text.Encoding.UTF8.GetString(envBytes));
        Assert.Contains("provider: hetzner", System.Text.Encoding.UTF8.GetString(ssh.WrittenFiles["/etc/hashi/traefik/traefik.yml"]));
        Assert.Contains(ssh.Commands, x => x.Contains("EnvironmentFile=-/etc/hashi/traefik/acme.env", StringComparison.Ordinal));
        Assert.Contains(ssh.Commands, x => x.Contains("set -a && . '/tmp/hashi-traefik-", StringComparison.Ordinal));
        Assert.DoesNotContain(ssh.Commands, x => x.Contains("dns-token", StringComparison.Ordinal));
    }

    [Fact]
    public async Task Apply_rejects_missing_dns_provider_binding_before_remote_write()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        await using var db = new HashiDbContext(options);
        db.AppSettings.Add(new AppSettingsEntity
        {
            AcmeEmail = "admin@example.com",
        });
        await db.SaveChangesAsync();
        var ssh = new FakeSshRemoteExecutor();
        var sync = TestPlatformHelpers.CreateTraefikSync(db, ssh);

        var result = await sync.ApplyAsync(new TraefikApplyRequest(
            Guid.NewGuid(),
            "10.0.0.1",
            22,
            "root",
            "password",
            "secret",
            null,
            null));

        Assert.False(result.Succeeded);
        Assert.Contains("DNS provider", result.Message ?? string.Empty, StringComparison.OrdinalIgnoreCase);
        Assert.Equal(0, ssh.WriteCount);
        Assert.Empty(ssh.Commands);
    }

    [Fact]
    public async Task Apply_rejects_disabled_dns_provider_binding_before_remote_write()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        await using var db = new HashiDbContext(options);
        var serviceSync = ReadyServiceSyncVault();
        var dnsConnectionId = await SeedDnsConnectionAsync(db, serviceSync, enabled: false);
        db.AppSettings.Add(new AppSettingsEntity
        {
            AcmeEmail = "admin@example.com",
            AcmeDnsProviderConnectionId = dnsConnectionId,
        });
        await db.SaveChangesAsync();
        var ssh = new FakeSshRemoteExecutor();
        var sync = TestPlatformHelpers.CreateTraefikSync(db, ssh, serviceSync: serviceSync);

        var result = await sync.ApplyAsync(new TraefikApplyRequest(
            Guid.NewGuid(),
            "10.0.0.1",
            22,
            "root",
            "password",
            "secret",
            null,
            null));

        Assert.False(result.Succeeded);
        Assert.Contains("disabled", result.Message ?? string.Empty, StringComparison.OrdinalIgnoreCase);
        Assert.Equal(0, ssh.WriteCount);
        Assert.Empty(ssh.Commands);
    }

    [Fact]
    public async Task Apply_rejects_unsupported_dns_provider_binding_before_remote_write()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        await using var db = new HashiDbContext(options);
        var serviceSync = ReadyServiceSyncVault();
        var dnsConnectionId = await SeedDnsConnectionAsync(db, serviceSync, provider: "cloudflare");
        db.AppSettings.Add(new AppSettingsEntity
        {
            AcmeEmail = "admin@example.com",
            AcmeDnsProviderConnectionId = dnsConnectionId,
        });
        await db.SaveChangesAsync();
        var ssh = new FakeSshRemoteExecutor();
        var sync = TestPlatformHelpers.CreateTraefikSync(db, ssh, serviceSync: serviceSync);

        var result = await sync.ApplyAsync(new TraefikApplyRequest(
            Guid.NewGuid(),
            "10.0.0.1",
            22,
            "root",
            "password",
            "secret",
            null,
            null));

        Assert.False(result.Succeeded);
        Assert.Contains("not supported", result.Message ?? string.Empty, StringComparison.OrdinalIgnoreCase);
        Assert.Equal(0, ssh.WriteCount);
        Assert.Empty(ssh.Commands);
    }

    [Fact]
    public async Task Apply_rejects_dns_provider_without_service_sync_credentials_before_remote_write()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        await using var db = new HashiDbContext(options);
        var serviceSync = new ServiceSyncVaultState();
        var dnsConnectionId = await SeedDnsConnectionAsync(db, serviceSync);
        db.AppSettings.Add(new AppSettingsEntity
        {
            AcmeEmail = "admin@example.com",
            AcmeDnsProviderConnectionId = dnsConnectionId,
        });
        await db.SaveChangesAsync();
        var ssh = new FakeSshRemoteExecutor();
        var sync = TestPlatformHelpers.CreateTraefikSync(db, ssh, serviceSync: serviceSync);

        var result = await sync.ApplyAsync(new TraefikApplyRequest(
            Guid.NewGuid(),
            "10.0.0.1",
            22,
            "root",
            "password",
            "secret",
            null,
            null));

        Assert.False(result.Succeeded);
        Assert.Contains("service sync", result.Message ?? string.Empty, StringComparison.OrdinalIgnoreCase);
        Assert.Equal(0, ssh.WriteCount);
        Assert.Empty(ssh.Commands);
    }

    [Fact]
    public async Task Apply_rejects_invalid_render_before_remote_write()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        await using var db = new HashiDbContext(options);
        db.TraefikUserMiddlewares.Add(new TraefikUserMiddlewareEntity
        {
            Yaml = "http:\n  middlewares:\n    broken: [",
        });
        await db.SaveChangesAsync();
        var ssh = new FakeSshRemoteExecutor();
        var sync = TestPlatformHelpers.CreateTraefikSync(db, ssh);

        var result = await sync.ApplyAsync(new TraefikApplyRequest(
            Guid.NewGuid(),
            "10.0.0.1",
            22,
            "root",
            "password",
            "secret",
            null,
            null));

        Assert.False(result.Succeeded);
        Assert.Equal(0, ssh.WriteCount);
        Assert.Empty(ssh.Commands);
        Assert.Contains("YAML validation", result.Message ?? string.Empty, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public async Task Apply_validates_staged_config_before_active_write()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        await using var db = new HashiDbContext(options);
        var ssh = new FakeSshRemoteExecutor();
        ssh.CommandResults.Enqueue(new RemoteCommandResult(true, string.Empty, null));
        ssh.CommandResults.Enqueue(new RemoteCommandResult(true, string.Empty, null));
        ssh.CommandResults.Enqueue(new RemoteCommandResult(false, "bad config", "bad config"));
        var sync = TestPlatformHelpers.CreateTraefikSync(db, ssh);

        var result = await sync.ApplyAsync(new TraefikApplyRequest(
            Guid.NewGuid(),
            "10.0.0.1",
            22,
            "root",
            "password",
            "secret",
            null,
            null));

        Assert.False(result.Succeeded);
        Assert.Contains("bad config", result.Message ?? string.Empty, StringComparison.Ordinal);
        Assert.Contains(ssh.Commands, x => x.Contains("traefik check --configFile", StringComparison.Ordinal));
        Assert.DoesNotContain(ssh.WrittenFiles.Keys, x => x.StartsWith("/etc/hashi/traefik/", StringComparison.Ordinal));
        Assert.All(ssh.WrittenFiles.Keys, x => Assert.StartsWith("/tmp/hashi-traefik-", x, StringComparison.Ordinal));
    }

    [Fact]
    public async Task Apply_skips_active_writes_when_remote_content_already_matches()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        await using var db = new HashiDbContext(options);
        var traefik = TestPlatformHelpers.CreateTraefikPlatform(db);
        var render = await traefik.RenderAsync();
        var ssh = new FakeSshRemoteExecutor();
        ssh.ReadFiles["/etc/hashi/traefik/traefik.yml"] = System.Text.Encoding.UTF8.GetBytes(render.StaticConfigYaml);
        foreach (var (fileName, selector) in DynamicFileMap)
        {
            ssh.ReadFiles[$"/etc/hashi/traefik/dynamic/{fileName}"] = System.Text.Encoding.UTF8.GetBytes(selector(render.DynamicFiles));
        }

        var sync = TestPlatformHelpers.CreateTraefikSync(db, ssh);

        var result = await sync.ApplyAsync(new TraefikApplyRequest(
            Guid.NewGuid(),
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
        Assert.DoesNotContain(ssh.Commands, x => x.Contains("traefik check", StringComparison.Ordinal));
    }

    [Fact]
    public async Task Install_fails_when_package_install_command_fails()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        await using var db = new HashiDbContext(options);
        var ssh = new FakeSshRemoteExecutor
        {
            CommandResult = new RemoteCommandResult(false, "apt failed", "apt failed"),
        };
        var sync = TestPlatformHelpers.CreateTraefikSync(db, ssh);

        var result = await sync.InstallAsync(new TraefikInstallRequest(
            Guid.NewGuid(),
            "10.0.0.1",
            22,
            "root",
            "password",
            "secret",
            null,
            null));

        Assert.False(result.Succeeded);
        Assert.DoesNotContain("install -y traefik || apt-get install -y traefik2 || true", ssh.Commands.Single(), StringComparison.Ordinal);
        Assert.DoesNotContain("apk add --no-cache traefik || true", ssh.Commands.Single(), StringComparison.Ordinal);
    }

    private static ServiceSyncVaultState ReadyServiceSyncVault()
    {
        var serviceSync = new ServiceSyncVaultState();
        serviceSync.Initialize([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32]);
        return serviceSync;
    }

    private static async Task<Guid> SeedDnsConnectionAsync(
        HashiDbContext db,
        ServiceSyncVaultState serviceSync,
        bool enabled = true,
        string provider = DnsProviderTypeNames.Hetzner)
    {
        var vault = new VaultSessionState();
        vault.Unlock(new byte[32]);
        var secrets = new SecretRecordService(db, vault, serviceSync);
        var token = await secrets.StoreAsync(
            Hashi.Core.Auth.SecretPurpose.DnsProviderToken,
            "DNS provider",
            System.Text.Encoding.UTF8.GetBytes("dns-token"),
            serviceSyncEligible: true);
        var connectionId = Guid.NewGuid();
        db.Connections.Add(new ConnectionEntity
        {
            Id = connectionId,
            Name = "dns",
            Type = ConnectionTypeNames.DnsProvider,
            Enabled = enabled,
            SecretId = token.Id,
            SettingsJson = $$"""{"provider":"{{provider}}","zoneName":"example.com","defaultTtl":3600}""",
        });
        await db.SaveChangesAsync();
        return connectionId;
    }
}
