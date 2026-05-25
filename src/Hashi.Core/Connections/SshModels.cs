namespace Hashi.Core.Connections;

public enum ConnectionHealthState
{
    Unknown,
    Validating,
    Healthy,
    Degraded,
    Failed,
}

public enum OsFamily
{
    Unknown,
    Debian,
    Ubuntu,
    Alpine,
}

public sealed record SshConnectionSettings(
    string Host,
    int Port,
    string Username,
    OsFamily DetectedOs,
    string? ConfigPath,
    string? DynamicPath);

public sealed record SshValidationResult(
    bool Succeeded,
    OsFamily OsFamily,
    string? PackageManager,
    string? Error);

public sealed record RemoteWriteResult(bool Succeeded, string RemotePath, string? Error);

public interface ISshRemoteExecutor
{
    Task<SshValidationResult> ValidateAsync(SshConnectionSettings settings, string password, CancellationToken cancellationToken = default);

    Task<SshValidationResult> ValidateWithPrivateKeyAsync(
        SshConnectionSettings settings,
        string privateKeyPem,
        string? passphrase,
        CancellationToken cancellationToken = default);

    Task<RemoteWriteResult> WriteAtomicAsync(
        SshConnectionSettings settings,
        string password,
        string remotePath,
        ReadOnlyMemory<byte> content,
        CancellationToken cancellationToken = default);
}
