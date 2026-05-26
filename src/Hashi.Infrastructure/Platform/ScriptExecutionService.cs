using System.Text;
using Hashi.Contracts.Api;
using Hashi.Core.Connections;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class ScriptExecutionService(
    HashiDbContext db,
    ISshRemoteExecutor ssh,
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
        var script = await db.Scripts.SingleOrDefaultAsync(x => x.Id == scriptId, cancellationToken)
            ?? throw new InvalidOperationException("Script not found.");
        var settings = new SshConnectionSettings(
            request.Host,
            request.Port <= 0 ? 22 : request.Port,
            request.Username,
            OsFamily.Unknown,
            null,
            null);
        var remotePath = $"/tmp/hashi-script-{script.Id:N}.sh";
        var write = request.AuthMode switch
        {
            "password" when !string.IsNullOrWhiteSpace(request.Password) =>
                await ssh.WriteAtomicAsync(settings, request.Password, remotePath, Encoding.UTF8.GetBytes(script.Body), cancellationToken),
            "private_key" when !string.IsNullOrWhiteSpace(request.PrivateKeyPem) =>
                await ssh.WriteAtomicWithPrivateKeyAsync(
                    settings, request.PrivateKeyPem, request.PrivateKeyPassphrase, remotePath, Encoding.UTF8.GetBytes(script.Body), cancellationToken),
            _ => new RemoteWriteResult(false, remotePath, "Unsupported auth mode."),
        };
        if (!write.Succeeded)
        {
            return new RunScriptResponse(false, string.Empty, write.Error);
        }

        var run = await ssh.RunCommandAsync(
            settings,
            request.AuthMode,
            request.Password,
            request.PrivateKeyPem,
            request.PrivateKeyPassphrase,
            $"chmod +x '{remotePath}' && bash '{remotePath}'",
            cancellationToken);
        script.LastRunAtUtc = DateTimeOffset.UtcNow;
        script.LastRunOutput = run.Output;
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("scripts", run.Succeeded ? "script_executed" : "script_failed", subjectType: "script", subjectId: script.Id.ToString(), cancellationToken: cancellationToken);
        return new RunScriptResponse(run.Succeeded, run.Output, run.Error);
    }
}
