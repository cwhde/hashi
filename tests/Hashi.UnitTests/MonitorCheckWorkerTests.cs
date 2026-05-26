using System.Net;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;
using Xunit;

namespace Hashi.UnitTests;

public sealed class MonitorCheckWorkerTests
{
    [Fact]
    public async Task MonitorCheckWorker_tcp_connect_success_marks_up()
    {
        var worker = CreateWorker(new FakeMonitorNetworkProbe
        {
            TcpResult = true,
        });

        var status = await worker.CheckEndpointStatusAsync(
            new MonitorEndpointEntity
            {
                CheckType = "tcp",
                Url = "tcp://service.internal:5432",
            },
            CreateHttpClient(),
            timeoutSeconds: 10,
            pulseAgentByResourceId: new Dictionary<Guid, Guid>(),
            pulseLastSeenById: new Dictionary<Guid, DateTimeOffset?>(),
            cancellationToken: CancellationToken.None);

        Assert.Equal("up", status);
    }

    [Fact]
    public async Task MonitorCheckWorker_dns_resolve_failure_marks_down()
    {
        var worker = CreateWorker(new FakeMonitorNetworkProbe
        {
            DnsResult = false,
        });

        var status = await worker.CheckEndpointStatusAsync(
            new MonitorEndpointEntity
            {
                CheckType = "dns",
                Url = "dns://invalid.example.invalid",
            },
            CreateHttpClient(),
            timeoutSeconds: 10,
            pulseAgentByResourceId: new Dictionary<Guid, Guid>(),
            pulseLastSeenById: new Dictionary<Guid, DateTimeOffset?>(),
            cancellationToken: CancellationToken.None);

        Assert.Equal("down", status);
    }

    [Fact]
    public async Task MonitorCheckWorker_tls_expiry_within_threshold_marks_degraded()
    {
        var worker = CreateWorker(new FakeMonitorNetworkProbe
        {
            TlsResult = new TlsProbeResult(true, DateTimeOffset.UtcNow.AddDays(3)),
        });

        var status = await worker.CheckEndpointStatusAsync(
            new MonitorEndpointEntity
            {
                CheckType = "tls",
                Url = "tls://edge.example.com:443",
            },
            CreateHttpClient(),
            timeoutSeconds: 10,
            pulseAgentByResourceId: new Dictionary<Guid, Guid>(),
            pulseLastSeenById: new Dictionary<Guid, DateTimeOffset?>(),
            cancellationToken: CancellationToken.None);

        Assert.Equal("degraded", status);
    }

    private static MonitorCheckWorker CreateWorker(IMonitorNetworkProbe probe)
    {
        var provider = new ServiceCollection().BuildServiceProvider();
        var scopeFactory = provider.GetRequiredService<IServiceScopeFactory>();
        return new MonitorCheckWorker(
            scopeFactory,
            CreateHttpClientFactory(),
            NullLogger<MonitorCheckWorker>.Instance,
            probe);
    }

    private static IHttpClientFactory CreateHttpClientFactory()
    {
        var services = new ServiceCollection();
        services.AddHttpClient("monitor-checks")
            .ConfigurePrimaryHttpMessageHandler(() => new StubHttpMessageHandler());
        return services.BuildServiceProvider().GetRequiredService<IHttpClientFactory>();
    }

    private static HttpClient CreateHttpClient()
        => new(new StubHttpMessageHandler())
        {
            BaseAddress = new Uri("http://localhost"),
        };

    private sealed class StubHttpMessageHandler : HttpMessageHandler
    {
        protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
            => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK));
    }

    private sealed class FakeMonitorNetworkProbe : IMonitorNetworkProbe
    {
        public bool TcpResult { get; init; }
        public bool UdpResult { get; init; }
        public bool DnsResult { get; init; }
        public bool PingResult { get; init; }
        public TlsProbeResult TlsResult { get; init; } = new(false, null);

        public Task<bool> CheckTcpAsync(string host, int port, TimeSpan timeout, CancellationToken cancellationToken)
            => Task.FromResult(TcpResult);

        public Task<bool> ProbeUdpAsync(string host, int port, TimeSpan timeout, CancellationToken cancellationToken)
            => Task.FromResult(UdpResult);

        public Task<bool> ResolveDnsAsync(string host, TimeSpan timeout, CancellationToken cancellationToken)
            => Task.FromResult(DnsResult);

        public Task<TlsProbeResult> CheckTlsAsync(string host, int port, TimeSpan timeout, CancellationToken cancellationToken)
            => Task.FromResult(TlsResult);

        public Task<bool> PingAsync(string host, TimeSpan timeout, CancellationToken cancellationToken)
            => Task.FromResult(PingResult);
    }
}
