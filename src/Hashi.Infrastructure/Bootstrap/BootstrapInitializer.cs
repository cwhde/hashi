using System.Security.Cryptography;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Bootstrap;

public sealed class BootstrapInitializer(
    HashiDbContext db,
    SetupStateService setupState,
    AuditService audit,
    ILogger<BootstrapInitializer> logger)
{
    private const string UsernameAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789";

    public async Task EnsureBootstrapCredentialsAsync(CancellationToken cancellationToken = default)
    {
        var state = await setupState.GetOrCreateAsync(cancellationToken);
        if (state.IsComplete || !string.IsNullOrEmpty(state.BootstrapPasswordHash))
        {
            return;
        }

        var username = $"hashi-{GenerateToken(6)}";
        var password = GenerateToken(24);
        state.BootstrapUsername = username;
        state.BootstrapPasswordHash = BCrypt.Net.BCrypt.HashPassword(password, workFactor: 12);
        state.UpdatedAtUtc = DateTimeOffset.UtcNow;
        await db.SaveChangesAsync(cancellationToken);

        logger.LogWarning(
            "Hashi bootstrap credentials generated. Username: {Username} Password: {Password}. " +
            "These are shown once in logs and must be replaced during passkey setup.",
            username,
            password);

        await audit.WriteAsync("setup", "bootstrap_credentials_generated", subjectType: "setup", cancellationToken: cancellationToken);
    }

    private static string GenerateToken(int length)
    {
        Span<char> chars = stackalloc char[length];
        for (var i = 0; i < length; i++)
        {
            chars[i] = UsernameAlphabet[RandomNumberGenerator.GetInt32(UsernameAlphabet.Length)];
        }

        return new string(chars);
    }
}

public static class BootstrapNetworkPolicy
{
    private static readonly (uint Address, int PrefixLength)[] AllowedRanges =
    [
        (Parse("10.0.0.0"), 8),
        (Parse("172.16.0.0"), 12),
        (Parse("192.168.0.0"), 16),
        (Parse("127.0.0.0"), 8),
    ];

    public static bool IsAllowed(string? remoteIp)
    {
        if (string.IsNullOrWhiteSpace(remoteIp))
        {
            return false;
        }

        if (remoteIp == "::1")
        {
            return true;
        }

        if (!System.Net.IPAddress.TryParse(remoteIp, out var address))
        {
            return false;
        }

        if (address.IsIPv4MappedToIPv6)
        {
            address = address.MapToIPv4();
        }

        if (address.AddressFamily != System.Net.Sockets.AddressFamily.InterNetwork)
        {
            return false;
        }

        var bytes = address.GetAddressBytes();
        var value = ((uint)bytes[0] << 24) | ((uint)bytes[1] << 16) | ((uint)bytes[2] << 8) | bytes[3];
        foreach (var (network, prefix) in AllowedRanges)
        {
            var mask = prefix == 0 ? 0u : uint.MaxValue << (32 - prefix);
            if ((value & mask) == (network & mask))
            {
                return true;
            }
        }

        return false;
    }

    private static uint Parse(string dotted)
    {
        var parts = dotted.Split('.').Select(byte.Parse).ToArray();
        return ((uint)parts[0] << 24) | ((uint)parts[1] << 16) | ((uint)parts[2] << 8) | parts[3];
    }
}
