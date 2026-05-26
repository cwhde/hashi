using Hashi.Api.Hosting;
using Microsoft.AspNetCore.Http;
using Xunit;

namespace Hashi.UnitTests;

public sealed class AdminApiAuthMiddlewareTests
{
    [Theory]
    [InlineData("/api/vault/secrets/abc", "POST", true)]
    [InlineData("/api/scripts", "POST", true)]
    [InlineData("/api/scripts/abc/run", "POST", true)]
    [InlineData("/api/connections/ssh", "POST", true)]
    [InlineData("/api/dns/connections/hetzner", "POST", true)]
    [InlineData("/api/settings/general", "PUT", true)]
    [InlineData("/api/sync/apply", "POST", true)]
    [InlineData("/api/firewall/apply", "POST", true)]
    [InlineData("/api/traefik/apply", "POST", true)]
    [InlineData("/api/resources/abc", "DELETE", true)]
    [InlineData("/api/security/blocklist/sync", "POST", true)]
    [InlineData("/api/pulse/agents", "POST", true)]
    [InlineData("/api/resources", "GET", false)]
    [InlineData("/api/resources", "POST", false)]
    [InlineData("/api/sync/runs", "GET", false)]
    public void RequiresReauthentication_matches_spec_paths(string path, string method, bool expected)
    {
        var actual = AdminApiAuthMiddleware.RequiresReauthentication(new PathString(path), method);
        Assert.Equal(expected, actual);
    }
}

public sealed class DnsDesiredStateBuilderTests
{
    [Fact]
    public void MergeRecords_generated_overrides_manual_with_same_key()
    {
        var manual = new[]
        {
            new Hashi.Core.Dns.DnsRecordSnapshot("", "app.example.com", Hashi.Core.Dns.DnsRecordType.A, "1.2.3.4", 3600, false),
        };
        var generated = new[]
        {
            new Hashi.Core.Dns.DnsRecordSnapshot("", "app.example.com", Hashi.Core.Dns.DnsRecordType.A, "203.0.113.10", 3600, true),
        };

        var merged = Hashi.Infrastructure.Dns.DnsDesiredStateBuilder.MergeRecords(manual, generated);

        Assert.Single(merged);
        Assert.Equal("203.0.113.10", merged[0].Value);
        Assert.True(merged[0].IsManagedByHashi);
    }
}
