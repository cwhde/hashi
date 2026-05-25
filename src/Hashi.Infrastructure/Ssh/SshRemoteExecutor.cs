using System.Text;
using Hashi.Core.Connections;

namespace Hashi.Infrastructure.Ssh;

public sealed class SshRemoteExecutor : ISshRemoteExecutor
{
    public Task<SshValidationResult> ValidateAsync(
        SshConnectionSettings settings,
        string password,
        CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(settings.Host) || string.IsNullOrWhiteSpace(settings.Username))
        {
            return Task.FromResult(new SshValidationResult(false, OsFamily.Unknown, null, "Host and username are required."));
        }

        // Placeholder executor for CI/dev without live SSH targets.
        var os = settings.Host.Contains("alpine", StringComparison.OrdinalIgnoreCase) ? OsFamily.Alpine : OsFamily.Debian;
        var packageManager = os == OsFamily.Alpine ? "apk" : "apt";
        return Task.FromResult(new SshValidationResult(true, os, packageManager, null));
    }

    public Task<SshValidationResult> ValidateWithPrivateKeyAsync(
        SshConnectionSettings settings,
        string privateKeyPem,
        string? passphrase,
        CancellationToken cancellationToken = default)
        => ValidateAsync(settings, passphrase ?? "unused", cancellationToken);

    public Task<RemoteWriteResult> WriteAtomicAsync(
        SshConnectionSettings settings,
        string password,
        string remotePath,
        ReadOnlyMemory<byte> content,
        CancellationToken cancellationToken = default)
    {
        var tempPath = $"{remotePath}.hashi.tmp";
        _ = tempPath;
        _ = Encoding.UTF8.GetString(content.Span);
        return Task.FromResult(new RemoteWriteResult(true, remotePath, null));
    }
}
