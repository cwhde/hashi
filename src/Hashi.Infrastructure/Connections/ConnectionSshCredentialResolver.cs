using System.Text.Json;
using Hashi.Core.Auth;
using Hashi.Core.Connections;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;

namespace Hashi.Infrastructure.Connections;

public sealed record ResolvedSshCredentials(
    SshConnectionSettings Settings,
    string AuthMode,
    string? Password,
    string? PrivateKeyPem,
    string? PrivateKeyPassphrase);

public static class ConnectionSshCredentialResolver
{
    public static async Task<ResolvedSshCredentials?> ResolveAsync(
        ConnectionEntity connection,
        SecretRecordService secrets,
        ConnectionTargetResolver? targetResolver = null,
        CancellationToken cancellationToken = default)
    {
        if (connection.SecretId is null)
        {
            return null;
        }

        var payload = await secrets.DecryptForPurposeAsync(connection.SecretId.Value, cancellationToken);
        if (payload is null)
        {
            return null;
        }

        using var doc = JsonDocument.Parse(payload);
        var root = doc.RootElement;
        var settings = targetResolver is null
            ? ParseSettings(connection)
            : await ResolveSettingsAsync(connection, targetResolver, cancellationToken);
        var authMode = root.GetProperty("authMode").GetString() ?? "password";
        return authMode switch
        {
            "private_key" => new ResolvedSshCredentials(
                settings,
                authMode,
                null,
                root.GetProperty("privateKeyPem").GetString(),
                root.TryGetProperty("privateKeyPassphrase", out var pp) ? pp.GetString() : null),
            _ => new ResolvedSshCredentials(
                settings,
                authMode,
                root.GetProperty("password").GetString(),
                null,
                null),
        };
    }

    public static async Task<SshConnectionSettings> ResolveSettingsAsync(
        ConnectionEntity connection,
        ConnectionTargetResolver targetResolver,
        CancellationToken cancellationToken = default)
    {
        var settings = ParseSettings(connection);
        var resolved = await targetResolver.ResolveConnectionAsync(connection, persistSnapshot: true, cancellationToken);
        if (resolved is null)
        {
            return settings;
        }

        if (resolved.Status == ConnectionTargetStatusNames.Failed)
        {
            throw new InvalidOperationException(resolved.Error ?? "Connection target could not be resolved.");
        }

        return settings with
        {
            Host = resolved.ResolvedHost,
            Port = resolved.BaseUri.Port,
        };
    }

    public static byte[] SerializeCredentialPayload(
        string authMode,
        string? password,
        string? privateKeyPem,
        string? privateKeyPassphrase)
        => authMode switch
        {
            "private_key" => JsonSerializer.SerializeToUtf8Bytes(new
            {
                authMode,
                privateKeyPem,
                privateKeyPassphrase,
            }),
            _ => JsonSerializer.SerializeToUtf8Bytes(new
            {
                authMode,
                password,
            }),
        };

    public static SshConnectionSettings ParseSettings(ConnectionEntity connection)
    {
        using var doc = JsonDocument.Parse(connection.SettingsJson);
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
