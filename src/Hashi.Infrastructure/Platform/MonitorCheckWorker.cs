using System.Diagnostics;
using System.Net;
using System.Net.NetworkInformation;
using System.Net.Security;
using System.Net.Sockets;
using System.Security.Authentication;
using System.Security.Cryptography.X509Certificates;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Platform;

public sealed class MonitorCheckWorker(
    IServiceScopeFactory scopeFactory,
    IHttpClientFactory httpClientFactory,
    ILogger<MonitorCheckWorker> logger,
    IMonitorNetworkProbe? networkProbe = null) : BackgroundService
{
    private static readonly TimeSpan TlsDegradedThreshold = TimeSpan.FromDays(14);
    private static readonly TimeSpan PulseDegradedThreshold = TimeSpan.FromMinutes(2);
    private static readonly TimeSpan PulseDownThreshold = TimeSpan.FromMinutes(5);
    private readonly IMonitorNetworkProbe _networkProbe = networkProbe ?? new DefaultMonitorNetworkProbe();

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        while (!stoppingToken.IsCancellationRequested)
        {
            var delaySeconds = 30;
            try
            {
                using var scope = scopeFactory.CreateScope();
                var settings = scope.ServiceProvider.GetRequiredService<AppSettingsService>();
                var appSettings = await settings.GetOrCreateAsync(stoppingToken);
                delaySeconds = Math.Clamp(appSettings.MonitorCheckIntervalSeconds, 15, 300);

                var jobs = scope.ServiceProvider.GetRequiredService<BackgroundJobService>();
                await jobs.BeginRunAsync(BackgroundJobKeys.MonitorCheck, stoppingToken);
                await RunChecksAsync(appSettings.MonitorCheckTimeoutSeconds, appSettings.MonitorDegradedLatencyMs, stoppingToken);
                await jobs.CompleteRunAsync(
                    BackgroundJobKeys.MonitorCheck,
                    true,
                    "Monitor checks completed.",
                    null,
                    delaySeconds,
                    stoppingToken);
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "Monitor check worker failed.");
                try
                {
                    using var scope = scopeFactory.CreateScope();
                    var jobs = scope.ServiceProvider.GetRequiredService<BackgroundJobService>();
                    await jobs.CompleteRunAsync(BackgroundJobKeys.MonitorCheck, false, null, ex.Message, delaySeconds, stoppingToken);
                }
                catch
                {
                }
            }

            await Task.Delay(TimeSpan.FromSeconds(delaySeconds), stoppingToken);
        }
    }

    private async Task RunChecksAsync(int timeoutSeconds, int degradedLatencyMs, CancellationToken cancellationToken)
    {
        using var scope = scopeFactory.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var monitoring = scope.ServiceProvider.GetRequiredService<MonitoringService>();
        var appSettings = scope.ServiceProvider.GetRequiredService<AppSettingsService>();
        await monitoring.SyncEndpointsFromResourcesAsync(cancellationToken);
        var endpoints = await db.MonitorEndpoints.Where(x => x.Enabled).ToListAsync(cancellationToken);
        var client = httpClientFactory.CreateClient("monitor-checks");
        client.Timeout = TimeSpan.FromSeconds(Math.Clamp(timeoutSeconds, 5, 120));
        var retentionDays = Math.Clamp((await appSettings.GetOrCreateAsync(cancellationToken)).MonitorSampleRetentionDays, 7, 365);
        var pulseAgentByResourceId = await db.Resources.AsNoTracking()
            .Where(x => x.PulseAgentId != null)
            .ToDictionaryAsync(x => x.Id, x => x.PulseAgentId!.Value, cancellationToken);
        var pulseLastSeenById = await db.PulseAgents.AsNoTracking()
            .ToDictionaryAsync(x => x.Id, x => x.LastSeenAtUtc, cancellationToken);
        await MonitorSamplePartitionService.EnsureWeeklyPartitionsAsync(db, cancellationToken);

        foreach (var endpoint in endpoints)
        {
            var previousStatus = endpoint.Status;
            var sw = Stopwatch.StartNew();
            var status = await CheckEndpointStatusAsync(
                endpoint,
                client,
                timeoutSeconds,
                pulseAgentByResourceId,
                pulseLastSeenById,
                cancellationToken);

            sw.Stop();
            var latency = (int)sw.ElapsedMilliseconds;
            if (status == "up"
                && IsHttpCheckType(endpoint.CheckType)
                && latency >= degradedLatencyMs)
            {
                status = "degraded";
            }

            endpoint.Status = status;
            endpoint.LastCheckedAtUtc = DateTimeOffset.UtcNow;
            endpoint.LastLatencyMs = latency;

            var sample = new MonitorSampleEntity
            {
                MonitorEndpointId = endpoint.Id,
                PartitionDate = DateOnly.FromDateTime(DateTime.UtcNow),
                CheckedAtUtc = DateTimeOffset.UtcNow,
                Status = status,
                LatencyMs = latency,
            };
            db.MonitorSamples.Add(sample);

            await monitoring.RecordTransitionAsync(endpoint, previousStatus, status, latency, cancellationToken);

            var routing = scope.ServiceProvider.GetRequiredService<NotificationRoutingService>();
            await routing.RouteMonitorTransitionAsync(endpoint, previousStatus, status, cancellationToken);
        }

        await db.SaveChangesAsync(cancellationToken);
        await MonitorRollupService.RollupRecentAsync(db, cancellationToken);
        await PruneOldSamplesAsync(db, retentionDays, cancellationToken);
    }

    internal async Task<string> CheckEndpointStatusAsync(
        MonitorEndpointEntity endpoint,
        HttpClient client,
        int timeoutSeconds,
        IReadOnlyDictionary<Guid, Guid> pulseAgentByResourceId,
        IReadOnlyDictionary<Guid, DateTimeOffset?> pulseLastSeenById,
        CancellationToken cancellationToken)
    {
        var checkType = NormalizeCheckType(endpoint.CheckType);
        var timeout = TimeSpan.FromSeconds(Math.Clamp(timeoutSeconds, 1, 120));
        try
        {
            switch (checkType)
            {
                case "http":
                case "https":
                case "h2c":
                    {
                        using var response = await client.GetAsync(endpoint.Url, cancellationToken);
                        return response.IsSuccessStatusCode ? "up" : "down";
                    }
                case "tcp":
                    {
                        if (!TryResolveHostPort(endpoint.Url, 80, out var host, out var port))
                        {
                            return "down";
                        }

                        return await _networkProbe.CheckTcpAsync(host, port, timeout, cancellationToken) ? "up" : "down";
                    }
                case "udp":
                    {
                        if (!TryResolveHostPort(endpoint.Url, 53, out var host, out var port))
                        {
                            return "down";
                        }

                        // UDP is connectionless; successful datagram send is a minimal liveness check.
                        return await _networkProbe.ProbeUdpAsync(host, port, timeout, cancellationToken) ? "up" : "down";
                    }
                case "dns":
                    {
                        var host = ResolveHost(endpoint.Url);
                        if (string.IsNullOrWhiteSpace(host))
                        {
                            return "down";
                        }

                        return await _networkProbe.ResolveDnsAsync(host, timeout, cancellationToken) ? "up" : "down";
                    }
                case "icmp":
                    {
                        var host = ResolveHost(endpoint.Url);
                        if (string.IsNullOrWhiteSpace(host))
                        {
                            return "down";
                        }

                        return await _networkProbe.PingAsync(host, timeout, cancellationToken) ? "up" : "down";
                    }
                case "tls":
                    {
                        if (!TryResolveHostPort(endpoint.Url, 443, out var host, out var port))
                        {
                            return "down";
                        }

                        var tlsResult = await _networkProbe.CheckTlsAsync(host, port, timeout, cancellationToken);
                        if (!tlsResult.HandshakeSucceeded || tlsResult.NotAfterUtc is null)
                        {
                            return "down";
                        }

                        var remaining = tlsResult.NotAfterUtc.Value - DateTimeOffset.UtcNow;
                        if (remaining <= TimeSpan.Zero)
                        {
                            return "down";
                        }

                        return remaining <= TlsDegradedThreshold ? "degraded" : "up";
                    }
                case "pulse":
                case "push":
                    {
                        if (endpoint.ResourceId is not Guid resourceId
                            || !pulseAgentByResourceId.TryGetValue(resourceId, out var pulseAgentId)
                            || !pulseLastSeenById.TryGetValue(pulseAgentId, out var lastSeenAtUtc)
                            || lastSeenAtUtc is null)
                        {
                            return "down";
                        }

                        var age = DateTimeOffset.UtcNow - lastSeenAtUtc.Value;
                        if (age > PulseDownThreshold)
                        {
                            return "down";
                        }

                        return age > PulseDegradedThreshold ? "degraded" : "up";
                    }
                default:
                    return "down";
            }
        }
        catch
        {
            return "down";
        }
    }

    internal static bool IsHttpCheckType(string? checkType)
    {
        var normalized = NormalizeCheckType(checkType);
        return normalized is "http" or "https" or "h2c";
    }

    private static string NormalizeCheckType(string? checkType)
        => (checkType ?? string.Empty).Trim().ToLowerInvariant();

    private static bool TryResolveHostPort(string input, int defaultPort, out string host, out int port)
    {
        host = string.Empty;
        port = defaultPort;

        if (Uri.TryCreate(input, UriKind.Absolute, out var uri))
        {
            host = uri.Host;
            port = uri.IsDefaultPort ? defaultPort : uri.Port;
            return !string.IsNullOrWhiteSpace(host) && port > 0;
        }

        var trimmed = input.Trim();
        if (string.IsNullOrWhiteSpace(trimmed))
        {
            return false;
        }

        if (trimmed.Contains(':', StringComparison.Ordinal))
        {
            var index = trimmed.LastIndexOf(':');
            var hostPart = trimmed[..index];
            var portPart = trimmed[(index + 1)..];
            if (!string.IsNullOrWhiteSpace(hostPart) && int.TryParse(portPart, out var parsedPort) && parsedPort > 0)
            {
                host = hostPart;
                port = parsedPort;
                return true;
            }
        }

        host = trimmed;
        return true;
    }

    private static string ResolveHost(string input)
    {
        if (Uri.TryCreate(input, UriKind.Absolute, out var uri))
        {
            return uri.Host;
        }

        if (input.Contains("://", StringComparison.Ordinal))
        {
            return string.Empty;
        }

        var trimmed = input.Trim();
        if (trimmed.Contains('/', StringComparison.Ordinal))
        {
            trimmed = trimmed.Split('/', 2, StringSplitOptions.TrimEntries)[0];
        }

        if (trimmed.Contains(':', StringComparison.Ordinal))
        {
            trimmed = trimmed.Split(':', 2, StringSplitOptions.TrimEntries)[0];
        }

        return trimmed;
    }

    private static Task PruneOldSamplesAsync(HashiDbContext db, int retentionDays, CancellationToken cancellationToken)
        => MonitorSamplePartitionService.DropExpiredPartitionsAsync(db, retentionDays, cancellationToken);
}

public interface IMonitorNetworkProbe
{
    Task<bool> CheckTcpAsync(string host, int port, TimeSpan timeout, CancellationToken cancellationToken);
    Task<bool> ProbeUdpAsync(string host, int port, TimeSpan timeout, CancellationToken cancellationToken);
    Task<bool> ResolveDnsAsync(string host, TimeSpan timeout, CancellationToken cancellationToken);
    Task<TlsProbeResult> CheckTlsAsync(string host, int port, TimeSpan timeout, CancellationToken cancellationToken);
    Task<bool> PingAsync(string host, TimeSpan timeout, CancellationToken cancellationToken);
}

public readonly record struct TlsProbeResult(bool HandshakeSucceeded, DateTimeOffset? NotAfterUtc);

internal sealed class DefaultMonitorNetworkProbe : IMonitorNetworkProbe
{
    public async Task<bool> CheckTcpAsync(string host, int port, TimeSpan timeout, CancellationToken cancellationToken)
    {
        using var linkedCts = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        linkedCts.CancelAfter(timeout);
        using var client = new TcpClient();
        await client.ConnectAsync(host, port, linkedCts.Token);
        return client.Connected;
    }

    public async Task<bool> ProbeUdpAsync(string host, int port, TimeSpan timeout, CancellationToken cancellationToken)
    {
        using var linkedCts = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        linkedCts.CancelAfter(timeout);
        using var client = new UdpClient();
        client.Connect(host, port);
        linkedCts.Token.ThrowIfCancellationRequested();
        await client.SendAsync([], 0);
        linkedCts.Token.ThrowIfCancellationRequested();
        return true;
    }

    public async Task<bool> ResolveDnsAsync(string host, TimeSpan timeout, CancellationToken cancellationToken)
    {
        using var linkedCts = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        linkedCts.CancelAfter(timeout);
        var addresses = await System.Net.Dns.GetHostAddressesAsync(host, linkedCts.Token);
        return addresses.Length > 0;
    }

    public async Task<TlsProbeResult> CheckTlsAsync(string host, int port, TimeSpan timeout, CancellationToken cancellationToken)
    {
        using var linkedCts = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        linkedCts.CancelAfter(timeout);
        using var client = new TcpClient();
        await client.ConnectAsync(host, port, linkedCts.Token);
        if (!client.Connected)
        {
            return new TlsProbeResult(false, null);
        }

        await using var stream = client.GetStream();
        using var ssl = new SslStream(stream, false, static (_, _, _, _) => true);
        await ssl.AuthenticateAsClientAsync(host, null, SslProtocols.None, false);
        if (ssl.RemoteCertificate is null)
        {
            return new TlsProbeResult(false, null);
        }

        var cert = new X509Certificate2(ssl.RemoteCertificate);
        return new TlsProbeResult(true, new DateTimeOffset(cert.NotAfter.ToUniversalTime(), TimeSpan.Zero));
    }

    public async Task<bool> PingAsync(string host, TimeSpan timeout, CancellationToken cancellationToken)
    {
        using var ping = new Ping();
        var timeoutMs = (int)Math.Clamp(timeout.TotalMilliseconds, 1, int.MaxValue);
        var reply = await ping.SendPingAsync(host, timeoutMs);
        cancellationToken.ThrowIfCancellationRequested();
        return reply.Status == IPStatus.Success;
    }
}
