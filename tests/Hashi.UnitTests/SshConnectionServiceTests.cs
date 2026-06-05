using System.Security.Cryptography;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Core.Connections;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Connections;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Hashi.UnitTests.Fakes;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class SshConnectionServiceTests
{
    [Theory]
    [InlineData(ConnectionTypeNames.TraefikHost)]
    [InlineData(ConnectionTypeNames.FirewallHost)]
    public async Task CreateAsync_stores_runtime_ssh_credentials_for_service_sync(string connectionType)
    {
        await using var db = CreateDb();
        var serviceSync = ReadyServiceSync();
        var secrets = CreateSecrets(db, serviceSync);
        var service = CreateService(db, secrets, new FakeSshRemoteExecutor());

        var connection = await service.CreateAsync(
            "runtime",
            connectionType,
            new SshConnectionSettings("10.0.0.10", 22, "root", OsFamily.Unknown, null, null),
            "password",
            "ssh-password",
            privateKeyPem: null,
            privateKeyPassphrase: null);

        var secret = await db.SecretRecords.SingleAsync(x => x.Id == connection.SecretId);
        Assert.True(secret.IsServiceSyncEligible);
        Assert.NotNull(secret.ServiceWrappedDekBlob);

        var decrypted = await secrets.DecryptForServiceSyncAsync(secret.Id);
        Assert.NotNull(decrypted);
        Assert.Contains("ssh-password", System.Text.Encoding.UTF8.GetString(decrypted), StringComparison.Ordinal);
    }

    [Fact]
    public async Task ValidateAsync_preserves_static_ssh_host_when_no_target_exists()
    {
        await using var db = CreateDb();
        var secrets = CreateSecrets(db, ReadyServiceSync());
        var executor = new FakeSshRemoteExecutor();
        var service = CreateService(db, secrets, executor);

        var connection = await service.CreateAsync(
            "static",
            ConnectionTypeNames.TraefikHost,
            new SshConnectionSettings("ssh.example.com", 2022, "root", OsFamily.Unknown, null, null),
            "password",
            "ssh-password",
            privateKeyPem: null,
            privateKeyPassphrase: null);

        await service.ValidateAsync(connection.Id);

        var settings = Assert.Single(executor.ValidationSettings);
        Assert.Equal("ssh.example.com", settings.Host);
        Assert.Equal(2022, settings.Port);
        Assert.Empty(db.ConnectionTargets);
    }

    [Fact]
    public async Task ValidateAsync_resolves_pulse_agent_target_before_ssh_validation()
    {
        await using var db = CreateDb();
        var agentId = SeedPulseAgent(db);
        var secrets = CreateSecrets(db, ReadyServiceSync());
        var executor = new FakeSshRemoteExecutor();
        var service = CreateService(db, secrets, executor);

        var connection = await service.CreateAsync(
            "agent-bound",
            ConnectionTypeNames.FirewallHost,
            new SshConnectionSettings("placeholder", 22, "root", OsFamily.Unknown, null, null),
            "password",
            "ssh-password",
            privateKeyPem: null,
            privateKeyPassphrase: null,
            targetRequest: new ConnectionTargetRequest(
                ConnectionTargetModeNames.PulseAgent,
                StaticHost: null,
                StaticIp: null,
                PulseAgentId: agentId,
                PulseIpMode: PulseTargetIpModeNames.Selected,
                PrivateCandidateSelector: PulsePrivateCandidateSelectorNames.Selected,
                Port: 2222,
                Scheme: "http",
                PathPrefix: null,
                TlsValidationMode: TlsValidationModeNames.System,
                ExpectedTlsHostname: null));

        await service.ValidateAsync(connection.Id);

        var settings = Assert.Single(executor.ValidationSettings);
        Assert.Equal("10.0.0.44", settings.Host);
        Assert.Equal(2222, settings.Port);
        var target = await db.ConnectionTargets.SingleAsync();
        Assert.Equal(ConnectionTargetOwnerTypeNames.Connection, target.OwnerType);
        Assert.Equal(ConnectionTargetStatusNames.Resolved, target.Status);
    }

    [Fact]
    public async Task CreateAsync_rejects_missing_pulse_agent_target_without_storing_connection()
    {
        await using var db = CreateDb();
        var secrets = CreateSecrets(db, ReadyServiceSync());
        var service = CreateService(db, secrets, new FakeSshRemoteExecutor());

        await Assert.ThrowsAsync<InvalidOperationException>(() => service.CreateAsync(
            "missing-agent",
            ConnectionTypeNames.TraefikHost,
            new SshConnectionSettings("placeholder", 22, "root", OsFamily.Unknown, null, null),
            "password",
            "ssh-password",
            privateKeyPem: null,
            privateKeyPassphrase: null,
            targetRequest: new ConnectionTargetRequest(
                ConnectionTargetModeNames.PulseAgent,
                StaticHost: null,
                StaticIp: null,
                PulseAgentId: Guid.NewGuid(),
                PulseIpMode: PulseTargetIpModeNames.Selected,
                PrivateCandidateSelector: PulsePrivateCandidateSelectorNames.Selected,
                Port: 22,
                Scheme: "http",
                PathPrefix: null,
                TlsValidationMode: TlsValidationModeNames.System,
                ExpectedTlsHostname: null)));

        Assert.Empty(db.Connections);
        Assert.Empty(db.ConnectionTargets);
    }

    [Fact]
    public async Task ValidateAsync_allows_stale_agent_target_with_last_known_ip()
    {
        await using var db = CreateDb();
        var agentId = SeedPulseAgent(db, DateTimeOffset.UtcNow.AddMinutes(-30));
        var secrets = CreateSecrets(db, ReadyServiceSync());
        var executor = new FakeSshRemoteExecutor();
        var service = CreateService(db, secrets, executor);

        var connection = await service.CreateAsync(
            "stale-agent",
            ConnectionTypeNames.FirewallHost,
            new SshConnectionSettings("placeholder", 22, "root", OsFamily.Unknown, null, null),
            "password",
            "ssh-password",
            privateKeyPem: null,
            privateKeyPassphrase: null,
            targetRequest: new ConnectionTargetRequest(
                ConnectionTargetModeNames.PulseAgent,
                StaticHost: null,
                StaticIp: null,
                PulseAgentId: agentId,
                PulseIpMode: PulseTargetIpModeNames.Selected,
                PrivateCandidateSelector: PulsePrivateCandidateSelectorNames.Selected,
                Port: 22,
                Scheme: "http",
                PathPrefix: null,
                TlsValidationMode: TlsValidationModeNames.System,
                ExpectedTlsHostname: null));

        await service.ValidateAsync(connection.Id);

        Assert.Equal("10.0.0.44", Assert.Single(executor.ValidationSettings).Host);
        Assert.Equal(ConnectionTargetStatusNames.Stale, await db.ConnectionTargets.Select(x => x.Status).SingleAsync());
    }

    private static SecretRecordService CreateSecrets(HashiDbContext db, ServiceSyncVaultState serviceSync)
    {
        var vault = new VaultSessionState();
        vault.Unlock(RandomNumberGenerator.GetBytes(32));
        return new SecretRecordService(db, vault, serviceSync);
    }

    private static ServiceSyncVaultState ReadyServiceSync()
    {
        var serviceSync = new ServiceSyncVaultState();
        serviceSync.Initialize(RandomNumberGenerator.GetBytes(32));
        return serviceSync;
    }

    private static SshConnectionService CreateService(
        HashiDbContext db,
        SecretRecordService secrets,
        FakeSshRemoteExecutor executor)
    {
        var audit = new AuditService(db);
        return new SshConnectionService(db, executor, secrets, audit, new ConnectionTargetResolver(db, audit));
    }

    private static Guid SeedPulseAgent(HashiDbContext db, DateTimeOffset? lastSeenAtUtc = null)
    {
        var agentId = Guid.NewGuid();
        db.PulseAgents.Add(new PulseAgentEntity
        {
            Id = agentId,
            Name = "edge",
            TokenHash = "hash",
            Status = "online",
            HeartbeatIntervalSeconds = 60,
            LastSeenAtUtc = lastSeenAtUtc ?? DateTimeOffset.UtcNow,
            LastSelectedIp = "10.0.0.44",
            LastPrivateIp = "10.0.0.44",
            LastPublicIp = "203.0.113.44",
            LastPrivateIpv4CandidatesJson = """["10.0.0.44"]""",
        });
        db.SaveChanges();
        return agentId;
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
