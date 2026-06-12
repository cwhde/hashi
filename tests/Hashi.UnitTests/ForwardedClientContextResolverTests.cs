using System.Net;
using Hashi.Api.Hosting;
using Microsoft.AspNetCore.Http;
using Microsoft.Extensions.Configuration;
using Xunit;

namespace Hashi.UnitTests;

public sealed class ForwardedClientContextResolverTests
{
    [Fact]
    public void Trusted_proxy_uses_forwarded_client_ip_and_method()
    {
        var context = new DefaultHttpContext();
        context.Connection.RemoteIpAddress = IPAddress.Parse("172.18.0.4");
        context.Request.Method = "GET";
        context.Request.Headers["X-Forwarded-For"] = "203.0.113.44, 172.18.0.4";
        context.Request.Headers["X-Forwarded-Method"] = "post";

        var resolved = CreateResolver().Resolve(context);

        Assert.Equal("203.0.113.44", resolved.ClientIp.ToString());
        Assert.Equal("POST", resolved.Method);
        Assert.True(resolved.TrustedProxy);
    }

    [Fact]
    public void Untrusted_direct_request_cannot_spoof_forwarded_ip_or_method()
    {
        var context = new DefaultHttpContext();
        context.Connection.RemoteIpAddress = IPAddress.Parse("198.51.100.9");
        context.Request.Method = "GET";
        context.Request.Headers["X-Forwarded-For"] = "203.0.113.44";
        context.Request.Headers["X-Forwarded-Method"] = "DELETE";

        var resolved = CreateResolver().Resolve(context);

        Assert.Equal("198.51.100.9", resolved.ClientIp.ToString());
        Assert.Equal("GET", resolved.Method);
        Assert.False(resolved.TrustedProxy);
    }

    [Fact]
    public void Trusted_proxy_walks_forwarded_chain_from_the_trusted_boundary()
    {
        var context = new DefaultHttpContext();
        context.Connection.RemoteIpAddress = IPAddress.Parse("172.18.0.4");
        context.Request.Headers["X-Forwarded-For"] = "198.51.100.200, 203.0.113.44, 172.18.0.5";

        var resolved = CreateResolver().Resolve(context);

        Assert.Equal("203.0.113.44", resolved.ClientIp.ToString());
    }

    private static ForwardedClientContextResolver CreateResolver()
        => new(new ConfigurationBuilder().Build());
}
