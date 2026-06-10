using System.Net;
using System.Net.Sockets;
using System.Text;
using Xunit;

namespace Hashi.IntegrationTests;

public sealed class SmtpFakeServerTests : IAsyncLifetime
{
    private TcpListener? _listener;
    private int _port;
    private CancellationTokenSource? _cts;

    public async Task InitializeAsync()
    {
        _listener = new TcpListener(IPAddress.Loopback, 0);
        _listener.Start();
        _port = ((IPEndPoint)_listener.LocalEndpoint).Port;
        _cts = new CancellationTokenSource();

        _ = AcceptConnectionsAsync(_cts.Token);
    }

    public async Task DisposeAsync()
    {
        if (_cts is not null)
        {
            await _cts.CancelAsync();
        }

        _listener?.Stop();
        if (_cts is not null)
        {
            await _cts.DisposeAsync();
        }
    }

    [Fact]
    public async Task Fake_smtp_server_accepts_connection()
    {
        using var client = new TcpClient();
        await client.ConnectAsync(IPAddress.Loopback, _port);
        Assert.True(client.Connected);
    }

    [Fact]
    public async Task Fake_smtp_server_resolves_to_loopback()
    {
        var addresses = await Dns.GetHostAddressesAsync("127.0.0.1");
        Assert.Contains(addresses, a => a.Equals(IPAddress.Loopback));
    }

    [Fact]
    public void Fake_smtp_port_is_available()
    {
        Assert.True(_port > 0);
    }

    private async Task AcceptConnectionsAsync(CancellationToken cancellationToken)
    {
        while (!cancellationToken.IsCancellationRequested)
        {
            try
            {
                var client = await _listener!.AcceptTcpClientAsync(cancellationToken);
                _ = HandleClientAsync(client, cancellationToken);
            }
            catch (OperationCanceledException)
            {
                break;
            }
            catch (ObjectDisposedException)
            {
                break;
            }
        }
    }

    private static async Task HandleClientAsync(TcpClient client, CancellationToken cancellationToken)
    {
        await using var _ = client;
        await using var stream = client.GetStream();
        var reader = new StreamReader(stream, Encoding.ASCII);
        var writer = new StreamWriter(stream, Encoding.ASCII) { AutoFlush = true };

        await writer.WriteLineAsync("220 localhost SMTP ready", cancellationToken);

        while (!cancellationToken.IsCancellationRequested && client.Connected)
        {
            var line = await reader.ReadLineAsync(cancellationToken);
            if (line is null)
            {
                break;
            }

            if (line.StartsWith("QUIT", StringComparison.OrdinalIgnoreCase))
            {
                await writer.WriteLineAsync("221 Bye", cancellationToken);
                break;
            }

            if (line.StartsWith("EHLO", StringComparison.OrdinalIgnoreCase)
                || line.StartsWith("HELO", StringComparison.OrdinalIgnoreCase))
            {
                await writer.WriteLineAsync("250-localhost", cancellationToken);
                await writer.WriteLineAsync("250 SIZE", cancellationToken);
            }
            else if (line.StartsWith("MAIL FROM", StringComparison.OrdinalIgnoreCase))
            {
                await writer.WriteLineAsync("250 OK", cancellationToken);
            }
            else if (line.StartsWith("RCPT TO", StringComparison.OrdinalIgnoreCase))
            {
                await writer.WriteLineAsync("250 OK", cancellationToken);
            }
            else if (line.StartsWith("DATA", StringComparison.OrdinalIgnoreCase))
            {
                await writer.WriteLineAsync("354 Start mail input", cancellationToken);
                var dataLine = await reader.ReadLineAsync(cancellationToken);
                while (dataLine is not null && dataLine != ".")
                {
                    dataLine = await reader.ReadLineAsync(cancellationToken);
                }

                await writer.WriteLineAsync("250 OK", cancellationToken);
            }
            else
            {
                await writer.WriteLineAsync("500 Command not recognized", cancellationToken);
            }
        }
    }
}
