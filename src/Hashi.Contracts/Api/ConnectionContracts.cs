namespace Hashi.Contracts.Api;

public sealed record CreateSshConnectionRequest(
    string Name,
    string ConnectionType,
    string Host,
    int Port,
    string Username,
    string AuthMode,
    string? Password,
    string? PrivateKeyPem,
    string? PrivateKeyPassphrase);

public sealed record SshValidationResponse(
    bool Succeeded,
    string OsFamily,
    string? PackageManager,
    string? Error);

public sealed record RemoteWriteRequest(
    string RemotePath,
    string ContentBase64,
    string Host,
    int Port,
    string Username,
    string AuthMode,
    string? Password,
    string? PrivateKeyPem,
    string? PrivateKeyPassphrase);

public sealed record RemoteWriteResponse(bool Succeeded, string RemotePath, string? Error);
