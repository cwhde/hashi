using System.Text;
using Hashi.Contracts.Api;
using Hashi.Core.Connections;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Connections;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class ScriptExecutionService(
    HashiDbContext db,
    ISshRemoteExecutor ssh,
    SecretRecordService secrets,
    AuditService audit)
{
    public async Task<IReadOnlyList<ScriptResponse>> ListAsync(CancellationToken cancellationToken = default)
    {
        var scripts = await db.Scripts.AsNoTracking().OrderBy(x => x.Name).ToListAsync(cancellationToken);
        return scripts.Select(ToResponse).ToList();
    }

    public async Task<ScriptResponse?> GetAsync(Guid scriptId, CancellationToken cancellationToken = default)
    {
        var script = await db.Scripts.AsNoTracking().SingleOrDefaultAsync(x => x.Id == scriptId, cancellationToken);
        return script is null ? null : ToResponse(script);
    }

    public async Task<ScriptResponse?> UpdateAsync(Guid scriptId, UpdateScriptRequest request, CancellationToken cancellationToken = default)
    {
        var script = await db.Scripts.SingleOrDefaultAsync(x => x.Id == scriptId, cancellationToken);
        if (script is null)
        {
            return null;
        }

        if (request.Name is not null)
        {
            script.Name = request.Name;
        }

        if (request.Description is not null)
        {
            script.Description = request.Description;
        }

        if (request.Body is not null)
        {
            script.Body = request.Body;
        }

        if (request.CronExpression is not null)
        {
            script.CronExpression = request.CronExpression;
        }

        if (request.Enabled is bool enabled)
        {
            script.Enabled = enabled;
        }

        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("scripts", "script_updated", subjectType: "script", subjectId: script.Id.ToString(), cancellationToken: cancellationToken);
        return ToResponse(script);
    }

    public async Task<bool> DeleteAsync(Guid scriptId, CancellationToken cancellationToken = default)
    {
        var script = await db.Scripts.SingleOrDefaultAsync(x => x.Id == scriptId, cancellationToken);
        if (script is null)
        {
            return false;
        }

        db.Scripts.Remove(script);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("scripts", "script_deleted", subjectType: "script", subjectId: scriptId.ToString(), cancellationToken: cancellationToken);
        return true;
    }

    public async Task<ScriptResponse> CreateAsync(CreateScriptRequest request, CancellationToken cancellationToken = default)
    {
        var entity = new ScriptEntity
        {
            ConnectionId = request.ConnectionId,
            Name = request.Name,
            Description = request.Description,
            Body = request.Body,
            CronExpression = request.CronExpression,
        };
        db.Scripts.Add(entity);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("scripts", "script_created", subjectType: "script", subjectId: entity.Id.ToString(), cancellationToken: cancellationToken);
        return ToResponse(entity);
    }

    public async Task<RunScriptResponse> RunAsync(Guid scriptId, RunScriptRequest request, CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(request.Host) || string.IsNullOrWhiteSpace(request.Username))
        {
            return await RunWithConnectionAsync(scriptId, cancellationToken);
        }

        var script = await db.Scripts.SingleOrDefaultAsync(x => x.Id == scriptId, cancellationToken)
            ?? throw new InvalidOperationException("Script not found.");
        var settings = new SshConnectionSettings(
            request.Host,
            request.Port <= 0 ? 22 : request.Port,
            request.Username,
            OsFamily.Unknown,
            null,
            null);
        return await ExecuteRemoteAsync(script, settings, request.AuthMode, request.Password, request.PrivateKeyPem, request.PrivateKeyPassphrase, cancellationToken: cancellationToken);
    }

    public async Task<RunScriptResponse> RunWithConnectionAsync(Guid scriptId, CancellationToken cancellationToken = default)
    {
        var script = await db.Scripts.SingleOrDefaultAsync(x => x.Id == scriptId, cancellationToken)
            ?? throw new InvalidOperationException("Script not found.");
        await DeployScriptAsync(script, cancellationToken);
        return await ExecuteOnConnectionAsync(script, cancellationToken);
    }

    public async Task SyncAllEnabledScriptsAsync(CancellationToken cancellationToken = default)
    {
        var scripts = await db.Scripts.Where(x => x.Enabled).ToListAsync(cancellationToken);
        foreach (var script in scripts)
        {
            try
            {
                await DeployScriptAsync(script, cancellationToken);
            }
            catch (Exception)
            {
                // Keep cron loop resilient; individual runs will surface errors.
            }
        }
    }

    private async Task<RunScriptResponse> ExecuteOnConnectionAsync(ScriptEntity script, CancellationToken cancellationToken)
    {
        var connection = await db.Connections.SingleOrDefaultAsync(x => x.Id == script.ConnectionId, cancellationToken)
            ?? throw new InvalidOperationException("Script connection not found.");
        var credentials = await ConnectionSshCredentialResolver.ResolveAsync(connection, secrets, cancellationToken);
        if (credentials is null)
        {
            return new RunScriptResponse(false, string.Empty, "SSH credentials unavailable; unlock vault or configure service-sync vault.");
        }

        return await ExecuteRemoteAsync(
            script,
            credentials.Settings,
            credentials.AuthMode,
            credentials.Password,
            credentials.PrivateKeyPem,
            credentials.PrivateKeyPassphrase,
            skipDeploy: true,
            cancellationToken);
    }

    private async Task DeployScriptAsync(ScriptEntity script, CancellationToken cancellationToken)
    {
        var connection = await db.Connections.SingleOrDefaultAsync(x => x.Id == script.ConnectionId, cancellationToken)
            ?? throw new InvalidOperationException("Script connection not found.");
        var credentials = await ConnectionSshCredentialResolver.ResolveAsync(connection, secrets, cancellationToken)
            ?? throw new InvalidOperationException("SSH credentials unavailable; unlock vault or configure service-sync vault.");
        var remotePath = $"/opt/hashi/scripts/{script.Id:N}.sh";
        var write = credentials.AuthMode switch
        {
            "password" when !string.IsNullOrWhiteSpace(credentials.Password) =>
                await ssh.WriteAtomicAsync(credentials.Settings, credentials.Password, remotePath, Encoding.UTF8.GetBytes(script.Body), cancellationToken),
            "private_key" when !string.IsNullOrWhiteSpace(credentials.PrivateKeyPem) =>
                await ssh.WriteAtomicWithPrivateKeyAsync(
                    credentials.Settings,
                    credentials.PrivateKeyPem,
                    credentials.PrivateKeyPassphrase,
                    remotePath,
                    Encoding.UTF8.GetBytes(script.Body),
                    cancellationToken),
            _ => new RemoteWriteResult(false, remotePath, "Unsupported auth mode."),
        };
        if (!write.Succeeded)
        {
            throw new InvalidOperationException(write.Error ?? "Failed to deploy script.");
        }

        await ssh.RunCommandAsync(
            credentials.Settings,
            credentials.AuthMode,
            credentials.Password,
            credentials.PrivateKeyPem,
            credentials.PrivateKeyPassphrase,
            $"mkdir -p /opt/hashi/scripts && chmod +x '{remotePath}'",
            cancellationToken);
    }

    private async Task<RunScriptResponse> ExecuteRemoteAsync(
        ScriptEntity script,
        SshConnectionSettings settings,
        string authMode,
        string? password,
        string? privateKeyPem,
        string? privateKeyPassphrase,
        bool skipDeploy = false,
        CancellationToken cancellationToken = default)
    {
        var remotePath = $"/opt/hashi/scripts/{script.Id:N}.sh";
        if (!skipDeploy)
        {
            var write = authMode switch
            {
                "password" when !string.IsNullOrWhiteSpace(password) =>
                    await ssh.WriteAtomicAsync(settings, password, remotePath, Encoding.UTF8.GetBytes(script.Body), cancellationToken),
                "private_key" when !string.IsNullOrWhiteSpace(privateKeyPem) =>
                    await ssh.WriteAtomicWithPrivateKeyAsync(
                        settings, privateKeyPem, privateKeyPassphrase, remotePath, Encoding.UTF8.GetBytes(script.Body), cancellationToken),
                _ => new RemoteWriteResult(false, remotePath, "Unsupported auth mode."),
            };
            if (!write.Succeeded)
            {
                return new RunScriptResponse(false, string.Empty, write.Error);
            }

            await ssh.RunCommandAsync(
                settings,
                authMode,
                password,
                privateKeyPem,
                privateKeyPassphrase,
                $"mkdir -p /opt/hashi/scripts && chmod +x '{remotePath}'",
                cancellationToken);
        }

        var run = await ssh.RunCommandAsync(
            settings,
            authMode,
            password,
            privateKeyPem,
            privateKeyPassphrase,
            $"bash '{remotePath}'",
            cancellationToken);
        script.LastRunAtUtc = DateTimeOffset.UtcNow;
        script.LastRunOutput = run.Output;
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("scripts", run.Succeeded ? "script_executed" : "script_failed", subjectType: "script", subjectId: script.Id.ToString(), cancellationToken: cancellationToken);
        return new RunScriptResponse(run.Succeeded, run.Output, run.Error);
    }

    public static ScriptResponse ToResponse(ScriptEntity entity) => new(
        entity.Id,
        entity.Name,
        entity.Enabled,
        entity.Description,
        entity.CronExpression,
        entity.LastRunAtUtc,
        entity.LastRunOutput);
}
