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

public sealed class ScriptExecutionServiceTests
{
    [Fact]
    public async Task RunAsync_runs_configured_target_and_persists_redacted_status_output()
    {
        await using var db = CreateDb();
        var rootKey = RandomNumberGenerator.GetBytes(32);
        var vault = new VaultSessionState();
        vault.Unlock(rootKey);
        var serviceSync = new ServiceSyncVaultState();
        serviceSync.Initialize(RandomNumberGenerator.GetBytes(32));
        var secrets = new SecretRecordService(db, vault, serviceSync);
        var defaultConnectionId = await AddConnectionAsync(db, secrets, "default", "10.0.0.10");
        var targetConnectionId = await AddConnectionAsync(db, secrets, "target", "10.0.0.20");
        var ssh = new FakeSshRemoteExecutor
        {
            CommandResult = new RemoteCommandResult(true, "ready super-secret-token", null),
        };
        var scripts = CreateService(db, ssh, secrets);

        var created = await scripts.CreateAsync(new CreateScriptRequest(
            defaultConnectionId,
            "Rotate",
            "Rotate things",
            "echo ready",
            string.Empty,
            [targetConnectionId],
            EnvironmentVariables:
            [
                new ScriptEnvironmentVariableRequest("API_TOKEN", "super-secret-token", IsSecret: true),
            ]));

        var result = await scripts.RunAsync(created.Id, new RunScriptRequest());

        Assert.True(result.Succeeded);
        Assert.DoesNotContain("super-secret-token", result.Output, StringComparison.Ordinal);
        Assert.Contains("[REDACTED]", result.Output, StringComparison.Ordinal);
        Assert.Contains(ssh.CommandSettings, x => x.Host == "10.0.0.20");
        Assert.DoesNotContain(ssh.CommandSettings, x => x.Host == "10.0.0.10");

        var script = await db.Scripts.SingleAsync(x => x.Id == created.Id);
        Assert.Equal(ScriptRunStatusNames.Succeeded, script.LastRunStatus);
        Assert.NotNull(script.LastRunId);
        Assert.DoesNotContain("super-secret-token", script.LastRunOutput ?? string.Empty, StringComparison.Ordinal);

        var run = await db.ScriptRuns.SingleAsync(x => x.ScriptId == created.Id);
        Assert.Equal(targetConnectionId, run.ConnectionId);
        Assert.True(run.Succeeded);
        Assert.Equal(ScriptRunStatusNames.Succeeded, run.Status);
        var output = await db.ScriptOutputs.SingleAsync(x => x.RunId == run.Id && x.Stream == ScriptOutputStreamNames.Stdout);
        Assert.Equal("ready [REDACTED]", output.Content);
    }

    [Fact]
    public async Task CreateAsync_rejects_missing_target_connection()
    {
        await using var db = CreateDb();
        var vault = new VaultSessionState();
        vault.Unlock(RandomNumberGenerator.GetBytes(32));
        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var scripts = CreateService(db, new FakeSshRemoteExecutor(), secrets);

        var ex = await Assert.ThrowsAsync<InvalidOperationException>(() => scripts.CreateAsync(new CreateScriptRequest(
            Guid.NewGuid(),
            "Bad",
            "No target",
            "echo no",
            string.Empty)));

        Assert.Contains("was not found", ex.Message, StringComparison.Ordinal);
    }

    [Fact]
    public async Task RunAsync_rejects_ad_hoc_ssh_target()
    {
        await using var db = CreateDb();
        var vault = new VaultSessionState();
        vault.Unlock(RandomNumberGenerator.GetBytes(32));
        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var connectionId = await AddConnectionAsync(db, secrets, "firewall", "10.0.0.30");
        var scripts = CreateService(db, new FakeSshRemoteExecutor(), secrets);
        var created = await scripts.CreateAsync(new CreateScriptRequest(connectionId, "Safe", "Safe", "echo ok", string.Empty));

        var ex = await Assert.ThrowsAsync<InvalidOperationException>(() => scripts.RunAsync(
            created.Id,
            new RunScriptRequest(Host: "203.0.113.10", Username: "root", Password: "pw")));

        Assert.Contains("configured target connections", ex.Message, StringComparison.Ordinal);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private static ScriptExecutionService CreateService(HashiDbContext db, FakeSshRemoteExecutor ssh, SecretRecordService secrets)
        => new(db, ssh, secrets, new AuditService(db));

    private static async Task<Guid> AddConnectionAsync(
        HashiDbContext db,
        SecretRecordService secrets,
        string name,
        string host)
    {
        var credential = await secrets.StoreAsync(
            SecretPurpose.SshCredential,
            $"{name} SSH",
            ConnectionSshCredentialResolver.SerializeCredentialPayload("password", "pw", null, null));
        var connection = new ConnectionEntity
        {
            Name = name,
            Type = ConnectionTypeNames.FirewallHost,
            SecretId = credential.Id,
            SettingsJson = $$"""{"Host":"{{host}}","Port":22,"Username":"root"}""",
        };
        db.Connections.Add(connection);
        await db.SaveChangesAsync();
        return connection.Id;
    }
}
