using Hashi.Core.Auth;
using Hashi.Core.Dns;
using Hashi.Core.Sync;
using Hashi.Infrastructure.Crypto;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Providers.Dns;
using Hashi.Infrastructure.Services;
using Hashi.UnitTests.Fakes;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;
using Xunit;

namespace Hashi.UnitTests;

public sealed class PulseAgentServiceTests
{
    [Fact]
    public async Task RevokeAgent_rejects_subsequent_heartbeat()
    {
        await using var db = CreateDb();
        var service = CreateService(db);
        var created = await service.CreateAgentAsync(new Hashi.Contracts.Api.CreatePulseAgentRequest("edge-1"));
        await service.RevokeAgentAsync(created.Id);

        var accepted = await service.AcceptHeartbeatAsync(
            created.Id,
            new Hashi.Contracts.Api.PulseHeartbeatAuthRequest(created.Token, "0.1.0", "host", ["10.0.0.5"]),
            "203.0.113.10");

        Assert.False(accepted);
    }

    [Fact]
    public void InstallRenderer_uses_placeholder_token_only()
    {
        var token = Convert.ToBase64String(System.Security.Cryptography.RandomNumberGenerator.GetBytes(32));
        var rendered = PulseInstallRenderer.Render("https://hashi.example.com", Guid.NewGuid());

        Assert.Contains("<PULSE_TOKEN>", rendered.LinuxInstallScript);
        Assert.Contains("<PULSE_TOKEN>", rendered.DockerRunCommand);
        Assert.DoesNotContain(token, rendered.LinuxInstallScript);
        Assert.DoesNotContain(token, rendered.DockerRunCommand);
        Assert.DoesNotContain("?token=", rendered.LinuxInstallScript, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("&token=", rendered.LinuxInstallScript, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("?token=", rendered.DockerRunCommand, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("&token=", rendered.DockerRunCommand, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public async Task Token_lifecycle_writes_redacted_audit_events()
    {
        await using var db = CreateDb();
        var service = CreateService(db);

        var created = await service.CreateAgentAsync(new Hashi.Contracts.Api.CreatePulseAgentRequest("edge-1"));
        var rotated = await service.RotateTokenAsync(created.Id);
        await service.RevokeAgentAsync(created.Id);

        Assert.NotNull(rotated);
        var events = await db.AuditEvents.OrderBy(x => x.CreatedAtUtc).ToListAsync();
        Assert.Contains(events, x => x.Category == "pulse" && x.Action == "agent_created" && x.SubjectId == created.Id.ToString());
        Assert.Contains(events, x => x.Category == "pulse" && x.Action == "token_rotated" && x.SubjectId == created.Id.ToString());
        Assert.Contains(events, x => x.Category == "pulse" && x.Action == "token_revoked" && x.SubjectId == created.Id.ToString());
        Assert.DoesNotContain(events, x =>
            (x.MetadataJson ?? string.Empty).Contains(created.Token, StringComparison.Ordinal)
            || (x.MetadataJson ?? string.Empty).Contains(rotated!.Token, StringComparison.Ordinal));
    }

    [Fact]
    public async Task Pulse_dns_sync_records_destructive_plan_as_pending_without_delete()
    {
        await using var db = CreateDb();
        var (providerFactory, vault) = await SeedDnsConnectionAsync(db);
        providerFactory.Provider.SeedZone(
            "zone-1",
            "example.com",
            new DnsRecordSnapshot("stale", "stale.example.com", DnsRecordType.A, "203.0.113.99", 3600, true));

        var service = CreateService(db, providerFactory, vault);
        var created = await service.CreateAgentAsync(new Hashi.Contracts.Api.CreatePulseAgentRequest("edge-1"));

        var accepted = await service.AcceptHeartbeatAsync(
            created.Id,
            new Hashi.Contracts.Api.PulseHeartbeatAuthRequest(created.Token, "0.1.0", "host", ["10.0.0.5"]),
            "203.0.113.10");

        Assert.True(accepted);
        var run = await db.SyncRuns.Include(x => x.Diffs).SingleAsync(x => x.Subsystem == "dns-pulse");
        Assert.Equal(SyncRunStatusNames.AwaitingConfirmation, run.Status);
        Assert.Equal(nameof(SyncRiskLevel.Destructive), run.RiskLevel);
        Assert.Contains(run.Diffs, x => x.ChangeKind == nameof(ProviderResultKind.Deleted));
        var records = await providerFactory.Provider.ListRecordsAsync("zone-1");
        Assert.Contains(records, x => x.Name == "stale.example.com");
        var agent = await db.PulseAgents.SingleAsync(x => x.Id == created.Id);
        Assert.NotNull(agent.DnsPendingAtUtc);
    }

    [Fact]
    public async Task Pulse_dns_sync_applies_safe_create_and_clears_pending_marker()
    {
        await using var db = CreateDb();
        var (providerFactory, vault) = await SeedDnsConnectionAsync(db);
        providerFactory.Provider.SeedZone("zone-1", "example.com");
        var agentId = Guid.NewGuid();
        db.PulseAgents.Add(new PulseAgentEntity
        {
            Id = agentId,
            Name = "edge-1",
            TokenHash = HashToken("pulse-token"),
            Status = "pending",
        });
        db.Resources.Add(new ResourceEntity
        {
            Name = "App",
            Slug = "app",
            Domain = "app.example.com",
            PulseAgentId = agentId,
        });
        await db.SaveChangesAsync();

        var service = CreateService(db, providerFactory, vault);
        var accepted = await service.AcceptHeartbeatAsync(
            agentId,
            new Hashi.Contracts.Api.PulseHeartbeatAuthRequest("pulse-token", "0.1.0", "host", ["10.0.0.5"]),
            "203.0.113.10");

        Assert.True(accepted);
        var run = await db.SyncRuns.Include(x => x.Diffs).SingleAsync(x => x.Subsystem == "dns-pulse");
        Assert.Equal(SyncRunStatusNames.Succeeded, run.Status);
        Assert.Contains(run.Diffs, x => x.ChangeKind == nameof(ProviderResultKind.Created));
        var records = await providerFactory.Provider.ListRecordsAsync("zone-1");
        Assert.Contains(records, x => x.Name == "app.example.com" && x.Value == "203.0.113.10");
        var agent = await db.PulseAgents.SingleAsync(x => x.Id == agentId);
        Assert.Null(agent.DnsPendingAtUtc);
    }

    private static PulseAgentService CreateService(
        HashiDbContext db,
        TestDnsProviderFactory? providerFactory = null,
        VaultSessionState? vault = null)
    {
        var audit = new AuditService(db);
        var secrets = new SecretRecordService(db, vault ?? new VaultSessionState(), new ServiceSyncVaultState());
        var httpClientFactory = new ServiceCollection().AddHttpClient().BuildServiceProvider().GetRequiredService<IHttpClientFactory>();
        IDnsProviderFactory dnsProviderFactory = providerFactory is null
            ? new DnsProviderFactory(httpClientFactory, NullLogger<HetznerDnsProvider>.Instance)
            : providerFactory;
        var dns = new DnsConnectionService(
            db,
            dnsProviderFactory,
            secrets,
            audit);
        return new PulseAgentService(db, dns, audit, NullLogger<PulseAgentService>.Instance);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private static async Task<(TestDnsProviderFactory ProviderFactory, VaultSessionState Vault)> SeedDnsConnectionAsync(HashiDbContext db)
    {
        db.AppSettings.Add(new AppSettingsEntity { RootDomain = "example.com" });
        var secretId = Guid.NewGuid();
        var rootKey = new byte[32];
        var dek = new byte[32];
        db.SecretRecords.Add(new SecretRecordEntity
        {
            Id = secretId,
            Purpose = SecretPurposeMapping.ToName(SecretPurpose.DnsProviderToken),
            Label = "DNS token",
            AdminWrappedDekBlob = AesGcmCipher.Encrypt(dek, rootKey).ToBlob(),
            CiphertextBlob = AesGcmCipher.Encrypt(System.Text.Encoding.UTF8.GetBytes("token"), dek).ToBlob(),
        });
        db.Connections.Add(new ConnectionEntity
        {
            Id = Guid.NewGuid(),
            Name = "hetzner",
            Type = ConnectionTypeNames.DnsProvider,
            Enabled = true,
            SecretId = secretId,
            SettingsJson = """{"provider":"hetzner","zoneName":"example.com","defaultTtl":3600}""",
        });
        db.DnsZones.Add(new DnsZoneEntity
        {
            ConnectionId = db.Connections.Local.Single().Id,
            ProviderZoneId = "zone-1",
            Name = "example.com",
            DefaultTtl = 3600,
        });
        await db.SaveChangesAsync();

        var vault = new VaultSessionState();
        vault.Unlock(rootKey);
        return (new TestDnsProviderFactory(), vault);
    }

    private static string HashToken(string token)
        => Convert.ToHexString(System.Security.Cryptography.SHA256.HashData(System.Text.Encoding.UTF8.GetBytes(token))).ToLowerInvariant();
}
