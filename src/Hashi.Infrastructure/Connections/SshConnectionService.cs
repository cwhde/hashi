using Hashi.Core.Auth;
using Hashi.Core.Connections;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Connections;

public sealed class SshConnectionService(
    HashiDbContext db,
    ISshRemoteExecutor ssh,
    SecretRecordService secrets,
    AuditService audit,
    ConnectionTargetResolver targetResolver)
{
    public async Task<ConnectionEntity> CreateAsync(
        string name,
        string connectionType,
        SshConnectionSettings settings,
        string authMode,
        string? password,
        string? privateKeyPem,
        string? privateKeyPassphrase,
        ConnectionTargetRequest? targetRequest = null,
        CancellationToken cancellationToken = default)
    {
        var connection = new ConnectionEntity
        {
            Name = name,
            Type = connectionType,
            SettingsJson = System.Text.Json.JsonSerializer.Serialize(new
            {
                settings.Host,
                settings.Port,
                settings.Username,
                authMode,
                settings.ConfigPath,
                settings.DynamicPath,
            }),
        };
        db.Connections.Add(connection);

        if (targetRequest is not null)
        {
            var target = BuildSshTarget(connection.Id, settings, targetRequest);
            var resolved = await targetResolver.ResolveAsync(target, persistSnapshot: false, cancellationToken);
            if (resolved.Status == ConnectionTargetStatusNames.Failed)
            {
                throw new InvalidOperationException(resolved.Error ?? "Connection target could not be resolved.");
            }

            connection.SettingsJson = SerializeSettings(settings with
            {
                Host = resolved.ResolvedHost,
                Port = target.Port,
            }, authMode);
            target.ResolvedIpSnapshot = resolved.ResolvedIp;
            target.LastResolvedAtUtc = DateTimeOffset.UtcNow;
            target.Status = resolved.Status;
            target.LastError = resolved.Error;
            target.UpdatedAtUtc = DateTimeOffset.UtcNow;
            db.ConnectionTargets.Add(target);
        }

        var credentialPayload = ConnectionSshCredentialResolver.SerializeCredentialPayload(
            authMode, password, privateKeyPem, privateKeyPassphrase);
        var secret = await secrets.StoreAsync(
            SecretPurpose.SshCredential,
            $"SSH: {name}",
            credentialPayload,
            cancellationToken,
            serviceSyncEligible: RuntimeSecretEligibility.IsRuntimeSshConnectionType(connectionType));
        connection.SecretId = secret.Id;

        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("connections", "ssh_connection_created", subjectType: "connection", subjectId: connection.Id.ToString(), cancellationToken: cancellationToken);
        return connection;
    }

    public async Task<SshValidationResult> ValidateAsync(Guid connectionId, CancellationToken cancellationToken = default)
    {
        var connection = await db.Connections.SingleAsync(x => x.Id == connectionId, cancellationToken);
        connection.HealthState = ConnectionHealthStateNames.Validating;
        await db.SaveChangesAsync(cancellationToken);

        var credentials = await ConnectionSshCredentialResolver.ResolveAsync(connection, secrets, cancellationToken: cancellationToken)
            ?? throw new InvalidOperationException("SSH credentials unavailable for this connection.");
        return await ValidateWithCredentialsAsync(
            await ConnectionSshCredentialResolver.ResolveSettingsAsync(connection, targetResolver, cancellationToken),
            credentials.AuthMode,
            credentials.Password,
            credentials.PrivateKeyPem,
            credentials.PrivateKeyPassphrase,
            connectionId,
            cancellationToken);
    }

    public async Task<SshValidationResult> ValidateWithCredentialsAsync(
        SshConnectionSettings settings,
        string authMode,
        string? password,
        string? privateKeyPem,
        string? privateKeyPassphrase,
        Guid? connectionId = null,
        CancellationToken cancellationToken = default)
    {
        var result = authMode switch
        {
            "password" when !string.IsNullOrWhiteSpace(password) =>
                await ssh.ValidateAsync(settings, password, cancellationToken),
            "private_key" when !string.IsNullOrWhiteSpace(privateKeyPem) =>
                await ssh.ValidateWithPrivateKeyAsync(settings, privateKeyPem, privateKeyPassphrase, cancellationToken),
            _ => new SshValidationResult(false, OsFamily.Unknown, null, "Unsupported auth mode."),
        };

        if (connectionId is Guid id)
        {
            var connection = await db.Connections.SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
            if (connection is not null)
            {
                connection.HealthState = result.Succeeded
                    ? ConnectionHealthStateNames.Healthy
                    : ConnectionHealthStateNames.Failed;
                connection.LastValidationMessage = result.Error;
                connection.LastValidatedAtUtc = DateTimeOffset.UtcNow;
                await db.SaveChangesAsync(cancellationToken);
            }
        }

        return result;
    }

    public async Task<RemoteWriteResult> WriteAtomicAsync(
        Guid connectionId,
        SshConnectionSettings settings,
        string authMode,
        string? password,
        string? privateKeyPem,
        string? privateKeyPassphrase,
        string remotePath,
        ReadOnlyMemory<byte> content,
        CancellationToken cancellationToken = default)
    {
        var result = authMode switch
        {
            "password" when !string.IsNullOrWhiteSpace(password) =>
                await ssh.WriteAtomicAsync(settings, password, remotePath, content, cancellationToken),
            "private_key" when !string.IsNullOrWhiteSpace(privateKeyPem) =>
                await ssh.WriteAtomicWithPrivateKeyAsync(settings, privateKeyPem, privateKeyPassphrase, remotePath, content, cancellationToken),
            _ => new RemoteWriteResult(false, remotePath, "Unsupported auth mode."),
        };
        await audit.WriteAsync(
            "connections",
            result.Succeeded ? "remote_write" : "remote_write_failed",
            outcome: result.Succeeded ? "success" : "failure",
            subjectType: "connection",
            subjectId: connectionId.ToString(),
            cancellationToken: cancellationToken);
        return result;
    }

    public async Task<IReadOnlyList<ConnectionEntity>> ListAsync(string? type, CancellationToken cancellationToken = default)
    {
        var query = db.Connections.AsNoTracking();
        if (!string.IsNullOrWhiteSpace(type))
        {
            query = query.Where(x => x.Type == type);
        }

        return await query.OrderBy(x => x.Name).ToListAsync(cancellationToken);
    }

    private static SshConnectionSettings ParseSettings(ConnectionEntity connection)
        => ConnectionSshCredentialResolver.ParseSettings(connection);

    private static ConnectionTargetEntity BuildSshTarget(
        Guid connectionId,
        SshConnectionSettings settings,
        ConnectionTargetRequest request)
    {
        var targetMode = NormalizeTargetMode(request.TargetMode);
        var pulseIpMode = NormalizePulseIpMode(request.PulseIpMode);
        var port = request.Port is >= 1 and <= 65535 ? request.Port : settings.Port;
        if (port is < 1 or > 65535)
        {
            throw new InvalidOperationException("Connection target port is invalid.");
        }

        return new ConnectionTargetEntity
        {
            OwnerType = ConnectionTargetOwnerTypeNames.Connection,
            OwnerId = connectionId,
            TargetMode = targetMode,
            StaticHost = targetMode == ConnectionTargetModeNames.StaticHost ? request.StaticHost?.Trim() : null,
            StaticIp = targetMode == ConnectionTargetModeNames.StaticIp ? request.StaticIp?.Trim() : null,
            PulseAgentId = targetMode == ConnectionTargetModeNames.PulseAgent ? request.PulseAgentId : null,
            PulseIpMode = pulseIpMode,
            PrivateCandidateSelector = string.IsNullOrWhiteSpace(request.PrivateCandidateSelector)
                ? PulsePrivateCandidateSelectorNames.Selected
                : request.PrivateCandidateSelector.Trim(),
            Port = port,
            Scheme = string.IsNullOrWhiteSpace(request.Scheme) ? "http" : request.Scheme.Trim(),
            PathPrefix = request.PathPrefix,
            TlsValidationMode = string.IsNullOrWhiteSpace(request.TlsValidationMode)
                ? TlsValidationModeNames.System
                : request.TlsValidationMode.Trim(),
            ExpectedTlsHostname = request.ExpectedTlsHostname,
            Status = ConnectionTargetStatusNames.Unresolved,
        };
    }

    private static string SerializeSettings(SshConnectionSettings settings, string authMode)
        => System.Text.Json.JsonSerializer.Serialize(new
        {
            settings.Host,
            settings.Port,
            settings.Username,
            authMode,
            settings.ConfigPath,
            settings.DynamicPath,
        });

    private static string NormalizeTargetMode(string? mode)
        => (mode ?? string.Empty).Trim().ToLowerInvariant() switch
        {
            ConnectionTargetModeNames.StaticIp => ConnectionTargetModeNames.StaticIp,
            ConnectionTargetModeNames.PulseAgent => ConnectionTargetModeNames.PulseAgent,
            ConnectionTargetModeNames.StaticHost => ConnectionTargetModeNames.StaticHost,
            _ => throw new InvalidOperationException("Connection target mode is invalid."),
        };

    private static string NormalizePulseIpMode(string? mode)
        => (mode ?? string.Empty).Trim().ToLowerInvariant() switch
        {
            "" => PulseTargetIpModeNames.Selected,
            PulseTargetIpModeNames.Selected => PulseTargetIpModeNames.Selected,
            PulseTargetIpModeNames.Public => PulseTargetIpModeNames.Public,
            PulseTargetIpModeNames.Private => PulseTargetIpModeNames.PrivateSelected,
            PulseTargetIpModeNames.PrivateSelected => PulseTargetIpModeNames.PrivateSelected,
            PulseTargetIpModeNames.PrivateCandidate => PulseTargetIpModeNames.PrivateCandidate,
            _ => throw new InvalidOperationException("Pulse target IP mode is invalid."),
        };
}
