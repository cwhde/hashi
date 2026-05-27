using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Connections;
using Hashi.Core.Traefik;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Connections;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class TraefikSyncService(
    HashiDbContext db,
    ISshRemoteExecutor ssh,
    TraefikPlatformService traefik,
    SecretRecordService secrets,
    AuditService audit)
{
    private const string DynamicDirectory = "/etc/hashi/traefik/dynamic";

    private static readonly (string FileName, Func<TraefikDynamicFiles, string> Selector)[] DynamicFileMap =
    [
        ("00-hashi-core.yml", f => f.CoreYaml),
        ("10-hashi-http-resources.yml", f => f.HttpResourcesYaml),
        ("20-hashi-stream-resources.yml", f => f.StreamResourcesYaml),
        ("30-user-middlewares.yml", f => f.UserMiddlewaresYaml),
        ("40-hashi-security.yml", f => f.SecurityYaml),
        ("90-hashi-health.yml", f => f.HealthYaml),
    ];

    public async Task<TraefikApplyResponse> ApplyAsync(TraefikApplyRequest request, CancellationToken cancellationToken = default)
    {
        var render = await traefik.RenderAsync(cancellationToken);
        var localValidation = TraefikConfigValidator.ValidateRender(render);
        if (!localValidation.IsValid)
        {
            var message = "Rendered Traefik config failed local YAML validation: " + string.Join("; ", localValidation.Errors);
            await audit.WriteAsync("traefik", "config_validation_failed", subjectType: "connection", subjectId: request.ConnectionId.ToString(), metadata: message, cancellationToken: cancellationToken);
            return new TraefikApplyResponse(false, render.ContentHash, false, message);
        }

        var state = await db.TraefikHostStates.SingleOrDefaultAsync(x => x.ConnectionId == request.ConnectionId, cancellationToken);
        var isNew = state is null;
        state ??= CreateDefaultState(request.ConnectionId);

        if (string.Equals(state.LastAppliedContentHash, render.ContentHash, StringComparison.Ordinal))
        {
            await audit.WriteAsync("traefik", "config_apply_noop", subjectType: "connection", subjectId: request.ConnectionId.ToString(), cancellationToken: cancellationToken);
            return new TraefikApplyResponse(true, render.ContentHash, true, "Config unchanged; skipped write.");
        }

        var settings = BuildSettings(request);
        var staticBytes = System.Text.Encoding.UTF8.GetBytes(render.StaticConfigYaml);

        var staticBackup = await ssh.ReadFileAsync(
            settings, request.AuthMode, request.Password, request.PrivateKeyPem, request.PrivateKeyPassphrase,
            state.StaticConfigPath, cancellationToken);
        if (staticBackup.Succeeded && staticBackup.Content is not null)
        {
            state.LastBackupStaticYaml = System.Text.Encoding.UTF8.GetString(staticBackup.Content);
        }

        var dynamicBackup = new Dictionary<string, string>();
        foreach (var (fileName, _) in DynamicFileMap)
        {
            var path = $"{DynamicDirectory}/{fileName}";
            var read = await ssh.ReadFileAsync(
                settings, request.AuthMode, request.Password, request.PrivateKeyPem, request.PrivateKeyPassphrase,
                path, cancellationToken);
            if (read.Succeeded && read.Content is not null && read.Content.Length > 0)
            {
                dynamicBackup[fileName] = System.Text.Encoding.UTF8.GetString(read.Content);
            }
        }

        if (dynamicBackup.Count > 0)
        {
            state.LastBackupDynamicYaml = JsonSerializer.Serialize(dynamicBackup);
        }

        var mkdir = await ssh.RunCommandAsync(
            settings, request.AuthMode, request.Password, request.PrivateKeyPem, request.PrivateKeyPassphrase,
            $"mkdir -p $(dirname {Quote(state.StaticConfigPath)}) {Quote(DynamicDirectory)}",
            cancellationToken);
        if (!mkdir.Succeeded)
        {
            return new TraefikApplyResponse(false, render.ContentHash, false, mkdir.Error ?? mkdir.Output);
        }

        if (RemoteContentMatches(staticBackup, render.StaticConfigYaml)
            && DynamicFilesMatch(dynamicBackup, render.DynamicFiles))
        {
            state.LastAppliedContentHash = render.ContentHash;
            state.LastAppliedAtUtc = DateTimeOffset.UtcNow;
            state.DynamicConfigPath = $"{DynamicDirectory}/10-hashi-http-resources.yml";
            if (isNew)
            {
                db.TraefikHostStates.Add(state);
            }

            await db.SaveChangesAsync(cancellationToken);
            await audit.WriteAsync("traefik", "config_apply_noop", subjectType: "connection", subjectId: request.ConnectionId.ToString(), cancellationToken: cancellationToken);
            return new TraefikApplyResponse(true, render.ContentHash, true, "Remote Traefik config already matches rendered content; skipped write.");
        }

        var remoteValidation = await ValidateStagedRemoteConfigAsync(settings, request, render, cancellationToken);
        if (!remoteValidation.Succeeded)
        {
            await audit.WriteAsync("traefik", "config_validation_failed", subjectType: "connection", subjectId: request.ConnectionId.ToString(), metadata: remoteValidation.Message, cancellationToken: cancellationToken);
            return new TraefikApplyResponse(false, render.ContentHash, false, remoteValidation.Message);
        }

        var staticWrite = await WriteAsync(settings, request, state.StaticConfigPath, staticBytes, cancellationToken);
        if (!staticWrite.Succeeded)
        {
            return new TraefikApplyResponse(false, render.ContentHash, false, staticWrite.Error);
        }

        foreach (var (fileName, selector) in DynamicFileMap)
        {
            var path = $"{DynamicDirectory}/{fileName}";
            var content = System.Text.Encoding.UTF8.GetBytes(selector(render.DynamicFiles));
            var write = await WriteAsync(settings, request, path, content, cancellationToken);
            if (!write.Succeeded)
            {
                return new TraefikApplyResponse(false, render.ContentHash, false, write.Error);
            }
        }

        state.LastAppliedContentHash = render.ContentHash;
        state.LastAppliedAtUtc = DateTimeOffset.UtcNow;
        state.DynamicConfigPath = $"{DynamicDirectory}/10-hashi-http-resources.yml";
        if (isNew)
        {
            db.TraefikHostStates.Add(state);
        }

        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("traefik", "config_applied", subjectType: "connection", subjectId: request.ConnectionId.ToString(), cancellationToken: cancellationToken);
        return new TraefikApplyResponse(true, render.ContentHash, false, null);
    }

    public async Task<TraefikApplyResponse> ApplyForConnectionAsync(
        Guid connectionId,
        bool confirmReplaceExisting,
        CancellationToken cancellationToken = default)
    {
        if (!confirmReplaceExisting)
        {
            var existing = await DetectExistingAsync(connectionId, cancellationToken);
            if (existing.Found)
            {
                return new TraefikApplyResponse(
                    false,
                    string.Empty,
                    false,
                    "Existing Traefik config detected. Confirm backup and Hashi ownership before applying.");
            }
        }

        return await ApplyForConnectionInternalAsync(connectionId, cancellationToken);
    }

    public async Task<TraefikApplyResponse> RollbackForConnectionAsync(
        Guid connectionId,
        CancellationToken cancellationToken = default)
    {
        var connection = await db.Connections.SingleOrDefaultAsync(x => x.Id == connectionId, cancellationToken)
            ?? throw new InvalidOperationException("Connection not found.");
        var credentials = await ConnectionSshCredentialResolver.ResolveAsync(connection, secrets, cancellationToken)
            ?? throw new InvalidOperationException("SSH credentials unavailable for connection.");

        return await RollbackAsync(new TraefikApplyRequest(
            connectionId,
            credentials.Settings.Host,
            credentials.Settings.Port,
            credentials.Settings.Username,
            credentials.AuthMode,
            credentials.Password,
            credentials.PrivateKeyPem,
            credentials.PrivateKeyPassphrase), cancellationToken);
    }

    public async Task<TraefikDetectExistingResponse> DetectExistingAsync(
        Guid connectionId,
        CancellationToken cancellationToken = default)
    {
        var connection = await db.Connections.SingleOrDefaultAsync(x => x.Id == connectionId, cancellationToken)
            ?? throw new InvalidOperationException("Connection not found.");
        var credentials = await ConnectionSshCredentialResolver.ResolveAsync(connection, secrets, cancellationToken)
            ?? throw new InvalidOperationException("SSH credentials unavailable for connection.");
        var state = await db.TraefikHostStates.SingleOrDefaultAsync(x => x.ConnectionId == connectionId, cancellationToken);
        var remotePath = state?.StaticConfigPath ?? "/etc/hashi/traefik/traefik.yml";
        var settings = new Hashi.Core.Connections.SshConnectionSettings(
            credentials.Settings.Host,
            credentials.Settings.Port <= 0 ? 22 : credentials.Settings.Port,
            credentials.Settings.Username,
            Hashi.Core.Connections.OsFamily.Unknown,
            null,
            null);
        var read = await ssh.ReadFileAsync(
            settings,
            credentials.AuthMode,
            credentials.Password,
            credentials.PrivateKeyPem,
            credentials.PrivateKeyPassphrase,
            remotePath,
            cancellationToken);

        if (!read.Succeeded || read.Content is null || read.Content.Length == 0)
        {
            return new TraefikDetectExistingResponse(false, null, remotePath);
        }

        var text = System.Text.Encoding.UTF8.GetString(read.Content);
        var preview = text.Length > 500 ? text[..500] + "..." : text;
        return new TraefikDetectExistingResponse(true, preview, remotePath);
    }

    public async Task<TraefikApplyResponse> ApplyForConnectionInternalAsync(Guid connectionId, CancellationToken cancellationToken = default)
    {
        var connection = await db.Connections.SingleOrDefaultAsync(x => x.Id == connectionId, cancellationToken)
            ?? throw new InvalidOperationException("Connection not found.");
        var credentials = await ConnectionSshCredentialResolver.ResolveAsync(connection, secrets, cancellationToken)
            ?? throw new InvalidOperationException("SSH credentials unavailable for connection.");

        return await ApplyAsync(new TraefikApplyRequest(
            connectionId,
            credentials.Settings.Host,
            credentials.Settings.Port,
            credentials.Settings.Username,
            credentials.AuthMode,
            credentials.Password,
            credentials.PrivateKeyPem,
            credentials.PrivateKeyPassphrase), cancellationToken);
    }

    public async Task<TraefikInstallResponse> InstallAsync(TraefikInstallRequest request, CancellationToken cancellationToken = default)
    {
        var settings = BuildSettings(request);
        var validation = request.AuthMode switch
        {
            "password" when !string.IsNullOrWhiteSpace(request.Password) =>
                await ssh.ValidateAsync(settings, request.Password, cancellationToken),
            "private_key" when !string.IsNullOrWhiteSpace(request.PrivateKeyPem) =>
                await ssh.ValidateWithPrivateKeyAsync(settings, request.PrivateKeyPem, request.PrivateKeyPassphrase, cancellationToken),
            _ => new Hashi.Core.Connections.SshValidationResult(false, Hashi.Core.Connections.OsFamily.Unknown, null, "Unsupported auth mode."),
        };

        if (!validation.Succeeded)
        {
            return new TraefikInstallResponse(false, validation.Error);
        }

        var installCommand = validation.OsFamily == Hashi.Core.Connections.OsFamily.Alpine
            ? BuildAlpineInstallScript()
            : BuildDebianInstallScript();
        var result = await ssh.RunCommandAsync(
            settings,
            request.AuthMode,
            request.Password,
            request.PrivateKeyPem,
            request.PrivateKeyPassphrase,
            installCommand,
            cancellationToken);
        return new TraefikInstallResponse(result.Succeeded, result.Error ?? result.Output);
    }

    private static string BuildDebianInstallScript() => """
        set -euo pipefail
        export DEBIAN_FRONTEND=noninteractive
        mkdir -p /etc/hashi/traefik/dynamic /var/log/hashi/traefik /var/lib/hashi/traefik
        apt-get update
        apt-get install -y traefik || apt-get install -y traefik2
        cat > /etc/systemd/system/traefik.service <<'UNIT'
        [Unit]
        Description=Hashi-managed Traefik
        After=network-online.target
        Wants=network-online.target

        [Service]
        Type=simple
        ExecStart=/usr/bin/traefik --configFile=/etc/hashi/traefik/traefik.yml
        Restart=on-failure
        RestartSec=5

        [Install]
        WantedBy=multi-user.target
        UNIT
        systemctl daemon-reload
        systemctl enable traefik || true
        echo "Traefik directories and systemd unit prepared."
        """;

    private static string BuildAlpineInstallScript() => """
        set -euo pipefail
        mkdir -p /etc/hashi/traefik/dynamic /var/log/hashi/traefik /var/lib/hashi/traefik
        apk add --no-cache traefik
        cat > /etc/init.d/traefik <<'INIT'
        #!/sbin/openrc-run
        name="traefik"
        command="/usr/bin/traefik"
        command_args="--configFile=/etc/hashi/traefik/traefik.yml"
        pidfile="/run/${RC_SVCNAME}.pid"
        INIT
        chmod +x /etc/init.d/traefik
        rc-update add traefik default || true
        echo "Traefik directories and OpenRC service prepared."
        """;

    public async Task<TraefikApplyResponse> RollbackAsync(TraefikApplyRequest request, CancellationToken cancellationToken = default)
    {
        var state = await db.TraefikHostStates.SingleOrDefaultAsync(x => x.ConnectionId == request.ConnectionId, cancellationToken);
        if (state is null || (string.IsNullOrWhiteSpace(state.LastBackupStaticYaml) && string.IsNullOrWhiteSpace(state.LastBackupDynamicYaml)))
        {
            return new TraefikApplyResponse(false, state?.LastAppliedContentHash ?? string.Empty, false, "No Traefik backup available to restore.");
        }

        var settings = BuildSettings(request);
        if (!string.IsNullOrWhiteSpace(state.LastBackupStaticYaml))
        {
            var backupBytes = System.Text.Encoding.UTF8.GetBytes(state.LastBackupStaticYaml);
            var write = await WriteAsync(settings, request, state.StaticConfigPath, backupBytes, cancellationToken);
            if (!write.Succeeded)
            {
                return new TraefikApplyResponse(false, state.LastAppliedContentHash ?? string.Empty, false, write.Error);
            }
        }

        if (!string.IsNullOrWhiteSpace(state.LastBackupDynamicYaml))
        {
            var dynamicFiles = JsonSerializer.Deserialize<Dictionary<string, string>>(state.LastBackupDynamicYaml) ?? [];
            foreach (var (fileName, content) in dynamicFiles)
            {
                var path = $"{DynamicDirectory}/{fileName}";
                var write = await WriteAsync(settings, request, path, System.Text.Encoding.UTF8.GetBytes(content), cancellationToken);
                if (!write.Succeeded)
                {
                    return new TraefikApplyResponse(false, state.LastAppliedContentHash ?? string.Empty, false, write.Error);
                }
            }
        }

        state.LastAppliedContentHash = null;
        state.LastAppliedAtUtc = null;
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("traefik", "config_rollback", subjectType: "connection", subjectId: request.ConnectionId.ToString(), cancellationToken: cancellationToken);
        return new TraefikApplyResponse(true, state.LastAppliedContentHash ?? string.Empty, false, "Traefik static and dynamic configs restored from backup.");
    }

    private async Task<Hashi.Core.Connections.RemoteWriteResult> WriteAsync(
        Hashi.Core.Connections.SshConnectionSettings settings,
        TraefikApplyRequest request,
        string remotePath,
        byte[] content,
        CancellationToken cancellationToken)
        => request.AuthMode switch
        {
            "password" when !string.IsNullOrWhiteSpace(request.Password) =>
                await ssh.WriteAtomicAsync(settings, request.Password, remotePath, content, cancellationToken),
            "private_key" when !string.IsNullOrWhiteSpace(request.PrivateKeyPem) =>
                await ssh.WriteAtomicWithPrivateKeyAsync(
                    settings, request.PrivateKeyPem, request.PrivateKeyPassphrase, remotePath, content, cancellationToken),
            _ => new Hashi.Core.Connections.RemoteWriteResult(false, remotePath, "Unsupported auth mode."),
        };

    private async Task<(bool Succeeded, string? Message)> ValidateStagedRemoteConfigAsync(
        Hashi.Core.Connections.SshConnectionSettings settings,
        TraefikApplyRequest request,
        TraefikRenderResult render,
        CancellationToken cancellationToken)
    {
        var stagingDirectory = $"/tmp/hashi-traefik-{render.ContentHash}";
        var stagingDynamicDirectory = $"{stagingDirectory}/dynamic";
        var mkdir = await ssh.RunCommandAsync(
            settings,
            request.AuthMode,
            request.Password,
            request.PrivateKeyPem,
            request.PrivateKeyPassphrase,
            $"rm -rf {Quote(stagingDirectory)} && mkdir -p {Quote(stagingDynamicDirectory)}",
            cancellationToken);
        if (!mkdir.Succeeded)
        {
            return (false, mkdir.Error ?? mkdir.Output);
        }

        var stagedStaticYaml = render.StaticConfigYaml.Replace(
            "directory: /etc/hashi/traefik/dynamic",
            $"directory: {stagingDynamicDirectory}",
            StringComparison.Ordinal);
        var staticWrite = await WriteAsync(
            settings,
            request,
            $"{stagingDirectory}/traefik.yml",
            System.Text.Encoding.UTF8.GetBytes(stagedStaticYaml),
            cancellationToken);
        if (!staticWrite.Succeeded)
        {
            return (false, staticWrite.Error);
        }

        foreach (var (fileName, selector) in DynamicFileMap)
        {
            var write = await WriteAsync(
                settings,
                request,
                $"{stagingDynamicDirectory}/{fileName}",
                System.Text.Encoding.UTF8.GetBytes(selector(render.DynamicFiles)),
                cancellationToken);
            if (!write.Succeeded)
            {
                return (false, write.Error);
            }
        }

        var check = await ssh.RunCommandAsync(
            settings,
            request.AuthMode,
            request.Password,
            request.PrivateKeyPem,
            request.PrivateKeyPassphrase,
            $"traefik check --configFile {Quote($"{stagingDirectory}/traefik.yml")}",
            cancellationToken);
        _ = await ssh.RunCommandAsync(
            settings,
            request.AuthMode,
            request.Password,
            request.PrivateKeyPem,
            request.PrivateKeyPassphrase,
            $"rm -rf {Quote(stagingDirectory)}",
            cancellationToken);

        return check.Succeeded
            ? (true, null)
            : (false, check.Error ?? check.Output);
    }

    private static bool RemoteContentMatches(Hashi.Core.Connections.RemoteReadResult read, string expected)
        => read.Succeeded
            && read.Content is not null
            && string.Equals(System.Text.Encoding.UTF8.GetString(read.Content), expected, StringComparison.Ordinal);

    private static bool DynamicFilesMatch(Dictionary<string, string> remoteFiles, TraefikDynamicFiles expected)
        => DynamicFileMap.All(x => remoteFiles.TryGetValue(x.FileName, out var remoteContent)
            && string.Equals(remoteContent, x.Selector(expected), StringComparison.Ordinal));

    private static TraefikHostStateEntity CreateDefaultState(Guid connectionId) => new()
    {
        ConnectionId = connectionId,
    };

    private static Hashi.Core.Connections.SshConnectionSettings BuildSettings(TraefikApplyRequest request) => new(
        request.Host,
        request.Port <= 0 ? 22 : request.Port,
        request.Username,
        Hashi.Core.Connections.OsFamily.Unknown,
        null,
        null);

    private static Hashi.Core.Connections.SshConnectionSettings BuildSettings(TraefikInstallRequest request) => new(
        request.Host,
        request.Port <= 0 ? 22 : request.Port,
        request.Username,
        Hashi.Core.Connections.OsFamily.Unknown,
        null,
        null);

    private static string Quote(string value) => $"'{value.Replace("'", "'\\''", StringComparison.Ordinal)}'";
}
