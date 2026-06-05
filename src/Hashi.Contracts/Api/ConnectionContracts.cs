namespace Hashi.Contracts.Api;

public static class ConnectionTypeContractNames
{
    public const string DnsProvider = "dns_provider";
    public const string TraefikHost = "traefik_host";
    public const string FirewallHost = "firewall_host";

    public static readonly string[] SshConnectionTypes =
    [
        TraefikHost,
        FirewallHost,
    ];

    public static bool IsSshConnectionType(string? value)
        => value is not null && SshConnectionTypes.Contains(value, StringComparer.Ordinal);
}

public sealed record CreateSshConnectionRequest(
    string Name,
    string ConnectionType,
    string Host,
    int Port,
    string Username,
    string AuthMode,
    string? Password,
    string? PrivateKeyPem,
    string? PrivateKeyPassphrase,
    ConnectionTargetRequest? Target = null);

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
