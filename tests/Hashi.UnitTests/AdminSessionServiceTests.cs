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
        var vaultSession = new VaultSessionState();
        var service = new AdminSessionService(db, new AppSettingsService(db), vaultSession, clock);
        var session = await service.CreateAsync(
            AdminAuthMethods.Passkey,
            "203.0.113.10",
            AdminSessionScopes.All);
        vaultSession.UnlockForSession(session.Id, new byte[32]);

        var result = await service.ValidateAsync(session.Id, "203.0.113.11");

        Assert.False(result.IsValid);
        Assert.Equal(AdminSessionInvalidReason.IpMismatch, result.InvalidReason);
        Assert.Equal("ip_mismatch", (await db.AdminSessions.SingleAsync()).RevocationReason);
        Assert.False(vaultSession.IsUnlockedForSession(session.Id));
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

    [Fact]
    public async Task Validate_rejects_session_at_idle_deadline()
    {
        await using var db = CreateDb();
        var clock = new MutableTimeProvider(new DateTimeOffset(2026, 6, 12, 8, 0, 0, TimeSpan.Zero));
        var service = CreateService(db, clock);
        var session = await service.CreateAsync(
            AdminAuthMethods.Passkey,
            "203.0.113.10",
            AdminSessionScopes.All);

        clock.Advance(TimeSpan.FromHours(4));
        var result = await service.ValidateAsync(session.Id, "203.0.113.10");

        Assert.False(result.IsValid);
        Assert.Equal(AdminSessionInvalidReason.IdleExpired, result.InvalidReason);
        Assert.Equal("idle_expired", session.RevocationReason);
    }

    [Fact]
    public async Task Validate_rejects_continuously_active_session_at_absolute_deadline()
    {
        await using var db = CreateDb();
        var clock = new MutableTimeProvider(new DateTimeOffset(2026, 6, 12, 8, 0, 0, TimeSpan.Zero));
        var service = CreateService(db, clock);
        var session = await service.CreateAsync(
            AdminAuthMethods.Passkey,
            "203.0.113.10",
            AdminSessionScopes.All);
        session.IdleExpiresAtUtc = session.AbsoluteExpiresAtUtc;
        await db.SaveChangesAsync();

        clock.Advance(TimeSpan.FromHours(8));
        var result = await service.ValidateAsync(session.Id, "203.0.113.10");

        Assert.False(result.IsValid);
        Assert.Equal(AdminSessionInvalidReason.AbsoluteExpired, result.InvalidReason);
        Assert.Equal("absolute_expired", session.RevocationReason);
    }

    [Fact]
    public async Task Revocation_invalidates_session_before_cookie_expiry()
    {
        await using var db = CreateDb();
        var clock = new MutableTimeProvider(new DateTimeOffset(2026, 6, 12, 8, 0, 0, TimeSpan.Zero));
        var service = CreateService(db, clock);
        var session = await service.CreateAsync(
            AdminAuthMethods.Passkey,
            "203.0.113.10",
            AdminSessionScopes.All);

        await service.RevokeAsync(session.Id, "manual");
        var result = await service.ValidateAsync(session.Id, "203.0.113.10");

        Assert.False(result.IsValid);
        Assert.Equal(AdminSessionInvalidReason.Revoked, result.InvalidReason);
    }

    [Theory]
    [InlineData("logout")]
    [InlineData("manual")]
    public async Task Revocation_audit_preserves_cause_without_exposing_session_token(string reason)
    {
        await using var db = CreateDb();
        var clock = new MutableTimeProvider(new DateTimeOffset(2026, 6, 12, 8, 0, 0, TimeSpan.Zero));
        var service = CreateService(db, clock);
        var session = await service.CreateAsync(
            AdminAuthMethods.Passkey,
            "203.0.113.10",
            AdminSessionScopes.All);

        await service.RevokeAsync(session.Id, reason);

        var audit = await db.AuditEvents.SingleAsync(x => x.Action == "session_revoked");
        Assert.Contains($"\"reason\":\"{reason}\"", audit.MetadataJson, StringComparison.Ordinal);
        Assert.DoesNotContain(session.Id, audit.SubjectId ?? string.Empty, StringComparison.Ordinal);
        Assert.DoesNotContain(session.Id, audit.MetadataJson ?? string.Empty, StringComparison.Ordinal);
    }

    [Fact]
    public async Task Validate_rejects_and_revokes_malformed_client_address()
    {
        await using var db = CreateDb();
        var clock = new MutableTimeProvider(new DateTimeOffset(2026, 6, 12, 8, 0, 0, TimeSpan.Zero));
        var service = CreateService(db, clock);
        var session = await service.CreateAsync(
            AdminAuthMethods.Passkey,
            "203.0.113.10",
            AdminSessionScopes.All);

        var result = await service.ValidateAsync(session.Id, "not-an-ip");

        Assert.False(result.IsValid);
        Assert.Equal(AdminSessionInvalidReason.InvalidClientIp, result.InvalidReason);
        Assert.Equal("client_ip_invalid", session.RevocationReason);
    }

    [Fact]
    public async Task Validate_rejects_and_revokes_malformed_stored_address()
    {
        await using var db = CreateDb();
        var clock = new MutableTimeProvider(new DateTimeOffset(2026, 6, 12, 8, 0, 0, TimeSpan.Zero));
        var service = CreateService(db, clock);
        var session = await service.CreateAsync(
            AdminAuthMethods.Passkey,
            "203.0.113.10",
            AdminSessionScopes.All);
        session.BoundIp = "not-an-ip";
        await db.SaveChangesAsync();

        var result = await service.ValidateAsync(session.Id, "203.0.113.10");

        Assert.False(result.IsValid);
        Assert.Equal(AdminSessionInvalidReason.InvalidClientIp, result.InvalidReason);
        Assert.Equal("client_ip_invalid", session.RevocationReason);
    }

    [Fact]
    public async Task Validate_rejects_unknown_session_without_creating_audit_data()
    {
        await using var db = CreateDb();
        var service = CreateService(
            db,
            new MutableTimeProvider(new DateTimeOffset(2026, 6, 12, 8, 0, 0, TimeSpan.Zero)));

        var result = await service.ValidateAsync("unknown-session", "203.0.113.10");

        Assert.False(result.IsValid);
        Assert.Equal(AdminSessionInvalidReason.Unknown, result.InvalidReason);
        Assert.Empty(db.AuditEvents);
    }

    [Fact]
    public async Task Unknown_scopes_are_removed_and_do_not_grant_access()
    {
        await using var db = CreateDb();
        var clock = new MutableTimeProvider(new DateTimeOffset(2026, 6, 12, 8, 0, 0, TimeSpan.Zero));
        var service = CreateService(db, clock);
        var session = await service.CreateAsync(
            AdminAuthMethods.Passkey,
            "203.0.113.10",
            [AdminSessionScopes.Read, "admin.superuser"]);

        var result = await service.ValidateAsync(session.Id, "203.0.113.10");

        Assert.Equal([AdminSessionScopes.Read], result.Scopes);
        Assert.False(await service.HasScopeAsync(session.Id, "admin.superuser"));
    }

    [Fact]
    public async Task Activity_renewal_is_audited_without_exposing_session_token()
    {
        await using var db = CreateDb();
        var clock = new MutableTimeProvider(new DateTimeOffset(2026, 6, 12, 8, 0, 0, TimeSpan.Zero));
        var service = CreateService(db, clock);
        var session = await service.CreateAsync(
            AdminAuthMethods.Passkey,
            "203.0.113.10",
            AdminSessionScopes.All);

        clock.Advance(AdminSessionService.ActivityWriteInterval);
        await service.ValidateAsync(session.Id, "203.0.113.10");

        Assert.Contains(db.AuditEvents, x => x.Action == "session_activity_renewed");
        Assert.All(db.AuditEvents, audit =>
        {
            Assert.NotEqual(session.Id, audit.SubjectId);
            Assert.DoesNotContain(session.Id, audit.SubjectId ?? string.Empty, StringComparison.Ordinal);
            Assert.DoesNotContain(session.Id, audit.MetadataJson ?? string.Empty, StringComparison.Ordinal);
        });
    }

    [Fact]
    public async Task Cleanup_audits_expiry_and_removes_vault_material()
    {
        await using var db = CreateDb();
        var clock = new MutableTimeProvider(new DateTimeOffset(2026, 6, 12, 8, 0, 0, TimeSpan.Zero));
        var vaultSession = new VaultSessionState();
        var service = new AdminSessionService(db, new AppSettingsService(db), vaultSession, clock);
        var session = await service.CreateAsync(
            AdminAuthMethods.Passkey,
            "203.0.113.10",
            AdminSessionScopes.All);
        vaultSession.UnlockForSession(session.Id, new byte[32]);
        session.IdleExpiresAtUtc = session.AbsoluteExpiresAtUtc;
        await db.SaveChangesAsync();

        clock.Advance(TimeSpan.FromHours(8));
        await service.CleanupAsync();

        Assert.Equal("absolute_expired", session.RevocationReason);
        Assert.False(vaultSession.IsUnlockedForSession(session.Id));
        Assert.Contains(db.AuditEvents, x => x.Action == "session_absolute_expired");
    }

    [Fact]
    public async Task Reauthentication_failure_is_audited_without_exposing_session_token()
    {
        await using var db = CreateDb();
        var clock = new MutableTimeProvider(new DateTimeOffset(2026, 6, 12, 8, 0, 0, TimeSpan.Zero));
        var service = CreateService(db, clock);
        var session = await service.CreateAsync(
            AdminAuthMethods.Passkey,
            "203.0.113.10",
            AdminSessionScopes.All);

        await service.RecordReauthenticationFailureAsync(session.Id, "assertion_failed");

        var audit = await db.AuditEvents.SingleAsync(x => x.Action == "session_reauthentication_failed");
        Assert.Equal("failure", audit.Outcome);
        Assert.DoesNotContain(session.Id, audit.SubjectId ?? string.Empty, StringComparison.Ordinal);
        Assert.DoesNotContain(session.Id, audit.MetadataJson ?? string.Empty, StringComparison.Ordinal);
    }

    private static AdminSessionService CreateService(HashiDbContext db, TimeProvider clock)
        => new(db, new AppSettingsService(db), new VaultSessionState(), clock);

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
