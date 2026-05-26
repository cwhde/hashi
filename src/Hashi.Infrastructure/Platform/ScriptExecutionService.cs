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
        return scripts.Select(x => new ScriptResponse(x.Id, x.Name, x.Enabled, x.Description)).ToList();
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
        return new ScriptResponse(entity.Id, entity.Name, entity.Enabled, entity.Description);
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
        return await ExecuteRemoteAsync(script, settings, request.AuthMode, request.Password, request.PrivateKeyPem, request.PrivateKeyPassphrase, cancellationToken);
    }

    public async Task<RunScriptResponse> RunWithConnectionAsync(Guid scriptId, CancellationToken cancellationToken = default)
    {
        var script = await db.Scripts.SingleOrDefaultAsync(x => x.Id == scriptId, cancellationToken)
            ?? throw new InvalidOperationException("Script not found.");
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
            cancellationToken);
    }

    private async Task<RunScriptResponse> ExecuteRemoteAsync(
        ScriptEntity script,
        SshConnectionSettings settings,
        string authMode,
        string? password,
        string? privateKeyPem,
        string? privateKeyPassphrase,
        CancellationToken cancellationToken)
    {
        var remotePath = $"/opt/hashi/scripts/{script.Id:N}.sh";
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
}
