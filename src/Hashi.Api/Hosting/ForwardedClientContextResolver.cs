using System.Net;
using Microsoft.AspNetCore.Http;
using Microsoft.Extensions.Configuration;

namespace Hashi.Api.Hosting;

public sealed record ForwardedClientContext(IPAddress ClientIp, string Method, bool TrustedProxy);

public sealed class ForwardedClientContextResolver(IConfiguration configuration)
{
    private static readonly string[] DefaultTrustedProxyCidrs =
    [
        "127.0.0.0/8",
        "::1/128",
        "172.16.0.0/12",
    ];

    public ForwardedClientContext Resolve(HttpContext context)
    {
        if (!TryResolve(context, out var resolved))
        {
            throw new InvalidOperationException("A canonical client IP address could not be resolved.");
        }

        return resolved;
    }

    public bool TryResolve(HttpContext context, out ForwardedClientContext resolved)
    {
        var remoteIp = context.Connection.RemoteIpAddress;
        if (remoteIp is null)
        {
            resolved = default!;
            return false;
        }

        remoteIp = Normalize(remoteIp);
        if (!IsTrustedProxy(remoteIp))
        {
            resolved = new ForwardedClientContext(remoteIp, NormalizeMethod(context.Request.Method), false);
            return true;
        }

        if (!TryResolveForwardedClientIp(context, remoteIp, out var clientIp))
        {
            resolved = default!;
            return false;
        }

        var method = context.Request.Headers["X-Forwarded-Method"].FirstOrDefault()
            ?? context.Request.Headers["X-Original-Method"].FirstOrDefault()
            ?? context.Request.Headers["X-Forwarded-Http-Method"].FirstOrDefault()
            ?? context.Request.Method;
        resolved = new ForwardedClientContext(clientIp, NormalizeMethod(method), true);
        return true;
    }

    private bool TryResolveForwardedClientIp(
        HttpContext context,
        IPAddress remoteIp,
        out IPAddress clientIp)
    {
        if (context.Request.Headers.TryGetValue("X-Forwarded-For", out var forwardedValues))
        {
            var forwardedFor = forwardedValues.ToString();
            if (string.IsNullOrWhiteSpace(forwardedFor))
            {
                clientIp = default!;
                return false;
            }

            var values = forwardedFor.Split(',', StringSplitOptions.TrimEntries);
            if (values.Length == 0 || values.Any(string.IsNullOrWhiteSpace))
            {
                clientIp = default!;
                return false;
            }

            var chain = new List<IPAddress>(values.Length + 1);
            foreach (var value in values)
            {
                if (!IPAddress.TryParse(StripPort(value), out var parsed))
                {
                    clientIp = default!;
                    return false;
                }

                chain.Add(Normalize(parsed));
            }

            chain.Add(remoteIp);
            for (var index = chain.Count - 1; index >= 0; index--)
            {
                if (!IsTrustedProxy(chain[index]))
                {
                    clientIp = Normalize(chain[index]);
                    return true;
                }
            }

            clientIp = Normalize(chain[0]);
            return true;
        }

        if (context.Request.Headers.TryGetValue("X-Real-IP", out var realIpValues)
            || context.Request.Headers.TryGetValue("X-Forwarded-Client-IP", out realIpValues))
        {
            var realIp = realIpValues.ToString();
            if (!IPAddress.TryParse(StripPort(realIp), out var parsed))
            {
                clientIp = default!;
                return false;
            }

            clientIp = Normalize(parsed);
            return true;
        }

        clientIp = remoteIp;
        return true;
    }

    private bool IsTrustedProxy(IPAddress remoteIp)
    {
        if (IPAddress.IsLoopback(remoteIp))
        {
            return true;
        }

        foreach (var cidr in ConfiguredTrustedProxyCidrs())
        {
            if (IsInCidr(remoteIp, cidr))
            {
                return true;
            }
        }

        return false;
    }

    private IReadOnlyList<string> ConfiguredTrustedProxyCidrs()
    {
        var configured = configuration.GetSection("Hashi:ForwardAuth:TrustedProxyCidrs").Get<string[]>()
            ?? configuration.GetSection("Hashi:EdgeAuth:TrustedProxyCidrs").Get<string[]>();
        return configured is { Length: > 0 } ? configured : DefaultTrustedProxyCidrs;
    }

    private static string NormalizeMethod(string? method)
        => string.IsNullOrWhiteSpace(method) ? "GET" : method.Trim().ToUpperInvariant();

    private static string? StripPort(string? value)
    {
        if (string.IsNullOrWhiteSpace(value))
        {
            return null;
        }

        var trimmed = value.Trim();
        if (trimmed.StartsWith('['))
        {
            var closing = trimmed.IndexOf(']');
            return closing > 0 ? trimmed[1..closing] : trimmed;
        }

        var colon = trimmed.LastIndexOf(':');
        return colon > 0 && trimmed.Count(x => x == ':') == 1 ? trimmed[..colon] : trimmed;
    }

    private static bool IsInCidr(IPAddress ip, string cidr)
    {
        if (string.IsNullOrWhiteSpace(cidr) || !cidr.Contains('/'))
        {
            return false;
        }

        var parts = cidr.Split('/', 2);
        if (!IPAddress.TryParse(parts[0], out var network) || !int.TryParse(parts[1], out var prefixLength))
        {
            return false;
        }

        var ipBytes = ip.MapToIPv6().GetAddressBytes();
        var networkBytes = network.MapToIPv6().GetAddressBytes();
        if (prefixLength <= 32 && ip.AddressFamily == System.Net.Sockets.AddressFamily.InterNetwork && network.AddressFamily == System.Net.Sockets.AddressFamily.InterNetwork)
        {
            ipBytes = ip.GetAddressBytes();
            networkBytes = network.GetAddressBytes();
        }

        if (ipBytes.Length != networkBytes.Length || prefixLength < 0 || prefixLength > ipBytes.Length * 8)
        {
            return false;
        }

        var fullBytes = prefixLength / 8;
        var remainingBits = prefixLength % 8;
        for (var i = 0; i < fullBytes; i++)
        {
            if (ipBytes[i] != networkBytes[i])
            {
                return false;
            }
        }

        if (remainingBits == 0)
        {
            return true;
        }

        var mask = (byte)(0xFF << (8 - remainingBits));
        return (ipBytes[fullBytes] & mask) == (networkBytes[fullBytes] & mask);
    }

    private static IPAddress Normalize(IPAddress address)
        => address.IsIPv4MappedToIPv6 ? address.MapToIPv4() : address;
}
