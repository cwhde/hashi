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
            Heartbeat(created.Token),
            "203.0.113.10");

        Assert.Equal(PulseHeartbeatAcceptResult.Unauthorized, accepted);
    }

    [Fact]
    public void InstallRenderer_uses_placeholder_token_only()
    {
        var token = Convert.ToBase64String(System.Security.Cryptography.RandomNumberGenerator.GetBytes(32));
        var rendered = PulseInstallRenderer.Render("https://hashi.example.com", Guid.NewGuid());

        Assert.Contains("<PULSE_TOKEN>", rendered.LinuxInstallScript);
        Assert.Contains("<PULSE_TOKEN>", rendered.DockerComposeSnippet);
        Assert.Contains("sudo env", rendered.LinuxInstallScript);
        Assert.DoesNotContain(token, rendered.LinuxInstallScript);
        Assert.DoesNotContain(token, rendered.DockerComposeSnippet);
        Assert.DoesNotContain("?token=", rendered.LinuxInstallScript, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("&token=", rendered.LinuxInstallScript, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("?token=", rendered.DockerComposeSnippet, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("&token=", rendered.DockerComposeSnippet, StringComparison.OrdinalIgnoreCase);
        Assert.Contains("services:", rendered.DockerComposeSnippet);
        Assert.Contains("hashi-pulse:", rendered.DockerComposeSnippet);
        Assert.DoesNotContain("docker run", rendered.DockerComposeSnippet, StringComparison.OrdinalIgnoreCase);

        var installer = File.ReadAllText(FindRepoFile("agents", "pulse", "install.sh"));
        Assert.Contains("detect_arch()", installer);
        Assert.Contains("hashi-pulse-linux-${arch}", installer);
        Assert.Contains("sha256_file()", installer);
        Assert.Contains("install_systemd_timer()", installer);
        Assert.Contains("install_cron()", installer);
        Assert.Contains("install -m 0600 -o root -g root", installer);
        Assert.Contains("ExecStart=${RUNNER}", installer);
        Assert.Contains("/etc/cron.d/${SERVICE_NAME}", installer);
        Assert.DoesNotContain("HASHI_PULSE_SOURCE_DIR", installer);
        Assert.DoesNotContain("go build", installer);
        Assert.DoesNotContain("Environment=HASHI_PULSE_TOKEN", installer);
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
    public async Task CreateAgent_persists_contract_fields()
    {
        await using var db = CreateDb();
        var service = CreateService(db);

        var created = await service.CreateAgentAsync(new Hashi.Contracts.Api.CreatePulseAgentRequest(
            "edge-1",
            "docker",
            ["heartbeat", "heartbeat", "status"],
            30));

        var agent = await db.PulseAgents.SingleAsync(x => x.Id == created.Id);
        var response = PulseAgentService.ToResponse(agent);
        Assert.Equal("docker", response.InstallType);
        Assert.Equal(["heartbeat", "status"], response.AllowedScopes);
        Assert.Equal(30, response.HeartbeatIntervalSeconds);
        Assert.Equal("pending", response.Status);
        Assert.Empty(response.LastPrivateIpv4Candidates);
        Assert.Empty(response.LastPrivateIpv6Candidates);
    }

    [Fact]
    public async Task Heartbeat_persists_timestamp_candidates_selected_interface_and_docker_metadata()
    {
        await using var db = CreateDb();
        var service = CreateService(db);
        var created = await service.CreateAgentAsync(new Hashi.Contracts.Api.CreatePulseAgentRequest("edge-1"));
        var timestamp = DateTimeOffset.UtcNow;

        var accepted = await service.AcceptHeartbeatAsync(
            created.Id,
            new Hashi.Contracts.Api.PulseHeartbeatAuthRequest(
                created.Token,
                "0.2.0",
                "host",
                ["10.0.0.5", "203.0.113.7", "10.0.0.5"],
                ["fd00::5", "2001:db8::5"],
                "eth0",
                "fd00::5",
                timestamp,
                new Hashi.Contracts.Api.PulseDockerMetadataRequest("container-1", "hashi-pulse:latest", "bridge")),
            "203.0.113.10");

        Assert.Equal(PulseHeartbeatAcceptResult.Accepted, accepted);
        var agent = await db.PulseAgents.SingleAsync(x => x.Id == created.Id);
        Assert.Equal("203.0.113.10", agent.LastPublicIp);
        Assert.Equal("fd00::5", agent.LastPrivateIp);
        Assert.Equal("fd00::5", agent.LastSelectedIp);
        Assert.Equal("eth0", agent.LastSelectedInterface);
        Assert.Equal("0.2.0", agent.LastAgentVersion);
        Assert.Equal("docker", agent.InstallType);
        Assert.Equal("online", agent.Status);
        Assert.Equal("""["10.0.0.5"]""", agent.LastPrivateIpv4CandidatesJson);
        Assert.Equal("""["fd00::5"]""", agent.LastPrivateIpv6CandidatesJson);
        Assert.Contains("container-1", agent.LastDockerMetadataJson);

        var heartbeat = await db.PulseHeartbeats.SingleAsync(x => x.PulseAgentId == created.Id);
        Assert.Equal(timestamp, heartbeat.AgentTimestampUtc);
        Assert.Equal("""["10.0.0.5"]""", heartbeat.PrivateIpv4CandidatesJson);
        Assert.Equal("""["fd00::5"]""", heartbeat.PrivateIpv6CandidatesJson);
        Assert.Equal("fd00::5", heartbeat.SelectedIp);
        Assert.Equal("eth0", heartbeat.SelectedInterface);
        Assert.Contains("hashi-pulse:latest", heartbeat.DockerMetadataJson);
    }

    [Fact]
    public async Task Heartbeat_rejects_stale_timestamp_without_persisting()
    {
        await using var db = CreateDb();
        var service = CreateService(db);
        var created = await service.CreateAgentAsync(new Hashi.Contracts.Api.CreatePulseAgentRequest("edge-1"));

        var accepted = await service.AcceptHeartbeatAsync(
            created.Id,
            new Hashi.Contracts.Api.PulseHeartbeatAuthRequest(
                created.Token,
                "0.2.0",
                "host",
                ["10.0.0.5"],
                [],
                "eth0",
                "10.0.0.5",
                DateTimeOffset.UtcNow.AddMinutes(-10),
                null),
            "203.0.113.10");

        Assert.Equal(PulseHeartbeatAcceptResult.InvalidTimestamp, accepted);
        Assert.False(await db.PulseHeartbeats.AnyAsync());
        var agent = await db.PulseAgents.SingleAsync(x => x.Id == created.Id);
        Assert.Null(agent.LastSeenAtUtc);
        Assert.Equal("pending", agent.Status);
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
            Heartbeat(created.Token),
            "203.0.113.10");

        Assert.Equal(PulseHeartbeatAcceptResult.Accepted, accepted);
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
            Heartbeat("pulse-token"),
            "203.0.113.10");

        Assert.Equal(PulseHeartbeatAcceptResult.Accepted, accepted);
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
        return new PulseAgentService(db, dns, audit, new ConnectionTargetResolver(db, audit), NullLogger<PulseAgentService>.Instance);
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

    private static Hashi.Contracts.Api.PulseHeartbeatAuthRequest Heartbeat(string token)
        => new(token, "0.1.0", "host", ["10.0.0.5"], [], "eth0", "10.0.0.5", DateTimeOffset.UtcNow, null);

    private static string FindRepoFile(params string[] parts)
    {
        var current = new DirectoryInfo(AppContext.BaseDirectory);
        while (current is not null)
        {
            var path = Path.Combine([current.FullName, .. parts]);
            if (File.Exists(path))
            {
                return path;
            }

            current = current.Parent;
        }

        throw new FileNotFoundException(string.Join(Path.DirectorySeparatorChar, parts));
    }
}
