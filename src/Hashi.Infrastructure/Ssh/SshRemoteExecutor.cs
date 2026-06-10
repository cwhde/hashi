using System.Security.Cryptography;
using Hashi.Core.Connections;
using Renci.SshNet;

namespace Hashi.Infrastructure.Ssh;

public sealed class SshRemoteExecutor : ISshRemoteExecutor
{
    public Task<SshValidationResult> ValidateAsync(
        SshConnectionSettings settings,
        string password,
        CancellationToken cancellationToken = default)
        => Task.Run(() => ValidateCore(settings, () => SshConnectionHelper.CreatePasswordClient(settings, password)), cancellationToken);

    public Task<SshValidationResult> ValidateWithPrivateKeyAsync(
        SshConnectionSettings settings,
        string privateKeyPem,
        string? passphrase,
        CancellationToken cancellationToken = default)
        => Task.Run(
            () => ValidateCore(settings, () => SshConnectionHelper.CreatePrivateKeyClient(settings, privateKeyPem, passphrase)),
            cancellationToken);

    public Task<RemoteWriteResult> WriteAtomicAsync(
        SshConnectionSettings settings,
        string password,
        string remotePath,
        ReadOnlyMemory<byte> content,
        CancellationToken cancellationToken = default)
        => Task.Run(
            () => WriteAtomicCore(settings, () => SshConnectionHelper.CreatePasswordClient(settings, password), remotePath, content),
            cancellationToken);

    public Task<RemoteWriteResult> WriteAtomicWithPrivateKeyAsync(
        SshConnectionSettings settings,
        string privateKeyPem,
        string? passphrase,
        string remotePath,
        ReadOnlyMemory<byte> content,
        CancellationToken cancellationToken = default)
        => Task.Run(
            () => WriteAtomicCore(
                settings,
                () => SshConnectionHelper.CreatePrivateKeyClient(settings, privateKeyPem, passphrase),
                remotePath,
                content),
            cancellationToken);

    public Task<RemoteReadResult> ReadFileAsync(
        SshConnectionSettings settings,
        string authMode,
        string? password,
        string? privateKeyPem,
        string? privateKeyPassphrase,
        string remotePath,
        CancellationToken cancellationToken = default)
        => Task.Run(
            () => ReadFileCore(settings, CreateClientFactory(authMode, settings, password, privateKeyPem, privateKeyPassphrase), remotePath),
            cancellationToken);

    public Task<RemoteCommandResult> RunCommandAsync(
        SshConnectionSettings settings,
        string authMode,
        string? password,
        string? privateKeyPem,
        string? privateKeyPassphrase,
        string command,
        CancellationToken cancellationToken = default)
        => Task.Run(
            () => RunCommandCore(settings, CreateClientFactory(authMode, settings, password, privateKeyPem, privateKeyPassphrase), command),
            cancellationToken);

    private static Func<SshClient> CreateClientFactory(
        string authMode,
        SshConnectionSettings settings,
        string? password,
        string? privateKeyPem,
        string? privateKeyPassphrase)
        => authMode switch
        {
            "password" when !string.IsNullOrWhiteSpace(password) =>
                () => SshConnectionHelper.CreatePasswordClient(settings, password),
            "private_key" when !string.IsNullOrWhiteSpace(privateKeyPem) =>
                () => SshConnectionHelper.CreatePrivateKeyClient(settings, privateKeyPem, privateKeyPassphrase),
            _ => throw new InvalidOperationException("Unsupported auth mode."),
        };

    private static SshValidationResult ValidateCore(SshConnectionSettings settings, Func<SshClient> createClient)
    {
        if (string.IsNullOrWhiteSpace(settings.Host) || string.IsNullOrWhiteSpace(settings.Username))
        {
            return new SshValidationResult(false, OsFamily.Unknown, null, "Host and username are required.");
        }

        try
        {
            using var client = createClient();
            client.Connect();
            if (!client.IsConnected)
            {
                return new SshValidationResult(false, OsFamily.Unknown, null, "SSH connection failed.");
            }

            var osRelease = SshConnectionHelper.RunCommand(client, "cat /etc/os-release 2>/dev/null || true");
            var (osFamily, packageManager) = OsReleaseParser.Parse(osRelease);
            return new SshValidationResult(true, osFamily, packageManager, null);
        }
        catch (Exception ex)
        {
            return new SshValidationResult(false, OsFamily.Unknown, null, ex.Message);
        }
    }

    private static RemoteWriteResult WriteAtomicCore(
        SshConnectionSettings settings,
        Func<SshClient> createClient,
        string remotePath,
        ReadOnlyMemory<byte> content,
        Func<ReadOnlyMemory<byte>, (bool IsValid, string? Error)>? validateContent = null)
    {
        if (string.IsNullOrWhiteSpace(remotePath))
        {
            return new RemoteWriteResult(false, remotePath, "Remote path is required.");
        }

        var tempPath = $"{remotePath}.hashi.tmp";
        try
        {
            using var client = createClient();
            client.Connect();
            using var sftp = new SftpClient(client.ConnectionInfo);
            sftp.Connect();

            if (sftp.Exists(remotePath))
            {
                using var existingStream = sftp.OpenRead(remotePath);
                using var existingMs = new MemoryStream();
                existingStream.CopyTo(existingMs);
                var existingHash = Convert.ToHexString(SHA256.HashData(existingMs.ToArray()));
                var newHash = Convert.ToHexString(SHA256.HashData(content.Span));
                if (string.Equals(existingHash, newHash, StringComparison.OrdinalIgnoreCase))
                {
                    return new RemoteWriteResult(true, remotePath, null);
                }
            }

            using (var stream = sftp.OpenWrite(tempPath))
            {
                stream.Write(content.Span);
            }

            if (sftp.Exists(tempPath))
            {
                using var tempStream = sftp.OpenRead(tempPath);
                using var tempMs = new MemoryStream();
                tempStream.CopyTo(tempMs);
                var tempBytes = tempMs.ToArray();
                var writtenHash = Convert.ToHexString(SHA256.HashData(tempBytes));
                var expectedHash = Convert.ToHexString(SHA256.HashData(content.Span));
                if (!string.Equals(writtenHash, expectedHash, StringComparison.OrdinalIgnoreCase))
                {
                    SshConnectionHelper.RunCommand(client, $"rm -f {ShellQuote(tempPath)}");
                    return new RemoteWriteResult(false, remotePath, $"Content hash mismatch after write: expected {expectedHash}, got {writtenHash}.");
                }

                if (validateContent is not null)
                {
                    var (isValid, validationError) = validateContent(tempBytes);
                    if (!isValid)
                    {
                        SshConnectionHelper.RunCommand(client, $"rm -f {ShellQuote(tempPath)}");
                        return new RemoteWriteResult(false, remotePath, validationError ?? "Content validation failed before atomic move.");
                    }
                }
            }

            SshConnectionHelper.RunCommand(
                client,
                $"mv -f {ShellQuote(tempPath)} {ShellQuote(remotePath)}");
            return new RemoteWriteResult(true, remotePath, null);
        }
        catch (Exception ex)
        {
            return new RemoteWriteResult(false, remotePath, ex.Message);
        }
    }

    private static RemoteReadResult ReadFileCore(
        SshConnectionSettings settings,
        Func<SshClient> createClient,
        string remotePath)
    {
        try
        {
            using var client = createClient();
            client.Connect();
            using var sftp = new SftpClient(client.ConnectionInfo);
            sftp.Connect();
            if (!sftp.Exists(remotePath))
            {
                return new RemoteReadResult(false, null, "Remote file not found.");
            }

            using var stream = sftp.OpenRead(remotePath);
            using var memory = new MemoryStream();
            stream.CopyTo(memory);
            return new RemoteReadResult(true, memory.ToArray(), null);
        }
        catch (Exception ex)
        {
            return new RemoteReadResult(false, null, ex.Message);
        }
    }

    private static RemoteCommandResult RunCommandCore(
        SshConnectionSettings settings,
        Func<SshClient> createClient,
        string command)
    {
        try
        {
            using var client = createClient();
            client.Connect();
            var output = SshConnectionHelper.RunCommand(client, command);
            return new RemoteCommandResult(true, output, null);
        }
        catch (Exception ex)
        {
            return new RemoteCommandResult(false, string.Empty, ex.Message);
        }
    }

    private static string ShellQuote(string value) => $"'{value.Replace("'", "'\\''", StringComparison.Ordinal)}'";
}
