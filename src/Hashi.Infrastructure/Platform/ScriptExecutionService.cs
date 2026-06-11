using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
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
    AuditService audit,
    ConnectionTargetResolver targetResolver)
{
    private const string ScriptDirectory = "/opt/hashi/scripts";
    private const string ManifestPath = $"{ScriptDirectory}/manifest.json";
    private const string CronPath = "/etc/cron.d/hashi-scripts";
    private static readonly JsonSerializerOptions ManifestJsonOptions = new(JsonSerializerDefaults.Web)
    {
        WriteIndented = true,
    };

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
        await ValidateTargetConnectionsAsync([request.ConnectionId], cancellationToken);
        var targets = NormalizeTargets(request.TargetConnectionIds);
        if (targets.Count > 0)
        {
            await ValidateTargetConnectionsAsync(targets, cancellationToken);
        }

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
        var scripts = await db.Scripts.AsNoTracking()
            .Where(x => x.Enabled)
            .OrderBy(x => x.Name)
            .ToListAsync(cancellationToken);
        var targetConnections = await db.Connections.AsNoTracking()
            .Where(x => x.Type == ConnectionTypeNames.FirewallHost && x.Enabled)
            .OrderBy(x => x.Name)
            .ToListAsync(cancellationToken);
        var scriptsByConnection = new Dictionary<Guid, List<ScriptEntity>>();
        foreach (var connection in targetConnections)
        {
            scriptsByConnection[connection.Id] = [];
        }

        foreach (var script in scripts)
        {
            try
            {
                var targetConnectionIds = await GetEnabledTargetConnectionIdsAsync(script, cancellationToken);
                await ValidateTargetConnectionsAsync(targetConnectionIds, cancellationToken);
                foreach (var connectionId in targetConnectionIds)
                {
                    if (scriptsByConnection.TryGetValue(connectionId, out var hostScripts))
                    {
                        hostScripts.Add(script);
                    }
                }
            }
            catch (Exception)
            {
                // Keep cron loop resilient; individual runs will surface errors.
            }
        }

        foreach (var connection in targetConnections)
        {
            try
            {
                await SyncHostScriptsAsync(connection, scriptsByConnection[connection.Id], cancellationToken);
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
            var credentials = await ConnectionSshCredentialResolver.ResolveAsync(connection, secrets, targetResolver, cancellationToken);
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
        var credentials = await ConnectionSshCredentialResolver.ResolveAsync(connection, secrets, targetResolver, cancellationToken)
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
                await ssh.WriteAtomicAsync(
                    credentials.Settings,
                    credentials.Password,
                    remotePath,
                    Encoding.UTF8.GetBytes(script.Body),
                    cancellationToken,
                    "bash -n {path}"),
            "private_key" when !string.IsNullOrWhiteSpace(credentials.PrivateKeyPem) =>
                await ssh.WriteAtomicWithPrivateKeyAsync(
                    credentials.Settings,
                    credentials.PrivateKeyPem,
                    credentials.PrivateKeyPassphrase,
                    remotePath,
                    Encoding.UTF8.GetBytes(script.Body),
                    cancellationToken,
                    "bash -n {path}"),
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

    private async Task SyncHostScriptsAsync(
        ConnectionEntity connection,
        IReadOnlyList<ScriptEntity> scripts,
        CancellationToken cancellationToken)
    {
        var credentials = await ConnectionSshCredentialResolver.ResolveAsync(connection, secrets, targetResolver, cancellationToken)
            ?? throw new InvalidOperationException("SSH credentials unavailable; unlock vault or configure service-sync vault.");

        await RunRequiredCommandAsync(
            credentials,
            $"install -d -o root -g root -m 0750 {ShellQuote(ScriptDirectory)} && install -d -o root -g root -m 0750 /var/log/hashi/scripts && install -d -o root -g root -m 0755 /etc/cron.d",
            "Failed to prepare script sync directories.",
            cancellationToken);

        foreach (var script in scripts)
        {
            await WriteRemoteFileAsync(credentials, RemotePath(script), Encoding.UTF8.GetBytes(script.Body), cancellationToken);
            await RunRequiredCommandAsync(
                credentials,
                $"chown root:root {ShellQuote(RemotePath(script))} && chmod 0750 {ShellQuote(RemotePath(script))}",
                "Failed to harden script permissions.",
                cancellationToken);
        }

        await WriteRemoteFileAsync(credentials, ManifestPath, Encoding.UTF8.GetBytes(RenderManifest(scripts)), cancellationToken);
        await RunRequiredCommandAsync(
            credentials,
            $"chown root:root {ShellQuote(ManifestPath)} && chmod 0640 {ShellQuote(ManifestPath)}",
            "Failed to harden script manifest.",
            cancellationToken);

        var checkSystemd = await ssh.RunCommandAsync(
            credentials.Settings,
            credentials.AuthMode,
            credentials.Password,
            credentials.PrivateKeyPem,
            credentials.PrivateKeyPassphrase,
            "[ -d /run/systemd/system ]",
            cancellationToken);

        bool useSystemd = checkSystemd.Succeeded;

        if (useSystemd)
        {
            var activeTimerScriptIds = new HashSet<Guid>();

            foreach (var script in scripts)
            {
                if (script.Enabled && !string.IsNullOrWhiteSpace(script.CronExpression))
                {
                    var onCalendar = ConvertCronToOnCalendar(script.CronExpression);
                    if (onCalendar != null)
                    {
                        var servicePath = $"/etc/systemd/system/hashi-script-{script.Id:N}.service";
                        var timerPath = $"/etc/systemd/system/hashi-script-{script.Id:N}.timer";

                        var serviceContent = RenderSystemdService(script);
                        var timerContent = RenderSystemdTimer(script, onCalendar);

                        await WriteRemoteFileAsync(credentials, servicePath, Encoding.UTF8.GetBytes(serviceContent), cancellationToken);
                        await WriteRemoteFileAsync(credentials, timerPath, Encoding.UTF8.GetBytes(timerContent), cancellationToken);

                        await RunRequiredCommandAsync(
                            credentials,
                            $"chown root:root {ShellQuote(servicePath)} {ShellQuote(timerPath)} && chmod 0644 {ShellQuote(servicePath)} {ShellQuote(timerPath)}",
                            $"Failed to harden systemd files for script {script.Name}.",
                            cancellationToken);

                        await RunRequiredCommandAsync(
                            credentials,
                            $"systemctl daemon-reload && systemctl enable --now hashi-script-{script.Id:N}.timer",
                            $"Failed to enable and start systemd timer for script {script.Name}.",
                            cancellationToken);

                        activeTimerScriptIds.Add(script.Id);
                    }
                }
            }

            // Clean up obsolete timers
            var findTimers = await ssh.RunCommandAsync(
                credentials.Settings,
                credentials.AuthMode,
                credentials.Password,
                credentials.PrivateKeyPem,
                credentials.PrivateKeyPassphrase,
                "find /etc/systemd/system/ -name 'hashi-script-*.timer' 2>/dev/null",
                cancellationToken);

            if (findTimers.Succeeded && !string.IsNullOrWhiteSpace(findTimers.Output))
            {
                var lines = findTimers.Output.Split('\n', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries);
                foreach (var line in lines)
                {
                    var fileName = Path.GetFileName(line);
                    var match = SystemdTimerRegex().Match(fileName);
                    if (match.Success && match.Groups[1].Value is string idHex && Guid.TryParse(idHex, out var scriptId))
                    {
                        if (!activeTimerScriptIds.Contains(scriptId))
                        {
                            var servicePath = $"/etc/systemd/system/hashi-script-{idHex}.service";
                            var timerPath = $"/etc/systemd/system/hashi-script-{idHex}.timer";

                            await ssh.RunCommandAsync(
                                credentials.Settings,
                                credentials.AuthMode,
                                credentials.Password,
                                credentials.PrivateKeyPem,
                                credentials.PrivateKeyPassphrase,
                                $"systemctl disable --now hashi-script-{idHex}.timer && rm -f {ShellQuote(timerPath)} {ShellQuote(servicePath)}",
                                cancellationToken);
                        }
                    }
                }
            }

            // Final daemon reload and clean up legacy cron
            await RunRequiredCommandAsync(
                credentials,
                $"systemctl daemon-reload && systemctl reset-failed && rm -f {ShellQuote(CronPath)}",
                "Failed to clean up legacy cron and finalize systemd reload.",
                cancellationToken);
        }
        else
        {
            await WriteRemoteFileAsync(credentials, CronPath, Encoding.UTF8.GetBytes(RenderCron(scripts)), cancellationToken);
            await RunRequiredCommandAsync(
                credentials,
                $"chown root:root {ShellQuote(CronPath)} && chmod 0644 {ShellQuote(CronPath)}",
                "Failed to harden legacy cron file.",
                cancellationToken);
        }
    }

    private async Task WriteRemoteFileAsync(
        ResolvedSshCredentials credentials,
        string remotePath,
        byte[] content,
        CancellationToken cancellationToken)
    {
        var write = credentials.AuthMode switch
        {
            "password" when !string.IsNullOrWhiteSpace(credentials.Password) =>
                await ssh.WriteAtomicAsync(credentials.Settings, credentials.Password, remotePath, content, cancellationToken),
            "private_key" when !string.IsNullOrWhiteSpace(credentials.PrivateKeyPem) =>
                await ssh.WriteAtomicWithPrivateKeyAsync(
                    credentials.Settings,
                    credentials.PrivateKeyPem,
                    credentials.PrivateKeyPassphrase,
                    remotePath,
                    content,
                    cancellationToken),
            _ => new RemoteWriteResult(false, remotePath, "Unsupported auth mode."),
        };
        if (!write.Succeeded)
        {
            throw new InvalidOperationException(write.Error ?? $"Failed to write {remotePath}.");
        }
    }

    private async Task RunRequiredCommandAsync(
        ResolvedSshCredentials credentials,
        string command,
        string errorMessage,
        CancellationToken cancellationToken)
    {
        var result = await ssh.RunCommandAsync(
            credentials.Settings,
            credentials.AuthMode,
            credentials.Password,
            credentials.PrivateKeyPem,
            credentials.PrivateKeyPassphrase,
            command,
            cancellationToken);
        if (!result.Succeeded)
        {
            throw new InvalidOperationException(result.Error ?? errorMessage);
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
        var targets = NormalizeTargets(targetConnectionIds);
        if (targets.Count > 0)
        {
            await ValidateTargetConnectionsAsync(targets, cancellationToken);
        }

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
        return targets.Count > 0 ? targets : await GetDefaultTargetConnectionIdsAsync(cancellationToken);
    }

    private async Task<IReadOnlyList<Guid>> GetDefaultTargetConnectionIdsAsync(CancellationToken cancellationToken)
    {
        return await db.Connections.AsNoTracking()
            .Where(x => x.Type == ConnectionTypeNames.FirewallHost && x.Enabled)
            .OrderBy(x => x.Name)
            .Select(x => x.Id)
            .ToListAsync(cancellationToken);
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
            .ToListAsync(cancellationToken);

        var defaultIds = targets.Count == 0
            ? await GetDefaultTargetConnectionIdsAsync(cancellationToken)
            : (IReadOnlyList<Guid>)Array.Empty<Guid>();

        var connectionIds = targets.Select(x => x.ConnectionId)
            .Concat(defaultIds)
            .Distinct()
            .ToList();

        var connectionNames = await db.Connections.AsNoTracking()
            .Where(x => connectionIds.Contains(x.Id))
            .ToDictionaryAsync(x => x.Id, x => x.Name, cancellationToken);

        var targetResponses = new List<ScriptTargetResponse>();
        if (targets.Count > 0)
        {
            foreach (var target in targets)
            {
                var name = connectionNames.TryGetValue(target.ConnectionId, out var n) ? n : "Unknown";
                targetResponses.Add(new ScriptTargetResponse(target.Id, target.ConnectionId, name, target.Enabled));
            }
        }
        else
        {
            foreach (var connectionId in defaultIds)
            {
                var name = connectionNames.TryGetValue(connectionId, out var n) ? n : "Unknown";
                targetResponses.Add(new ScriptTargetResponse(Guid.Empty, connectionId, name, true));
            }
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
            entity.Body,
            entity.CronExpression,
            entity.RunTimeoutSeconds,
            entity.LastRunAtUtc,
            entity.LastRunOutput,
            entity.LastRunError,
            entity.LastRunStatus,
            entity.LastRunId,
            targetResponses,
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

    internal static string RenderManifest(IReadOnlyList<ScriptEntity> scripts)
    {
        var entries = scripts
            .OrderBy(x => x.Name, StringComparer.OrdinalIgnoreCase)
            .Select(x => new ScriptManifestEntry(
                x.Id,
                x.Name,
                RemotePath(x),
                Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(x.Body))).ToLowerInvariant(),
                x.Enabled,
                string.IsNullOrWhiteSpace(x.CronExpression) ? null : x.CronExpression,
                x.RunTimeoutSeconds))
            .ToList();
        return JsonSerializer.Serialize(new ScriptManifest(entries), ManifestJsonOptions) + Environment.NewLine;
    }

    internal static string RenderCron(IReadOnlyList<ScriptEntity> scripts)
    {
        var builder = new StringBuilder()
            .AppendLine("# Hashi-managed script schedules. Do not edit by hand.")
            .AppendLine("SHELL=/bin/bash")
            .AppendLine("PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
            .AppendLine();

        foreach (var script in scripts
            .Where(x => x.Enabled && !string.IsNullOrWhiteSpace(x.CronExpression))
            .OrderBy(x => x.Name, StringComparer.OrdinalIgnoreCase))
        {
            builder
                .Append("# ")
                .AppendLine(SanitizeCronComment(script.Name))
                .Append(script.CronExpression.Trim())
                .Append(" root timeout ")
                .Append(script.RunTimeoutSeconds)
                .Append(" bash ")
                .Append(RemotePath(script))
                .Append(" >> /var/log/hashi/scripts/")
                .Append(script.Id.ToString("N"))
                .Append(".log 2>&1")
                .AppendLine()
                .AppendLine();
        }

        return builder.ToString();
    }

    private static IReadOnlyList<Guid> NormalizeTargets(IReadOnlyList<Guid>? targetConnectionIds)
    {
        var targets = (targetConnectionIds is { Count: > 0 } ? targetConnectionIds : [])
            .Where(x => x != Guid.Empty)
            .Distinct()
            .ToList();
        return targets;
    }

    private static string SanitizeCronComment(string value)
        => value.ReplaceLineEndings(" ").Replace("#", string.Empty, StringComparison.Ordinal).Trim();

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

    private sealed record ScriptManifest(IReadOnlyList<ScriptManifestEntry> Scripts);

    private sealed record ScriptManifestEntry(
        Guid Id,
        string Name,
        string Path,
        string Sha256,
        bool Enabled,
        string? CronExpression,
        int RunTimeoutSeconds);

    internal static string? ConvertCronToOnCalendar(string cronExpression)
    {
        if (string.IsNullOrWhiteSpace(cronExpression)) return null;

        var parts = cronExpression.Split(' ', StringSplitOptions.RemoveEmptyEntries);
        if (parts.Length != 5) return null;

        var min = parts[0];
        var hour = parts[1];
        var dom = parts[2];
        var mon = parts[3];
        var dow = parts[4];

        string? dowStr = null;
        if (dow != "*")
        {
            var dowParts = dow.Split(',');
            var mappedDows = new List<string>();
            foreach (var dp in dowParts)
            {
                if (dp.Contains('-'))
                {
                    var rangeParts = dp.Split('-');
                    if (rangeParts.Length == 2)
                    {
                        var start = MapSingleDow(rangeParts[0]);
                        var end = MapSingleDow(rangeParts[1]);
                        if (start != null && end != null)
                        {
                            mappedDows.Add($"{start}-{end}");
                            continue;
                        }
                    }
                    return null;
                }
                var single = MapSingleDow(dp);
                if (single != null)
                {
                    mappedDows.Add(single);
                }
                else
                {
                    return null;
                }
            }
            dowStr = string.Join(",", mappedDows);
        }

        var yearMonthDay = $"*-{MapCronField(mon)}-{MapCronField(dom)}";
        var time = $"{MapCronTimeField(hour)}:{MapCronTimeField(min)}:00";

        if (dowStr != null)
        {
            return $"{dowStr} {yearMonthDay} {time}";
        }
        return $"{yearMonthDay} {time}";
    }

    private static string? MapSingleDow(string value)
    {
        var clean = value.Trim().ToLowerInvariant();
        return clean switch
        {
            "0" or "7" or "sun" or "sunday" => "Sun",
            "1" or "mon" or "monday" => "Mon",
            "2" or "tue" or "tuesday" => "Tue",
            "3" or "wed" or "wednesday" => "Wed",
            "4" or "thu" or "thursday" => "Thu",
            "5" or "fri" or "friday" => "Fri",
            "6" or "sat" or "saturday" => "Sat",
            _ => null
        };
    }

    private static string MapCronField(string field)
    {
        if (field == "*") return "*";
        if (field.StartsWith("*/")) return field[2..];
        return field;
    }

    private static string MapCronTimeField(string field)
    {
        if (field == "*") return "*";
        if (field.StartsWith("*/")) return field;
        if (int.TryParse(field, out var val))
        {
            return val.ToString("D2");
        }
        return field;
    }

    internal static string RenderSystemdService(ScriptEntity script)
    {
        var builder = new StringBuilder()
            .AppendLine("[Unit]")
            .AppendLine($"Description=Hashi Script execution - {script.Name}")
            .AppendLine("RefuseManualStart=no")
            .AppendLine("RefuseManualStop=no")
            .AppendLine()
            .AppendLine("[Service]")
            .AppendLine("Type=oneshot")
            .AppendLine($"ExecStart=/bin/bash {RemotePath(script)}")
            .AppendLine("User=root")
            .AppendLine("Group=root")
            .AppendLine($"TimeoutStartSec={script.RunTimeoutSeconds}")
            .AppendLine($"StandardOutput=append:/var/log/hashi/scripts/{script.Id:N}.log")
            .AppendLine($"StandardError=append:/var/log/hashi/scripts/{script.Id:N}.log");

        return builder.ToString();
    }

    internal static string RenderSystemdTimer(ScriptEntity script, string onCalendar)
    {
        var builder = new StringBuilder()
            .AppendLine("[Unit]")
            .AppendLine($"Description=Hashi Script timer - {script.Name}")
            .AppendLine()
            .AppendLine("[Timer]")
            .AppendLine($"OnCalendar={onCalendar}")
            .AppendLine("Persistent=true")
            .AppendLine()
            .AppendLine("[Install]")
            .AppendLine("WantedBy=timers.target");

        return builder.ToString();
    }

    [GeneratedRegex(@"^hashi-script-([0-9a-fA-F]{32})\.timer$", RegexOptions.IgnoreCase | RegexOptions.CultureInvariant)]
    private static partial Regex SystemdTimerRegex();
}
