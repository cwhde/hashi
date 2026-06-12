using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class AdminSessionServiceTests
{
    [Fact]
    public async Task Create_uses_four_hour_idle_and_eight_hour_absolute_limits()
    {
        await using var db = CreateDb();
        db.AppSettings.Add(new AppSettingsEntity
        {
            AdminSessionMinutes = 999,
            AdminSessionAbsoluteMinutes = 999,
        });
        await db.SaveChangesAsync();
        var now = new DateTimeOffset(2026, 6, 12, 8, 0, 0, TimeSpan.Zero);
        var service = CreateService(db, new MutableTimeProvider(now));

        var session = await service.CreateAsync(
            AdminAuthMethods.Passkey,
            "::ffff:203.0.113.10",
            AdminSessionScopes.All);

        Assert.Equal("203.0.113.10", session.BoundIp);
        Assert.Equal(now.AddHours(4), session.IdleExpiresAtUtc);
        Assert.Equal(now.AddHours(8), session.AbsoluteExpiresAtUtc);
    }

    [Fact]
    public async Task Validate_rejects_and_revokes_token_used_from_another_ip()
    {
        await using var db = CreateDb();
        var clock = new MutableTimeProvider(new DateTimeOffset(2026, 6, 12, 8, 0, 0, TimeSpan.Zero));
        var service = CreateService(db, clock);
        var session = await service.CreateAsync(
            AdminAuthMethods.Passkey,
            "203.0.113.10",
            AdminSessionScopes.All);

        var result = await service.ValidateAsync(session.Id, "203.0.113.11");

        Assert.False(result.IsValid);
        Assert.Equal(AdminSessionInvalidReason.IpMismatch, result.InvalidReason);
        Assert.Equal("ip_mismatch", (await db.AdminSessions.SingleAsync()).RevocationReason);
    }

    [Fact]
    public async Task Activity_extends_idle_expiry_but_never_beyond_absolute_expiry()
    {
        await using var db = CreateDb();
        var clock = new MutableTimeProvider(new DateTimeOffset(2026, 6, 12, 8, 0, 0, TimeSpan.Zero));
        var service = CreateService(db, clock);
        var session = await service.CreateAsync(
            AdminAuthMethods.Passkey,
            "203.0.113.10",
            AdminSessionScopes.All);

        clock.Advance(TimeSpan.FromHours(3));
        var result = await service.ValidateAsync(session.Id, "203.0.113.10");

        Assert.True(result.IsValid);
        Assert.Equal(clock.GetUtcNow().AddHours(4), result.Session!.IdleExpiresAtUtc);

        clock.Advance(TimeSpan.FromHours(3));
        result = await service.ValidateAsync(session.Id, "203.0.113.10");

        Assert.True(result.IsValid);
        Assert.Equal(session.AbsoluteExpiresAtUtc, result.Session!.IdleExpiresAtUtc);
    }

    [Fact]
    public async Task Recent_reauthentication_does_not_extend_absolute_expiry()
    {
        await using var db = CreateDb();
        var clock = new MutableTimeProvider(new DateTimeOffset(2026, 6, 12, 8, 0, 0, TimeSpan.Zero));
        var service = CreateService(db, clock);
        var session = await service.CreateAsync(
            AdminAuthMethods.Passkey,
            "203.0.113.10",
            AdminSessionScopes.All);
        var absoluteExpiry = session.AbsoluteExpiresAtUtc;

        clock.Advance(TimeSpan.FromHours(7));
        session.IdleExpiresAtUtc = absoluteExpiry;
        await db.SaveChangesAsync();
        await service.MarkReauthenticatedAsync(session.Id);

        Assert.Equal(absoluteExpiry, session.AbsoluteExpiresAtUtc);
        Assert.True(AdminSessionService.IsRecentlyReauthenticated(session, clock.GetUtcNow()));

        clock.Advance(TimeSpan.FromHours(1));
        var result = await service.ValidateAsync(session.Id, "203.0.113.10");
        Assert.Equal(AdminSessionInvalidReason.AbsoluteExpired, result.InvalidReason);
    }

    private static AdminSessionService CreateService(HashiDbContext db, TimeProvider clock)
        => new(db, new AppSettingsService(db), clock);

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private sealed class MutableTimeProvider(DateTimeOffset utcNow) : TimeProvider
    {
        public override DateTimeOffset GetUtcNow() => utcNow;

        public void Advance(TimeSpan duration) => utcNow = utcNow.Add(duration);
    }
}
