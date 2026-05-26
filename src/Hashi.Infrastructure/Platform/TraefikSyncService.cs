using Hashi.Contracts.Api;
using Hashi.Core.Connections;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class TraefikSyncService(
    HashiDbContext db,
    ISshRemoteExecutor ssh,
    AuditService audit)
{
    public async Task<TraefikApplyResponse> ApplyAsync(TraefikApplyRequest request, CancellationToken cancellationToken = default)
    {
        var render = await RenderCurrentAsync(cancellationToken);
        var state = await db.TraefikHostStates.SingleOrDefaultAsync(x => x.ConnectionId == request.ConnectionId, cancellationToken);
        var isNew = state is null;
        state ??= CreateDefaultState(request.ConnectionId);

        if (string.Equals(state.LastAppliedContentHash, render.ContentHash, StringComparison.Ordinal))
        {
            return new TraefikApplyResponse(true, render.ContentHash, true, "Config unchanged; skipped write.");
        }

        var settings = BuildSettings(request);
        var staticBytes = System.Text.Encoding.UTF8.GetBytes(render.StaticConfigYaml);
        var dynamicBytes = System.Text.Encoding.UTF8.GetBytes(render.DynamicHttpYaml);

        var staticBackup = await ssh.ReadFileAsync(
            settings, request.AuthMode, request.Password, request.PrivateKeyPem, request.PrivateKeyPassphrase,
            state.StaticConfigPath, cancellationToken);
        if (staticBackup.Succeeded && staticBackup.Content is not null)
        {
            state.LastBackupStaticYaml = System.Text.Encoding.UTF8.GetString(staticBackup.Content);
        }

        var dynamicBackup = await ssh.ReadFileAsync(
            settings, request.AuthMode, request.Password, request.PrivateKeyPem, request.PrivateKeyPassphrase,
            state.DynamicConfigPath, cancellationToken);
        if (dynamicBackup.Succeeded && dynamicBackup.Content is not null)
        {
            state.LastBackupDynamicYaml = System.Text.Encoding.UTF8.GetString(dynamicBackup.Content);
        }

        await ssh.RunCommandAsync(
            settings, request.AuthMode, request.Password, request.PrivateKeyPem, request.PrivateKeyPassphrase,
            $"mkdir -p $(dirname {Quote(state.StaticConfigPath)}) $(dirname {Quote(state.DynamicConfigPath)})",
            cancellationToken);

        var staticWrite = await WriteAsync(settings, request, state.StaticConfigPath, staticBytes, cancellationToken);
        if (!staticWrite.Succeeded)
        {
            return new TraefikApplyResponse(false, render.ContentHash, false, staticWrite.Error);
        }

        var dynamicWrite = await WriteAsync(settings, request, state.DynamicConfigPath, dynamicBytes, cancellationToken);
        if (!dynamicWrite.Succeeded)
        {
            return new TraefikApplyResponse(false, render.ContentHash, false, dynamicWrite.Error);
        }

        state.LastAppliedContentHash = render.ContentHash;
        state.LastAppliedAtUtc = DateTimeOffset.UtcNow;
        if (isNew)
        {
            db.TraefikHostStates.Add(state);
        }

        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("traefik", "config_applied", subjectType: "connection", subjectId: request.ConnectionId.ToString(), cancellationToken: cancellationToken);
        return new TraefikApplyResponse(true, render.ContentHash, false, null);
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
            ? "apk add --no-cache traefik || true"
            : "apt-get update && apt-get install -y traefik || true";
        var result = await ssh.RunCommandAsync(
            settings,
            request.AuthMode,
            request.Password,
            request.PrivateKeyPem,
            request.PrivateKeyPassphrase,
            installCommand,
            cancellationToken);
        return new TraefikInstallResponse(result.Succeeded, result.Error);
    }

    private async Task<Hashi.Core.Traefik.TraefikRenderResult> RenderCurrentAsync(CancellationToken cancellationToken)
    {
        var resources = await db.Resources.AsNoTracking().ToListAsync(cancellationToken);
        var defs = resources.Select(x => new Hashi.Core.Resources.ResourceDefinition(
            x.Id, x.Name, x.Slug, Enum.Parse<Hashi.Core.Resources.ResourceKind>(x.Kind, ignoreCase: true),
            x.Enabled, x.IsSystem, x.Domain, x.TargetScheme, x.TargetHost, x.TargetPort)).ToList();
        return Hashi.Core.Traefik.TraefikConfigRenderer.Render(defs);
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
