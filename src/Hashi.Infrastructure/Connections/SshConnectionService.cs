using Hashi.Core.Connections;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Connections;

public sealed class SshConnectionService(
    HashiDbContext db,
    ISshRemoteExecutor ssh,
    AuditService audit)
{
    public async Task<ConnectionEntity> CreateAsync(
        string name,
        string connectionType,
        SshConnectionSettings settings,
        string authMode,
        string? password,
        string? privateKeyPem,
        string? privateKeyPassphrase,
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
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("connections", "ssh_connection_created", subjectType: "connection", subjectId: connection.Id.ToString(), cancellationToken: cancellationToken);
        return connection;
    }

    public async Task<SshValidationResult> ValidateAsync(Guid connectionId, CancellationToken cancellationToken = default)
    {
        var connection = await db.Connections.SingleAsync(x => x.Id == connectionId, cancellationToken);
        var settings = ParseSettings(connection);
        connection.HealthState = ConnectionHealthStateNames.Validating;
        await db.SaveChangesAsync(cancellationToken);

        // Validation uses request-time credentials stored outside DB for now.
        throw new InvalidOperationException("Use validate endpoint with credentials payload.");
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
    {
        using var doc = System.Text.Json.JsonDocument.Parse(connection.SettingsJson);
        var root = doc.RootElement;
        return new SshConnectionSettings(
            root.GetProperty("Host").GetString() ?? string.Empty,
            root.TryGetProperty("Port", out var port) ? port.GetInt32() : 22,
            root.GetProperty("Username").GetString() ?? string.Empty,
            OsFamily.Unknown,
            root.TryGetProperty("ConfigPath", out var configPath) ? configPath.GetString() : null,
            root.TryGetProperty("DynamicPath", out var dynamicPath) ? dynamicPath.GetString() : null);
    }
}
