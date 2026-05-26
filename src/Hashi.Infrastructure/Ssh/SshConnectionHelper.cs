using System.Text;
using Hashi.Core.Connections;
using Renci.SshNet;

namespace Hashi.Infrastructure.Ssh;

internal static class SshConnectionHelper
{
    public static SshClient CreatePasswordClient(SshConnectionSettings settings, string password)
    {
        var auth = new PasswordAuthenticationMethod(settings.Username, password);
        var connectionInfo = new ConnectionInfo(settings.Host, settings.Port, settings.Username, auth)
        {
            Timeout = TimeSpan.FromSeconds(15),
        };
        return new SshClient(connectionInfo);
    }

    public static SshClient CreatePrivateKeyClient(
        SshConnectionSettings settings,
        string privateKeyPem,
        string? passphrase)
    {
        using var keyStream = new MemoryStream(Encoding.UTF8.GetBytes(privateKeyPem));
        var keyFile = string.IsNullOrEmpty(passphrase)
            ? new PrivateKeyFile(keyStream)
            : new PrivateKeyFile(keyStream, passphrase);
        var auth = new PrivateKeyAuthenticationMethod(settings.Username, keyFile);
        var connectionInfo = new ConnectionInfo(settings.Host, settings.Port, settings.Username, auth)
        {
            Timeout = TimeSpan.FromSeconds(15),
        };
        return new SshClient(connectionInfo);
    }

    public static string RunCommand(SshClient client, string command)
    {
        using var cmd = client.CreateCommand(command);
        cmd.CommandTimeout = TimeSpan.FromSeconds(30);
        var result = cmd.Execute();
        if (cmd.ExitStatus is > 0)
        {
            throw new InvalidOperationException(
                $"Remote command failed ({cmd.ExitStatus}): {cmd.Error.Trim()}");
        }

        return result;
    }
}
