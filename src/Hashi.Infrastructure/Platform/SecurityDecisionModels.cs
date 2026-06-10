using System.Net;
using Hashi.Infrastructure.Persistence.Entities;

namespace Hashi.Infrastructure.Platform;

public static class SecurityDecisionActionNames
{
    public const string AllowUpstream = "allow_upstream";
    public const string RequireSso = "require_sso";
    public const string RequireChallenge = "require_challenge";
    public const string DenyInvalidMetadata = "deny_invalid_metadata";
    public const string DenyManualBlock = "deny_manual_block";
    public const string DenyFirewallBlocked = "deny_firewall_blocked";
    public const string DenyBlocklist = "deny_blocklist";
    public const string DenySoftBlock = "deny_soft_block";
    public const string DenyResourceRule = "deny_resource_rule";
    public const string DenyRateLimited = "deny_rate_limited";
}

public static class SecurityDecisionResponseModeNames
{
    public const string Allow = "allow";
    public const string Deny = "deny";
    public const string Redirect = "redirect";
    public const string ApiChallenge = "api_challenge";
    public const string RateLimited = "rate_limited";
}

public sealed record SecurityDecisionRequest(
    string Host,
    string Path,
    IPAddress ClientIp,
    string? CountryCode,
    string? RegionCode,
    string? Asn,
    string? EdgeSessionKey = null,
    string? Mode = null,
    bool TrustedForwardedContext = true,
    string Method = "GET",
    string? AcceptHeader = null);

public sealed record SecurityDecisionExplanation(string Step, string Outcome, string Details);

public sealed record SecurityDecisionMatchedState(
    Guid? SecuritySubjectId,
    Guid? SecuritySubjectStateId,
    string? CurrentState,
    bool ChallengeRequired,
    int RequestsWhileChallenged,
    DateTimeOffset? SoftBlockedUntilUtc,
    DateTimeOffset? FirewallBlockedUntilUtc,
    DateTimeOffset? RateLimitedUntilUtc = null,
    int RateLimitRequestCount = 0);

public sealed record SecurityDecisionResult(
    string Action,
    string ResponseMode,
    int StatusCode,
    string? RedirectUrl,
    string Decision,
    string Reason,
    Guid? ResourceId,
    NormalizedSecuritySubject Subject,
    IReadOnlyList<SecurityDecisionExplanation> Explanation,
    IReadOnlyList<Guid> MatchedManualEntryIds,
    IReadOnlyList<Guid> MatchedBlocklistEntryIds,
    IReadOnlyList<Guid> MatchedResourceRuleIds,
    SecurityDecisionMatchedState? MatchedState)
{
    public static SecurityDecisionResult Create(
        string action,
        string responseMode,
        int statusCode,
        string? redirectUrl,
        string decision,
        string reason,
        Guid? resourceId,
        NormalizedSecuritySubject subject,
        IReadOnlyList<SecurityDecisionExplanation> explanation,
        IReadOnlyList<Guid>? matchedManualEntryIds = null,
        IReadOnlyList<Guid>? matchedBlocklistEntryIds = null,
        IReadOnlyList<Guid>? matchedResourceRuleIds = null,
        SecurityDecisionMatchedState? matchedState = null)
        => new(
            action,
            responseMode,
            statusCode,
            redirectUrl,
            decision,
            reason,
            resourceId,
            subject,
            explanation,
            matchedManualEntryIds ?? [],
            matchedBlocklistEntryIds ?? [],
            matchedResourceRuleIds ?? [],
            matchedState);
}

internal sealed record SecurityDecisionContext(
    ResourceEntity? Resource,
    string NormalizedHost,
    bool HasOidcProvider,
    bool HasValidSession,
    string? RootDomain,
    bool IsPublicChallengeResource);
