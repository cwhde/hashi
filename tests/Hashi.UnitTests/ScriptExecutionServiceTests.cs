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

    [Fact]
    public async Task SyncAllEnabledScriptsAsync_writes_manifest_and_host_cron_without_disabled_scripts()
    {
        await using var db = CreateDb();
        var vault = new VaultSessionState();
        vault.Unlock(RandomNumberGenerator.GetBytes(32));
        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var connectionId = await AddConnectionAsync(db, secrets, "firewall", "10.0.0.40");
        db.Scripts.AddRange(
            new ScriptEntity
            {
                ConnectionId = connectionId,
                Name = "Enabled",
                Body = "echo enabled",
                CronExpression = "*/5 * * * *",
                Enabled = true,
                RunTimeoutSeconds = 120,
            },
            new ScriptEntity
            {
                ConnectionId = connectionId,
                Name = "Disabled",
                Body = "echo disabled",
                CronExpression = "* * * * *",
                Enabled = false,
            });
        await db.SaveChangesAsync();
        var enabledScript = await db.Scripts.SingleAsync(x => x.Name == "Enabled");
        var ssh = new FakeSshRemoteExecutor();
        ssh.CommandResults.Enqueue(new RemoteCommandResult(true, "", null));
        ssh.CommandResults.Enqueue(new RemoteCommandResult(true, "", null));
        ssh.CommandResults.Enqueue(new RemoteCommandResult(true, "", null));
        ssh.CommandResults.Enqueue(new RemoteCommandResult(false, "", "No systemd"));
        ssh.CommandResults.Enqueue(new RemoteCommandResult(true, "", null));
        var scripts = CreateService(db, ssh, secrets);

        await scripts.SyncAllEnabledScriptsAsync();

        Assert.True(ssh.WrittenFiles.ContainsKey("/opt/hashi/scripts/manifest.json"));
        Assert.True(ssh.WrittenFiles.ContainsKey("/etc/cron.d/hashi-scripts"));
        Assert.True(ssh.WrittenFiles.ContainsKey($"/opt/hashi/scripts/{enabledScript.Id:N}.sh"));
        var manifest = System.Text.Encoding.UTF8.GetString(ssh.WrittenFiles["/opt/hashi/scripts/manifest.json"]);
        var cron = System.Text.Encoding.UTF8.GetString(ssh.WrittenFiles["/etc/cron.d/hashi-scripts"]);
        Assert.Contains(enabledScript.Id.ToString(), manifest);
        Assert.Contains("sha256", manifest);
        Assert.Contains("*/5 * * * * root timeout 120 bash", cron);
        Assert.Contains($"/opt/hashi/scripts/{enabledScript.Id:N}.sh", cron);
        Assert.DoesNotContain("Disabled", manifest, StringComparison.Ordinal);
        Assert.DoesNotContain("disabled", cron, StringComparison.OrdinalIgnoreCase);
        Assert.Contains(ssh.Commands, command => command.Contains("/etc/cron.d/hashi-scripts", StringComparison.Ordinal));
    }

    [Fact]
    public async Task Scripts_without_explicit_targets_default_to_all_enabled_firewall_hosts()
    {
        await using var db = CreateDb();
        var vault = new VaultSessionState();
        vault.Unlock(RandomNumberGenerator.GetBytes(32));
        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var firstConnectionId = await AddConnectionAsync(db, secrets, "firewall-a", "10.0.0.51");
        var secondConnectionId = await AddConnectionAsync(db, secrets, "firewall-b", "10.0.0.52");
        await AddConnectionAsync(db, secrets, "firewall-disabled", "10.0.0.53", enabled: false);
        var ssh = new FakeSshRemoteExecutor();
        var scripts = CreateService(db, ssh, secrets);

        var created = await scripts.CreateAsync(new CreateScriptRequest(
            firstConnectionId,
            "Default Target",
            "Runs everywhere",
            "echo all",
            "0 * * * *"));

        Assert.Contains(created.Targets, x => x.ConnectionId == firstConnectionId && x.Enabled);
        Assert.Contains(created.Targets, x => x.ConnectionId == secondConnectionId && x.Enabled);
        Assert.Equal(2, created.Targets.Count);

        await scripts.SyncAllEnabledScriptsAsync();

        var hosts = ssh.CommandSettings.Select(x => x.Host).Distinct().ToList();
        Assert.Contains("10.0.0.51", hosts);
        Assert.Contains("10.0.0.52", hosts);
        Assert.DoesNotContain("10.0.0.53", hosts);
    }

    [Fact]
    public void Script_rendering_outputs_manifest_hashes_and_cleans_stale_cron_entries()
    {
        var enabled = new ScriptEntity
        {
            Id = Guid.Parse("11111111-1111-1111-1111-111111111111"),
            Name = "Enabled",
            Body = "echo enabled",
            CronExpression = "15 * * * *",
            Enabled = true,
            RunTimeoutSeconds = 60,
        };
        var disabled = new ScriptEntity
        {
            Id = Guid.Parse("22222222-2222-2222-2222-222222222222"),
            Name = "Disabled",
            Body = "echo disabled",
            CronExpression = "* * * * *",
            Enabled = false,
        };

        var manifest = ScriptExecutionService.RenderManifest([enabled]);
        var cron = ScriptExecutionService.RenderCron([enabled, disabled]);

        Assert.Contains("11111111-1111-1111-1111-111111111111", manifest);
        Assert.Contains("sha256", manifest);
        Assert.Contains("15 * * * * root timeout 60 bash /opt/hashi/scripts/11111111111111111111111111111111.sh", cron);
        Assert.DoesNotContain("22222222-2222-2222-2222-222222222222", manifest);
        Assert.DoesNotContain("Disabled", cron);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private static ScriptExecutionService CreateService(HashiDbContext db, FakeSshRemoteExecutor ssh, SecretRecordService secrets)
    {
        var audit = new AuditService(db);
        return new(db, ssh, secrets, audit, new ConnectionTargetResolver(db, audit));
    }

    private static async Task<Guid> AddConnectionAsync(
        HashiDbContext db,
        SecretRecordService secrets,
        string name,
        string host,
        bool enabled = true)
    {
        var credential = await secrets.StoreAsync(
            SecretPurpose.SshCredential,
            $"{name} SSH",
            ConnectionSshCredentialResolver.SerializeCredentialPayload("password", "pw", null, null));
        var connection = new ConnectionEntity
        {
            Name = name,
            Type = ConnectionTypeNames.FirewallHost,
            Enabled = enabled,
            SecretId = credential.Id,
            SettingsJson = $$"""{"Host":"{{host}}","Port":22,"Username":"root"}""",
        };
        db.Connections.Add(connection);
        await db.SaveChangesAsync();
        return connection.Id;
    }

    [Theory]
    [InlineData("0 3 * * *", "*-*-* 03:00:00")]
    [InlineData("*/5 * * * *", "*-*-* *:*/5:00")]
    [InlineData("30 2 * * 1-5", "Mon-Fri *-*-* 02:30:00")]
    [InlineData("0 0 1 1 *", "*-1-1 00:00:00")]
    [InlineData("0 12 * * 0,6", "Sun,Sat *-*-* 12:00:00")]
    public void ConvertCronToOnCalendar_translates_cron_expressions(string cron, string expected)
    {
        var result = ScriptExecutionService.ConvertCronToOnCalendar(cron);
        Assert.Equal(expected, result);
    }

    [Fact]
    public async Task SyncAllEnabledScriptsAsync_writes_systemd_timers_when_systemd_present()
    {
        await using var db = CreateDb();
        var vault = new VaultSessionState();
        vault.Unlock(RandomNumberGenerator.GetBytes(32));
        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var connectionId = await AddConnectionAsync(db, secrets, "firewall", "10.0.0.40");

        var script = new ScriptEntity
        {
            ConnectionId = connectionId,
            Name = "SystemdBackup",
            Body = "echo backup",
            CronExpression = "0 3 * * *",
            Enabled = true,
            RunTimeoutSeconds = 120,
        };
        db.Scripts.Add(script);
        await db.SaveChangesAsync();

        var ssh = new FakeSshRemoteExecutor();
        // Since default CommandResult.Succeeded is true, systemd is detected as present.
        // We can set output for find command.
        ssh.CommandResult = new RemoteCommandResult(true, "/etc/systemd/system/hashi-script-00000000000000000000000000000000.timer\n", null);

        var scripts = CreateService(db, ssh, secrets);
        await scripts.SyncAllEnabledScriptsAsync();

        var servicePath = $"/etc/systemd/system/hashi-script-{script.Id:N}.service";
        var timerPath = $"/etc/systemd/system/hashi-script-{script.Id:N}.timer";

        Assert.True(ssh.WrittenFiles.ContainsKey(servicePath));
        Assert.True(ssh.WrittenFiles.ContainsKey(timerPath));

        var serviceContent = System.Text.Encoding.UTF8.GetString(ssh.WrittenFiles[servicePath]);
        var timerContent = System.Text.Encoding.UTF8.GetString(ssh.WrittenFiles[timerPath]);

        Assert.Contains($"Description=Hashi Script execution - {script.Name}", serviceContent);
        Assert.Contains("Type=oneshot", serviceContent);
        Assert.Contains($"ExecStart=/bin/bash /opt/hashi/scripts/{script.Id:N}.sh", serviceContent);
        Assert.Contains($"TimeoutStartSec=120", serviceContent);
        Assert.Contains($"StandardOutput=append:/var/log/hashi/scripts/{script.Id:N}.log", serviceContent);

        Assert.Contains("OnCalendar=*-*-* 03:00:00", timerContent);
        Assert.Contains("Persistent=true", timerContent);

        // Check that daemon-reload and systemctl enable --now were called
        Assert.Contains(ssh.Commands, cmd => cmd.Contains("systemctl daemon-reload", StringComparison.Ordinal));
        Assert.Contains(ssh.Commands, cmd => cmd.Contains($"systemctl enable --now hashi-script-{script.Id:N}.timer", StringComparison.Ordinal));

        // Check cleanup of obsolete timer
        Assert.Contains(ssh.Commands, cmd => cmd.Contains("systemctl disable --now hashi-script-00000000000000000000000000000000.timer", StringComparison.Ordinal));
    }
}
