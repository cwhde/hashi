using System.Text;
using System.Text.RegularExpressions;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Core.Connections;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Connections;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed partial class ScriptExecutionService(
    HashiDbContext db,
    ISshRemoteExecutor ssh,
    SecretRecordService secrets,
    AuditService audit)
{
    private const string ScriptDirectory = "/opt/hashi/scripts";

    public async Task<IReadOnlyList<ScriptResponse>> ListAsync(CancellationToken cancellationToken = default)
    {
        var scripts = await db.Scripts.AsNoTracking().OrderBy(x => x.Name).ToListAsync(cancellationToken);
        var responses = new List<ScriptResponse>(scripts.Count);
        foreach (var script in scripts)
        {
            responses.Add(await ToResponseAsync(script, cancellationToken));
        }

        return responses;
    }

    public async Task<ScriptResponse?> GetAsync(Guid scriptId, CancellationToken cancellationToken = default)
    {
        var script = await db.Scripts.AsNoTracking().SingleOrDefaultAsync(x => x.Id == scriptId, cancellationToken);
        return script is null ? null : await ToResponseAsync(script, cancellationToken);
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

        if (request.RunTimeoutSeconds is int runTimeoutSeconds)
        {
            script.RunTimeoutSeconds = ValidateRunTimeout(runTimeoutSeconds);
        }

        if (request.TargetConnectionIds is not null)
        {
            await ReplaceTargetsAsync(script.Id, request.TargetConnectionIds, cancellationToken);
        }

        if (request.EnvironmentVariables is not null)
        {
            await ReplaceEnvironmentVariablesAsync(script.Id, script.Name, request.EnvironmentVariables, cancellationToken);
        }

        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("scripts", "script_updated", subjectType: "script", subjectId: script.Id.ToString(), cancellationToken: cancellationToken);
        return await ToResponseAsync(script, cancellationToken);
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
        var targets = NormalizeTargets(request.ConnectionId, request.TargetConnectionIds);
        await ValidateTargetConnectionsAsync([request.ConnectionId], cancellationToken);
        await ValidateTargetConnectionsAsync(targets, cancellationToken);

        var entity = new ScriptEntity
        {
            ConnectionId = request.ConnectionId,
            Name = request.Name,
            Description = request.Description,
            Body = request.Body,
            CronExpression = request.CronExpression,
            RunTimeoutSeconds = ValidateRunTimeout(request.RunTimeoutSeconds),
        };
        db.Scripts.Add(entity);

        foreach (var connectionId in targets)
        {
            db.ScriptTargets.Add(new ScriptTargetEntity { ScriptId = entity.Id, ConnectionId = connectionId });
        }

        if (request.EnvironmentVariables is not null)
        {
            await AddEnvironmentVariablesAsync(entity.Id, entity.Name, request.EnvironmentVariables, cancellationToken);
        }

        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("scripts", "script_created", subjectType: "script", subjectId: entity.Id.ToString(), cancellationToken: cancellationToken);
        return await ToResponseAsync(entity, cancellationToken);
    }

    public async Task<RunScriptResponse> RunAsync(Guid scriptId, RunScriptRequest request, CancellationToken cancellationToken = default)
    {
        if (!string.IsNullOrWhiteSpace(request.Host) || !string.IsNullOrWhiteSpace(request.Username))
        {
            throw new InvalidOperationException("Scripts can only run against configured target connections.");
        }

        return await RunWithConnectionAsync(scriptId, cancellationToken);
    }

    public async Task<RunScriptResponse> RunWithConnectionAsync(Guid scriptId, CancellationToken cancellationToken = default)
    {
        var script = await db.Scripts.SingleOrDefaultAsync(x => x.Id == scriptId, cancellationToken)
            ?? throw new InvalidOperationException("Script not found.");
        var targetConnectionIds = await GetEnabledTargetConnectionIdsAsync(script, cancellationToken);
        await ValidateTargetConnectionsAsync(targetConnectionIds, cancellationToken);

        var results = new List<RunScriptResponse>(targetConnectionIds.Count);
        foreach (var connectionId in targetConnectionIds)
        {
            var connection = await db.Connections.SingleAsync(x => x.Id == connectionId, cancellationToken);
            results.Add(await ExecuteOnConnectionAsync(script, connection, cancellationToken));
        }

        var succeeded = results.All(x => x.Succeeded);
        var output = string.Join(Environment.NewLine, results.Select(x => x.Output).Where(x => !string.IsNullOrWhiteSpace(x)));
        var error = string.Join(Environment.NewLine, results.Select(x => x.Error).Where(x => !string.IsNullOrWhiteSpace(x)));
        var runs = results.SelectMany(x => x.Runs ?? Array.Empty<ScriptRunResponse>()).ToList();
        return new RunScriptResponse(
            succeeded,
            output,
            string.IsNullOrWhiteSpace(error) ? null : error,
            succeeded ? ScriptRunStatusNames.Succeeded : ScriptRunStatusNames.Failed,
            runs.Count == 1 ? runs[0].Id : null,
            runs);
    }

    public async Task SyncAllEnabledScriptsAsync(CancellationToken cancellationToken = default)
    {
        var scripts = await db.Scripts.Where(x => x.Enabled).ToListAsync(cancellationToken);
        foreach (var script in scripts)
        {
            try
            {
                var targetConnectionIds = await GetEnabledTargetConnectionIdsAsync(script, cancellationToken);
                await ValidateTargetConnectionsAsync(targetConnectionIds, cancellationToken);
                foreach (var connectionId in targetConnectionIds)
                {
                    var connection = await db.Connections.SingleAsync(x => x.Id == connectionId, cancellationToken);
                    await DeployScriptAsync(script, connection, cancellationToken);
                }
            }
            catch (Exception)
            {
                // Keep cron loop resilient; individual runs will surface errors.
            }
        }
    }

    private async Task<RunScriptResponse> ExecuteOnConnectionAsync(
        ScriptEntity script,
        ConnectionEntity connection,
        CancellationToken cancellationToken)
    {
        var run = new ScriptRunEntity
        {
            ScriptId = script.Id,
            ConnectionId = connection.Id,
            Status = ScriptRunStatusNames.Running,
        };
        db.ScriptRuns.Add(run);
        await db.SaveChangesAsync(cancellationToken);

        var redactValues = new List<string>();
        try
        {
            var credentials = await ConnectionSshCredentialResolver.ResolveAsync(connection, secrets, cancellationToken);
            if (credentials is null)
            {
                return await CompleteRunAsync(script, run, false, string.Empty, "SSH credentials unavailable; unlock vault or configure service-sync vault.", redactValues, cancellationToken);
            }

            var envVars = await ResolveEnvironmentVariablesAsync(script.Id, redactValues, cancellationToken);
            if (envVars is null)
            {
                return await CompleteRunAsync(script, run, false, string.Empty, "Script environment secrets unavailable; unlock vault or configure service-sync vault.", redactValues, cancellationToken);
            }

            await DeployScriptAsync(script, connection, cancellationToken);
            return await ExecuteRemoteAsync(script, run, credentials, envVars, redactValues, cancellationToken);
        }
        catch (Exception ex)
        {
            return await CompleteRunAsync(script, run, false, string.Empty, ex.Message, redactValues, cancellationToken);
        }
    }

    private async Task DeployScriptAsync(ScriptEntity script, ConnectionEntity connection, CancellationToken cancellationToken)
    {
        var credentials = await ConnectionSshCredentialResolver.ResolveAsync(connection, secrets, cancellationToken)
            ?? throw new InvalidOperationException("SSH credentials unavailable; unlock vault or configure service-sync vault.");
        var remotePath = RemotePath(script);

        var prepare = await ssh.RunCommandAsync(
            credentials.Settings,
            credentials.AuthMode,
            credentials.Password,
            credentials.PrivateKeyPem,
            credentials.PrivateKeyPassphrase,
            $"install -d -o root -g root -m 0750 {ShellQuote(ScriptDirectory)}",
            cancellationToken);
        if (!prepare.Succeeded)
        {
            throw new InvalidOperationException(prepare.Error ?? "Failed to prepare script directory.");
        }

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

        var harden = await ssh.RunCommandAsync(
            credentials.Settings,
            credentials.AuthMode,
            credentials.Password,
            credentials.PrivateKeyPem,
            credentials.PrivateKeyPassphrase,
            $"chown root:root {ShellQuote(remotePath)} && chmod 0750 {ShellQuote(remotePath)}",
            cancellationToken);
        if (!harden.Succeeded)
        {
            throw new InvalidOperationException(harden.Error ?? "Failed to harden script permissions.");
        }
    }

    private async Task<RunScriptResponse> ExecuteRemoteAsync(
        ScriptEntity script,
        ScriptRunEntity run,
        ResolvedSshCredentials credentials,
        IReadOnlyDictionary<string, string> environment,
        IReadOnlyList<string> redactValues,
        CancellationToken cancellationToken)
    {
        var command = BuildRunCommand(script, environment);
        var result = await ssh.RunCommandAsync(
            credentials.Settings,
            credentials.AuthMode,
            credentials.Password,
            credentials.PrivateKeyPem,
            credentials.PrivateKeyPassphrase,
            command,
            cancellationToken);

        return await CompleteRunAsync(script, run, result.Succeeded, result.Output, result.Error, redactValues, cancellationToken);
    }

    private async Task<RunScriptResponse> CompleteRunAsync(
        ScriptEntity script,
        ScriptRunEntity run,
        bool succeeded,
        string output,
        string? error,
        IReadOnlyList<string> redactValues,
        CancellationToken cancellationToken)
    {
        var redactedOutput = Redact(output, redactValues);
        var redactedError = error is null ? null : Redact(error, redactValues);
        var status = succeeded ? ScriptRunStatusNames.Succeeded : ScriptRunStatusNames.Failed;
        run.CompletedAtUtc = DateTimeOffset.UtcNow;
        run.Succeeded = succeeded;
        run.Status = status;
        run.Error = redactedError;
        db.ScriptOutputs.Add(new ScriptOutputEntity
        {
            RunId = run.Id,
            Stream = ScriptOutputStreamNames.Stdout,
            Content = redactedOutput,
        });
        if (!string.IsNullOrWhiteSpace(redactedError))
        {
            db.ScriptOutputs.Add(new ScriptOutputEntity
            {
                RunId = run.Id,
                Stream = ScriptOutputStreamNames.Stderr,
                Content = redactedError,
            });
        }

        script.LastRunAtUtc = run.CompletedAtUtc;
        script.LastRunOutput = redactedOutput;
        script.LastRunError = redactedError;
        script.LastRunStatus = status;
        script.LastRunId = run.Id;
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("scripts", succeeded ? "script_executed" : "script_failed", subjectType: "script", subjectId: script.Id.ToString(), cancellationToken: cancellationToken);
        return new RunScriptResponse(
            succeeded,
            redactedOutput,
            redactedError,
            status,
            run.Id,
            [ToRunResponse(run)]);
    }

    private async Task<IReadOnlyDictionary<string, string>?> ResolveEnvironmentVariablesAsync(
        Guid scriptId,
        ICollection<string> redactValues,
        CancellationToken cancellationToken)
    {
        var entities = await db.ScriptEnvironmentVariables.AsNoTracking()
            .Where(x => x.ScriptId == scriptId)
            .OrderBy(x => x.Name)
            .ToListAsync(cancellationToken);
        var values = new Dictionary<string, string>(StringComparer.Ordinal);
        foreach (var entity in entities)
        {
            if (entity.IsSecret)
            {
                if (entity.SecretId is null)
                {
                    return null;
                }

                var plaintext = await secrets.DecryptForPurposeAsync(entity.SecretId.Value, cancellationToken);
                if (plaintext is null)
                {
                    return null;
                }

                var secretValue = Encoding.UTF8.GetString(plaintext);
                values[entity.Name] = secretValue;
                if (!string.IsNullOrEmpty(secretValue))
                {
                    redactValues.Add(secretValue);
                }
            }
            else
            {
                values[entity.Name] = entity.PlainValue ?? string.Empty;
            }
        }

        return values;
    }

    private async Task ReplaceTargetsAsync(Guid scriptId, IReadOnlyList<Guid> targetConnectionIds, CancellationToken cancellationToken)
    {
        var targets = NormalizeTargets(null, targetConnectionIds);
        await ValidateTargetConnectionsAsync(targets, cancellationToken);
        db.ScriptTargets.RemoveRange(db.ScriptTargets.Where(x => x.ScriptId == scriptId));
        foreach (var connectionId in targets)
        {
            db.ScriptTargets.Add(new ScriptTargetEntity { ScriptId = scriptId, ConnectionId = connectionId });
        }
    }

    private async Task ReplaceEnvironmentVariablesAsync(
        Guid scriptId,
        string scriptName,
        IReadOnlyList<ScriptEnvironmentVariableRequest> environmentVariables,
        CancellationToken cancellationToken)
    {
        db.ScriptEnvironmentVariables.RemoveRange(db.ScriptEnvironmentVariables.Where(x => x.ScriptId == scriptId));
        await AddEnvironmentVariablesAsync(scriptId, scriptName, environmentVariables, cancellationToken);
    }

    private async Task AddEnvironmentVariablesAsync(
        Guid scriptId,
        string scriptName,
        IReadOnlyList<ScriptEnvironmentVariableRequest> environmentVariables,
        CancellationToken cancellationToken)
    {
        var seen = new HashSet<string>(StringComparer.Ordinal);
        foreach (var request in environmentVariables)
        {
            ValidateEnvironmentName(request.Name);
            if (!seen.Add(request.Name))
            {
                throw new InvalidOperationException($"Duplicate script environment variable '{request.Name}'.");
            }

            Guid? secretId = request.SecretId;
            string? plainValue = request.Value;
            if (request.IsSecret)
            {
                if (secretId is null)
                {
                    if (request.Value is null)
                    {
                        throw new InvalidOperationException($"Secret script environment variable '{request.Name}' requires a secret id or value.");
                    }

                    var descriptor = await secrets.StoreAsync(
                        SecretPurpose.ScriptEnvironment,
                        $"Script {scriptName} environment {request.Name}",
                        Encoding.UTF8.GetBytes(request.Value),
                        cancellationToken);
                    secretId = descriptor.Id;
                }

                plainValue = null;
            }

            db.ScriptEnvironmentVariables.Add(new ScriptEnvironmentVariableEntity
            {
                ScriptId = scriptId,
                Name = request.Name,
                IsSecret = request.IsSecret,
                PlainValue = plainValue,
                SecretId = secretId,
            });
        }
    }

    private async Task<IReadOnlyList<Guid>> GetEnabledTargetConnectionIdsAsync(ScriptEntity script, CancellationToken cancellationToken)
    {
        var targets = await db.ScriptTargets.AsNoTracking()
            .Where(x => x.ScriptId == script.Id && x.Enabled)
            .OrderBy(x => x.ConnectionId)
            .Select(x => x.ConnectionId)
            .ToListAsync(cancellationToken);
        return targets.Count > 0 ? targets : [script.ConnectionId];
    }

    private async Task ValidateTargetConnectionsAsync(IReadOnlyList<Guid> connectionIds, CancellationToken cancellationToken)
    {
        if (connectionIds.Count == 0)
        {
            throw new InvalidOperationException("At least one script target connection is required.");
        }

        var distinctIds = connectionIds.Distinct().ToList();
        var connections = await db.Connections.AsNoTracking()
            .Where(x => distinctIds.Contains(x.Id))
            .ToListAsync(cancellationToken);
        var foundIds = connections.Select(x => x.Id).ToHashSet();
        var missingId = distinctIds.FirstOrDefault(x => !foundIds.Contains(x));
        if (missingId != Guid.Empty)
        {
            throw new InvalidOperationException($"Script target connection '{missingId}' was not found.");
        }

        var invalid = connections.FirstOrDefault(x => x.Type != ConnectionTypeNames.FirewallHost);
        if (invalid is not null)
        {
            throw new InvalidOperationException($"Script target connection '{invalid.Id}' must be a firewall host.");
        }
    }

    private async Task<ScriptResponse> ToResponseAsync(ScriptEntity entity, CancellationToken cancellationToken)
    {
        var targets = await db.ScriptTargets.AsNoTracking()
            .Where(x => x.ScriptId == entity.Id)
            .OrderBy(x => x.ConnectionId)
            .Select(x => new ScriptTargetResponse(x.Id, x.ConnectionId, x.Enabled))
            .ToListAsync(cancellationToken);
        if (targets.Count == 0)
        {
            targets.Add(new ScriptTargetResponse(Guid.Empty, entity.ConnectionId, true));
        }

        var environment = await db.ScriptEnvironmentVariables.AsNoTracking()
            .Where(x => x.ScriptId == entity.Id)
            .OrderBy(x => x.Name)
            .Select(x => new ScriptEnvironmentVariableResponse(x.Id, x.Name, x.IsSecret, x.SecretId))
            .ToListAsync(cancellationToken);

        return new ScriptResponse(
            entity.Id,
            entity.ConnectionId,
            entity.Name,
            entity.Enabled,
            entity.Description,
            entity.CronExpression,
            entity.RunTimeoutSeconds,
            entity.LastRunAtUtc,
            entity.LastRunOutput,
            entity.LastRunError,
            entity.LastRunStatus,
            entity.LastRunId,
            targets,
            environment);
    }

    private static ScriptRunResponse ToRunResponse(ScriptRunEntity entity) => new(
        entity.Id,
        entity.ScriptId,
        entity.ConnectionId,
        entity.StartedAtUtc,
        entity.CompletedAtUtc,
        entity.Status,
        entity.Succeeded,
        entity.Error);

    private static IReadOnlyList<Guid> NormalizeTargets(Guid? connectionId, IReadOnlyList<Guid>? targetConnectionIds)
    {
        var targets = (targetConnectionIds is { Count: > 0 } ? targetConnectionIds : connectionId is Guid id ? [id] : [])
            .Where(x => x != Guid.Empty)
            .Distinct()
            .ToList();
        return targets;
    }

    private static int ValidateRunTimeout(int seconds)
    {
        if (seconds is < 1 or > 86400)
        {
            throw new InvalidOperationException("Script run timeout must be between 1 and 86400 seconds.");
        }

        return seconds;
    }

    private static void ValidateEnvironmentName(string name)
    {
        if (!EnvironmentNameRegex().IsMatch(name))
        {
            throw new InvalidOperationException($"Invalid script environment variable name '{name}'.");
        }
    }

    private static string BuildRunCommand(ScriptEntity script, IReadOnlyDictionary<string, string> environment)
    {
        var prefix = environment.Count == 0
            ? string.Empty
            : string.Join(" ", environment.Select(x => $"{x.Key}={ShellQuote(x.Value)}")) + " ";
        return $"timeout {script.RunTimeoutSeconds} env {prefix}bash {ShellQuote(RemotePath(script))}";
    }

    private static string Redact(string value, IReadOnlyList<string> secretsToRedact)
    {
        var redacted = value;
        foreach (var secret in secretsToRedact.Where(x => !string.IsNullOrEmpty(x)).Distinct())
        {
            redacted = redacted.Replace(secret, "[REDACTED]", StringComparison.Ordinal);
        }

        return redacted;
    }

    private static string RemotePath(ScriptEntity script) => $"{ScriptDirectory}/{script.Id:N}.sh";

    private static string ShellQuote(string value) => $"'{value.Replace("'", "'\"'\"'", StringComparison.Ordinal)}'";

    [GeneratedRegex("^[A-Za-z_][A-Za-z0-9_]*$", RegexOptions.CultureInvariant)]
    private static partial Regex EnvironmentNameRegex();
}
