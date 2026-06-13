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

    [Fact]
    public void Trusted_proxy_uses_leftmost_address_when_entire_chain_is_trusted()
    {
        var context = new DefaultHttpContext();
        context.Connection.RemoteIpAddress = IPAddress.Loopback;
        context.Request.Headers["X-Forwarded-For"] = "127.0.0.2, 127.0.0.3";

        var resolved = CreateResolver().Resolve(context);

        Assert.Equal("127.0.0.2", resolved.ClientIp.ToString());
    }

    [Fact]
    public void Trusted_proxy_normalizes_ipv4_mapped_ipv6_client()
    {
        var context = new DefaultHttpContext();
        context.Connection.RemoteIpAddress = IPAddress.Loopback;
        context.Request.Headers["X-Forwarded-For"] = "::ffff:203.0.113.44";

        var resolved = CreateResolver().Resolve(context);

        Assert.Equal("203.0.113.44", resolved.ClientIp.ToString());
    }

    [Fact]
    public void Missing_direct_peer_address_fails_closed()
    {
        var context = new DefaultHttpContext();
        context.Connection.RemoteIpAddress = null;

        Assert.False(CreateResolver().TryResolve(context, out _));
    }

    [Theory]
    [InlineData(" ")]
    [InlineData("not-an-ip")]
    [InlineData("203.0.113.44:")]
    [InlineData("203.0.113.44:not-a-port")]
    [InlineData("203.0.113.44:65536")]
    [InlineData("[::ffff:203.0.113.44]junk")]
    [InlineData("[::ffff:203.0.113.44]:not-a-port")]
    [InlineData("203.0.113.44, not-an-ip")]
    [InlineData("203.0.113.44, , 172.18.0.4")]
    public void Trusted_proxy_rejects_malformed_forwarded_chain(string forwardedFor)
    {
        var context = new DefaultHttpContext();
        context.Connection.RemoteIpAddress = IPAddress.Parse("172.18.0.4");
        context.Request.Headers["X-Forwarded-For"] = forwardedFor;

        Assert.False(CreateResolver().TryResolve(context, out _));
    }

    [Theory]
    [InlineData("203.0.113.44:443", "203.0.113.44")]
    [InlineData("[2001:db8::44]", "2001:db8::44")]
    [InlineData("[2001:db8::44]:443", "2001:db8::44")]
    public void Trusted_proxy_accepts_valid_address_with_port(string forwardedFor, string expected)
    {
        var context = new DefaultHttpContext();
        context.Connection.RemoteIpAddress = IPAddress.Parse("172.18.0.4");
        context.Request.Headers["X-Forwarded-For"] = forwardedFor;

        var resolved = CreateResolver().Resolve(context);

        Assert.Equal(expected, resolved.ClientIp.ToString());
    }

    [Fact]
    public void Trusted_proxy_rejects_malformed_real_ip()
    {
        var context = new DefaultHttpContext();
        context.Connection.RemoteIpAddress = IPAddress.Parse("172.18.0.4");
        context.Request.Headers["X-Real-IP"] = "not-an-ip";

        Assert.False(CreateResolver().TryResolve(context, out _));
    }

    private static ForwardedClientContextResolver CreateResolver()
        => new(new ConfigurationBuilder().Build());
}
