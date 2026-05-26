using Hashi.Core.Connections;

namespace Hashi.UnitTests.Fakes;

public sealed class FakeSshRemoteExecutor : ISshRemoteExecutor
{
    public int WriteCount { get; private set; }
    public Dictionary<string, byte[]> ReadFiles { get; } = new(StringComparer.Ordinal);

    public Task<SshValidationResult> ValidateAsync(SshConnectionSettings settings, string password, CancellationToken cancellationToken = default)
        => Task.FromResult(new SshValidationResult(true, OsFamily.Debian, "linux", null));

    public Task<SshValidationResult> ValidateWithPrivateKeyAsync(
        SshConnectionSettings settings,
        string privateKeyPem,
        string? passphrase,
        CancellationToken cancellationToken = default)
        => Task.FromResult(new SshValidationResult(true, OsFamily.Debian, "linux", null));

    public Task<RemoteWriteResult> WriteAtomicAsync(
        SshConnectionSettings settings,
        string password,
        string remotePath,
        ReadOnlyMemory<byte> content,
        CancellationToken cancellationToken = default)
    {
        WriteCount++;
        return Task.FromResult(new RemoteWriteResult(true, remotePath, null));
    }

    public Task<RemoteWriteResult> WriteAtomicWithPrivateKeyAsync(
        SshConnectionSettings settings,
        string privateKeyPem,
        string? passphrase,
        string remotePath,
        ReadOnlyMemory<byte> content,
        CancellationToken cancellationToken = default)
    {
        WriteCount++;
        return Task.FromResult(new RemoteWriteResult(true, remotePath, null));
    }

    public Task<RemoteReadResult> ReadFileAsync(
        SshConnectionSettings settings,
        string authMode,
        string? password,
        string? privateKeyPem,
        string? privateKeyPassphrase,
        string remotePath,
        CancellationToken cancellationToken = default)
        => Task.FromResult(
            ReadFiles.TryGetValue(remotePath, out var bytes)
                ? new RemoteReadResult(true, bytes, null)
                : new RemoteReadResult(false, null, "Remote file not found."));

    public Task<RemoteCommandResult> RunCommandAsync(
        SshConnectionSettings settings,
        string authMode,
        string? password,
        string? privateKeyPem,
        string? privateKeyPassphrase,
        string command,
        CancellationToken cancellationToken = default)
        => Task.FromResult(new RemoteCommandResult(true, string.Empty, null));
}
