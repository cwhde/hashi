using System.Net;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Auth;

public sealed class AdminSessionService(
    HashiDbContext db,
    AppSettingsService settingsService,
    VaultSessionState vaultSession,
    TimeProvider? timeProvider = null)
{
    public static readonly TimeSpan RecentReauthenticationWindow = TimeSpan.FromMinutes(5);
    public static readonly TimeSpan ActivityWriteInterval = TimeSpan.FromMinutes(5);

    private readonly TimeProvider _timeProvider = timeProvider ?? TimeProvider.System;

    public async Task<AdminSessionEntity> CreateAsync(
        string authMethod,
        string clientIp,
        IReadOnlyCollection<string> scopes,
        Guid? passkeyCredentialId = null,
        string? userAgent = null,
        CancellationToken cancellationToken = default)
    {
        var now = UtcNow();
        var settings = await settingsService.GetOrCreateAsync(cancellationToken);
        var idleMinutes = Math.Clamp(settings.AdminSessionMinutes, 5, 240);
        var absoluteMinutes = Math.Clamp(settings.AdminSessionAbsoluteMinutes, idleMinutes, 480);
        var absoluteExpiry = now.AddMinutes(absoluteMinutes);
        var entity = new AdminSessionEntity
        {
            Id = Convert.ToHexString(RandomNumberGenerator.GetBytes(32)).ToLowerInvariant(),
            AuthMethod = authMethod,
            PasskeyCredentialId = passkeyCredentialId,
            BoundIp = NormalizeIp(clientIp),
            ScopesJson = JsonSerializer.Serialize(NormalizeScopes(scopes)),
            IdleTimeoutMinutes = idleMinutes,
            CreatedAtUtc = now,
            LastSeenAtUtc = now,
            IdleExpiresAtUtc = Min(now.AddMinutes(idleMinutes), absoluteExpiry),
            AbsoluteExpiresAtUtc = absoluteExpiry,
            UserAgentHash = HashUserAgent(userAgent),
        };

        db.AdminSessions.Add(entity);
        AddAudit("session_issued", entity, metadata: new
        {
            boundIp = entity.BoundIp,
            entity.AuthMethod,
            scopes = DeserializeScopes(entity.ScopesJson),
            entity.IdleExpiresAtUtc,
            entity.AbsoluteExpiresAtUtc,
        });
        await db.SaveChangesAsync(cancellationToken);
        return entity;
    }

    public async Task<AdminSessionValidationResult> ValidateAsync(
        string? sessionId,
        string clientIp,
        CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(sessionId))
        {
            return AdminSessionValidationResult.Invalid(AdminSessionInvalidReason.Unknown);
        }

        var entity = await db.AdminSessions.SingleOrDefaultAsync(x => x.Id == sessionId, cancellationToken);
        if (entity is null)
        {
            return AdminSessionValidationResult.Invalid(AdminSessionInvalidReason.Unknown);
        }

        if (entity.RevokedAtUtc is not null)
        {
            return AdminSessionValidationResult.Invalid(AdminSessionInvalidReason.Revoked, entity);
        }

        var now = UtcNow();
        string boundIp;
        string observedIp;
        try
        {
            boundIp = NormalizeIp(entity.BoundIp);
            observedIp = NormalizeIp(clientIp);
        }
        catch (InvalidOperationException)
        {
            await RevokeTrackedAsync(entity, "client_ip_invalid", "session_ip_invalid", new
            {
                storedIp = entity.BoundIp,
                observedIp = clientIp,
            }, cancellationToken);
            return AdminSessionValidationResult.Invalid(AdminSessionInvalidReason.InvalidClientIp, entity);
        }

        if (!string.Equals(boundIp, observedIp, StringComparison.Ordinal))
        {
            await RevokeTrackedAsync(entity, "ip_mismatch", "session_ip_mismatch", new
            {
                boundIp,
                observedIp,
            }, cancellationToken);
            return AdminSessionValidationResult.Invalid(AdminSessionInvalidReason.IpMismatch, entity);
        }

        if (now >= entity.AbsoluteExpiresAtUtc)
        {
            await RevokeTrackedAsync(entity, "absolute_expired", "session_absolute_expired", null, cancellationToken);
            return AdminSessionValidationResult.Invalid(AdminSessionInvalidReason.AbsoluteExpired, entity);
        }

        if (now >= entity.IdleExpiresAtUtc)
        {
            await RevokeTrackedAsync(entity, "idle_expired", "session_idle_expired", null, cancellationToken);
            return AdminSessionValidationResult.Invalid(AdminSessionInvalidReason.IdleExpired, entity);
        }

        if (now - entity.LastSeenAtUtc >= ActivityWriteInterval)
        {
            entity.LastSeenAtUtc = now;
            entity.IdleExpiresAtUtc = Min(now.AddMinutes(entity.IdleTimeoutMinutes), entity.AbsoluteExpiresAtUtc);
            AddAudit("session_activity_renewed", entity, metadata: new
            {
                entity.LastSeenAtUtc,
                entity.IdleExpiresAtUtc,
            });
            await db.SaveChangesAsync(cancellationToken);
        }

        return AdminSessionValidationResult.Valid(entity, DeserializeScopes(entity.ScopesJson));
    }

    public async Task<bool> HasScopeAsync(
        string sessionId,
        string scope,
        CancellationToken cancellationToken = default)
    {
        var scopesJson = await db.AdminSessions
            .Where(x => x.Id == sessionId && x.RevokedAtUtc == null)
            .Select(x => x.ScopesJson)
            .SingleOrDefaultAsync(cancellationToken);
        return scopesJson is not null && DeserializeScopes(scopesJson).Contains(scope, StringComparer.Ordinal);
    }

    public async Task MarkReauthenticatedAsync(string sessionId, CancellationToken cancellationToken = default)
    {
        var entity = await db.AdminSessions.SingleOrDefaultAsync(x => x.Id == sessionId, cancellationToken)
            ?? throw new InvalidOperationException("Admin session no longer exists.");
        if (entity.RevokedAtUtc is not null || UtcNow() >= entity.AbsoluteExpiresAtUtc || UtcNow() >= entity.IdleExpiresAtUtc)
        {
            throw new InvalidOperationException("Admin session is no longer valid.");
        }

        entity.ReauthenticatedAtUtc = UtcNow();
        AddAudit("session_reauthenticated", entity);
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task<bool> IsRecentlyReauthenticatedAsync(
        string sessionId,
        CancellationToken cancellationToken = default)
    {
        var at = await db.AdminSessions
            .Where(x => x.Id == sessionId && x.RevokedAtUtc == null)
            .Select(x => x.ReauthenticatedAtUtc)
            .SingleOrDefaultAsync(cancellationToken);
        var now = UtcNow();
        return at is not null && now >= at.Value && now - at.Value <= RecentReauthenticationWindow;
    }

    public static bool IsRecentlyReauthenticated(AdminSessionEntity session, DateTimeOffset now)
        => session.ReauthenticatedAtUtc is { } at
            && now >= at
            && now - at <= RecentReauthenticationWindow;

    public async Task RecordReauthenticationFailureAsync(
        string sessionId,
        string reason,
        CancellationToken cancellationToken = default)
    {
        var entity = await db.AdminSessions.SingleOrDefaultAsync(x => x.Id == sessionId, cancellationToken);
        if (entity is null)
        {
            return;
        }

        AddAudit(
            "session_reauthentication_failed",
            entity,
            outcome: "failure",
            metadata: new { reason });
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task RevokeAsync(
        string sessionId,
        string reason,
        CancellationToken cancellationToken = default)
    {
        var entity = await db.AdminSessions.SingleOrDefaultAsync(x => x.Id == sessionId, cancellationToken);
        if (entity is null || entity.RevokedAtUtc is not null)
        {
            return;
        }

        await RevokeTrackedAsync(entity, reason, "session_revoked", null, cancellationToken);
    }

    public async Task<IReadOnlyList<AdminSessionEntity>> ListActiveAsync(CancellationToken cancellationToken = default)
    {
        var now = UtcNow();
        return await db.AdminSessions
            .AsNoTracking()
            .Where(x => x.RevokedAtUtc == null
                && x.IdleExpiresAtUtc > now
                && x.AbsoluteExpiresAtUtc > now)
            .OrderByDescending(x => x.LastSeenAtUtc)
            .ToListAsync(cancellationToken);
    }

    public async Task<bool> RevokeByCorrelationIdAsync(
        string correlationId,
        string reason,
        CancellationToken cancellationToken = default)
    {
        var active = await db.AdminSessions
            .Where(x => x.RevokedAtUtc == null)
            .ToListAsync(cancellationToken);
        var entity = active.SingleOrDefault(x => string.Equals(CorrelationId(x.Id), correlationId, StringComparison.OrdinalIgnoreCase));
        if (entity is null)
        {
            return false;
        }

        await RevokeTrackedAsync(entity, reason, "session_revoked", null, cancellationToken);
        return true;
    }

    public async Task<int> RevokeOtherSessionsAsync(
        string currentSessionId,
        CancellationToken cancellationToken = default)
    {
        var entities = await db.AdminSessions
            .Where(x => x.Id != currentSessionId && x.RevokedAtUtc == null)
            .ToListAsync(cancellationToken);
        foreach (var entity in entities)
        {
            entity.RevokedAtUtc = UtcNow();
            entity.RevocationReason = "revoked_others";
            entity.ReauthenticatedAtUtc = null;
            AddAudit("session_revoked", entity, metadata: new { reason = "revoked_others" });
            vaultSession.LockForSession(entity.Id);
        }

        await db.SaveChangesAsync(cancellationToken);
        return entities.Count;
    }

    public async Task<int> RevokeForPasskeyAsync(
        Guid passkeyCredentialId,
        CancellationToken cancellationToken = default)
    {
        var entities = await db.AdminSessions
            .Where(x => x.PasskeyCredentialId == passkeyCredentialId && x.RevokedAtUtc == null)
            .ToListAsync(cancellationToken);
        foreach (var entity in entities)
        {
            entity.RevokedAtUtc = UtcNow();
            entity.RevocationReason = "passkey_removed";
            entity.ReauthenticatedAtUtc = null;
            AddAudit("session_revoked", entity, metadata: new { reason = "passkey_removed" });
            vaultSession.LockForSession(entity.Id);
        }

        await db.SaveChangesAsync(cancellationToken);
        return entities.Count;
    }

    public async Task<IReadOnlyList<string>> CleanupAsync(CancellationToken cancellationToken = default)
    {
        var now = UtcNow();
        var retentionCutoff = now.AddDays(-7);
        var expired = await db.AdminSessions
            .Where(x => x.RevokedAtUtc == null
                && (x.IdleExpiresAtUtc <= now || x.AbsoluteExpiresAtUtc <= now))
            .ToListAsync(cancellationToken);
        foreach (var entity in expired)
        {
            entity.RevokedAtUtc = now;
            entity.RevocationReason = entity.AbsoluteExpiresAtUtc <= now ? "absolute_expired" : "idle_expired";
            entity.ReauthenticatedAtUtc = null;
            vaultSession.LockForSession(entity.Id);
            AddAudit(
                entity.AbsoluteExpiresAtUtc <= now ? "session_absolute_expired" : "session_idle_expired",
                entity,
                outcome: "rejected");
        }

        var removed = await db.AdminSessions
            .Where(x => x.RevokedAtUtc != null && x.RevokedAtUtc < retentionCutoff)
            .ToListAsync(cancellationToken);
        db.AdminSessions.RemoveRange(removed);
        await db.SaveChangesAsync(cancellationToken);
        return expired.Select(x => x.Id).Concat(removed.Select(x => x.Id)).Distinct(StringComparer.Ordinal).ToArray();
    }

    public async Task RecordScopeFailureAsync(
        AdminSessionEntity session,
        string requiredScope,
        CancellationToken cancellationToken = default)
    {
        AddAudit(
            "session_scope_rejected",
            session,
            outcome: "rejected",
            metadata: new { requiredScope });
        await db.SaveChangesAsync(cancellationToken);
    }

    public static IReadOnlyList<string> GetScopes(AdminSessionEntity session)
        => DeserializeScopes(session.ScopesJson);

    public static string GetCorrelationId(AdminSessionEntity session) => CorrelationId(session.Id);

    public static string GetCorrelationId(string sessionId) => CorrelationId(sessionId);

    public static string NormalizeIp(string value)
    {
        if (!IPAddress.TryParse(value, out var address))
        {
            throw new InvalidOperationException("A valid client IP address is required for an admin session.");
        }

        return address.IsIPv4MappedToIPv6 ? address.MapToIPv4().ToString() : address.ToString();
    }

    private async Task RevokeTrackedAsync(
        AdminSessionEntity entity,
        string reason,
        string auditAction,
        object? metadata,
        CancellationToken cancellationToken)
    {
        entity.RevokedAtUtc = UtcNow();
        entity.RevocationReason = reason;
        entity.ReauthenticatedAtUtc = null;
        vaultSession.LockForSession(entity.Id);
        AddAudit(auditAction, entity, outcome: "rejected", metadata: metadata);
        await db.SaveChangesAsync(cancellationToken);
    }

    private void AddAudit(
        string action,
        AdminSessionEntity entity,
        string outcome = "success",
        object? metadata = null)
    {
        db.AuditEvents.Add(new AuditEventEntity
        {
            Category = "auth",
            Action = action,
            Outcome = outcome,
            SubjectType = "admin_session",
            SubjectId = CorrelationId(entity.Id),
            MetadataJson = metadata is null ? null : JsonSerializer.Serialize(metadata),
            CreatedAtUtc = UtcNow(),
        });
    }

    private static IReadOnlyList<string> NormalizeScopes(IEnumerable<string> scopes)
        => scopes
            .Where(AdminSessionScopes.All.Contains)
            .Distinct(StringComparer.Ordinal)
            .Order(StringComparer.Ordinal)
            .ToArray();

    private static IReadOnlyList<string> DeserializeScopes(string json)
        => JsonSerializer.Deserialize<string[]>(json) ?? [];

    private static string? HashUserAgent(string? userAgent)
        => string.IsNullOrWhiteSpace(userAgent)
            ? null
            : Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(userAgent))).ToLowerInvariant();

    private static string CorrelationId(string sessionId)
        => Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(sessionId))).ToLowerInvariant()[..16];

    private DateTimeOffset UtcNow() => _timeProvider.GetUtcNow();

    private static DateTimeOffset Min(DateTimeOffset left, DateTimeOffset right) => left <= right ? left : right;
}

public sealed record AdminSessionValidationResult(
    bool IsValid,
    AdminSessionInvalidReason? InvalidReason,
    AdminSessionEntity? Session,
    IReadOnlyList<string> Scopes)
{
    public static AdminSessionValidationResult Valid(AdminSessionEntity session, IReadOnlyList<string> scopes)
        => new(true, null, session, scopes);

    public static AdminSessionValidationResult Invalid(
        AdminSessionInvalidReason reason,
        AdminSessionEntity? session = null)
        => new(false, reason, session, []);
}

public enum AdminSessionInvalidReason
{
    Unknown,
    Revoked,
    InvalidClientIp,
    IpMismatch,
    IdleExpired,
    AbsoluteExpired,
}
