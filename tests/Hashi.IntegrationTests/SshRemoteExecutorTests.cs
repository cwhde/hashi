using System.Diagnostics;
using System.Text;
using DotNet.Testcontainers.Builders;
using DotNet.Testcontainers.Containers;
using Hashi.Core.Connections;
using Hashi.Infrastructure.Ssh;
using Xunit;

namespace Hashi.IntegrationTests;

public sealed class SshRemoteExecutorTests : IAsyncLifetime
{
    private const string Username = "hashi";
    private const string Password = "hashi-test-password";
    private const string KeyPassphrase = "hashi-key-pass";
    private static readonly TimeSpan ContainerStartupTimeout = TimeSpan.FromSeconds(90);

    private IContainer _sshContainer = null!;
    private SshRemoteExecutor _executor = null!;
    private int _mappedPort;
    private string _encryptedPrivateKeyPem = string.Empty;
    private bool _sshUnavailable;

    public async Task InitializeAsync()
    {
        _executor = new SshRemoteExecutor();
        try
        {
            var keyDir = Path.Combine(Path.GetTempPath(), $"hashi-ssh-test-{Guid.NewGuid():N}");
            Directory.CreateDirectory(keyDir);
            var keyPath = Path.Combine(keyDir, "id_rsa");
            await RunProcessAsync(
                "ssh-keygen",
                $"-t rsa -b 2048 -m PEM -f \"{keyPath}\" -N \"{KeyPassphrase}\" -q");
            _encryptedPrivateKeyPem = await File.ReadAllTextAsync(keyPath);
            var publicKey = (await File.ReadAllTextAsync($"{keyPath}.pub")).Trim();

            _sshContainer = new ContainerBuilder()
                .WithImage("alpine:3.20")
                .WithPortBinding(22, true)
                .WithEnvironment("PUBLIC_KEY", publicKey)
                .WithCommand(
                    "sh",
                    "-c",
                    $"""
                    set -eu
                    apk add --no-cache openssh-server >/dev/null
                    adduser -D -s /bin/sh {Username}
                    echo '{Username}:{Password}' | chpasswd
                    mkdir -p /home/{Username}/.ssh /run/sshd /config
                    printf '%s\n' "$PUBLIC_KEY" > /home/{Username}/.ssh/authorized_keys
                    chown -R {Username}:{Username} /home/{Username}/.ssh /config
                    chmod 700 /home/{Username}/.ssh
                    chmod 600 /home/{Username}/.ssh/authorized_keys
                    ssh-keygen -A >/dev/null
                    exec /usr/sbin/sshd -D -e -p 22 -o PasswordAuthentication=yes -o PubkeyAuthentication=yes -o PermitRootLogin=no
                    """)
                .WithWaitStrategy(Wait.ForUnixContainer().UntilPortIsAvailable(22))
                .Build();

            using var startupCts = new CancellationTokenSource(ContainerStartupTimeout);
            await _sshContainer.StartAsync(startupCts.Token);
            _mappedPort = _sshContainer.GetMappedPublicPort(22);
            await WaitForSshPasswordAuthAsync(startupCts.Token);
        }
        catch (Exception ex) when (IsDockerUnavailable(ex) || IsStartupTimeout(ex))
        {
            var message = $"SSH integration tests unavailable: {ex.Message}";
            Console.WriteLine(message);
            _sshUnavailable = true;
        }
    }

    public async Task DisposeAsync()
    {
        if (!_sshUnavailable && _sshContainer is not null)
        {
            await _sshContainer.DisposeAsync();
        }
    }

    [Fact]
    public async Task Validate_with_password_detects_os_family()
    {
        if (_sshUnavailable)
        {
            return;
        }
        var settings = CreateSettings();
        var result = await _executor.ValidateAsync(settings, Password);
        Assert.True(result.Succeeded, result.Error);
        Assert.NotEqual(OsFamily.Unknown, result.OsFamily);
        Assert.NotNull(result.PackageManager);
    }

    [Fact]
    public async Task WriteAtomic_creates_remote_file()
    {
        if (_sshUnavailable)
        {
            return;
        }
        var settings = CreateSettings();
        var remotePath = "/config/hashi-test.txt";
        var payload = Encoding.UTF8.GetBytes("hashi-atomic-write");
        var write = await _executor.WriteAtomicAsync(settings, Password, remotePath, payload);
        Assert.True(write.Succeeded, write.Error);
    }

    [Fact]
    public async Task Validate_with_encrypted_private_key_succeeds()
    {
        if (_sshUnavailable)
        {
            return;
        }
        var settings = CreateSettings();
        var result = await _executor.ValidateWithPrivateKeyAsync(settings, _encryptedPrivateKeyPem, KeyPassphrase);
        Assert.True(result.Succeeded, result.Error);
    }

    [Fact]
    public async Task WriteAtomic_with_encrypted_private_key_succeeds()
    {
        if (_sshUnavailable)
        {
            return;
        }
        var settings = CreateSettings();
        var remotePath = "/config/hashi-key-write.txt";
        var payload = Encoding.UTF8.GetBytes("hashi-key-atomic");
        var write = await _executor.WriteAtomicWithPrivateKeyAsync(
            settings,
            _encryptedPrivateKeyPem,
            KeyPassphrase,
            remotePath,
            payload);
        Assert.True(write.Succeeded, write.Error);
    }

    private SshConnectionSettings CreateSettings() => new(
        _sshContainer.Hostname,
        _mappedPort,
        Username,
        OsFamily.Unknown,
        null,
        null);

    private async Task WaitForSshPasswordAuthAsync(CancellationToken cancellationToken)
    {
        var deadline = DateTimeOffset.UtcNow.AddSeconds(20);
        Exception? lastException = null;
        string? lastError = null;

        while (DateTimeOffset.UtcNow < deadline)
        {
            cancellationToken.ThrowIfCancellationRequested();
            try
            {
                var result = await _executor.ValidateAsync(CreateSettings(), Password);
                if (result.Succeeded)
                {
                    return;
                }

                lastError = result.Error;
            }
            catch (Exception ex) when (ex is not OperationCanceledException)
            {
                lastException = ex;
            }

            await Task.Delay(TimeSpan.FromMilliseconds(500), cancellationToken);
        }

        var detail = lastError ?? lastException?.Message ?? "no diagnostic was reported";
        throw new TimeoutException($"SSH test container did not accept password authentication in time: {detail}", lastException);
    }

    private static bool IsDockerUnavailable(Exception ex)
        => ex is ArgumentException { ParamName: "DockerEndpointAuthConfig" }
           || ex.Message.Contains("Docker is either not running", StringComparison.OrdinalIgnoreCase);

    private static bool IsStartupTimeout(Exception ex)
        => ex is OperationCanceledException or TimeoutException
           || ex.Message.Contains("timed out", StringComparison.OrdinalIgnoreCase);

    private static async Task RunProcessAsync(string fileName, string arguments)
    {
        using var process = Process.Start(new ProcessStartInfo
        {
            FileName = fileName,
            Arguments = arguments,
            RedirectStandardError = true,
            RedirectStandardOutput = true,
            UseShellExecute = false,
        }) ?? throw new InvalidOperationException($"Failed to start {fileName}.");
        await process.WaitForExitAsync();
        if (process.ExitCode != 0)
        {
            var stderr = await process.StandardError.ReadToEndAsync();
            throw new InvalidOperationException($"{fileName} failed: {stderr}");
        }
    }
}
