using System.Net;
using System.Text.Json;
using Hashi.Core.Resources;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.AspNetCore.Http;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Platform;

public sealed class SecurityDecisionService(
    HashiDbContext db,
    OidcEdgeAuthService oidc,
    GeoIpLookupService? geoIp = null,
    CaptchaChallengeService? captcha = null,
    TimeProvider? timeProvider = null)
{
    public async Task<SecurityDecisionResult> DecideForwardAuthAsync(
        SecurityDecisionRequest request,
        CancellationToken cancellationToken = default)
    {
        var now = timeProvider?.GetUtcNow() ?? DateTimeOffset.UtcNow;
        var subject = SecuritySubjectNormalizer.NormalizeIp(request.ClientIp);
        var explanation = new List<SecurityDecisionExplanation>();

        if (!request.TrustedForwardedContext)
        {
            explanation.Add(new SecurityDecisionExplanation(
                "metadata",
                "deny",
                "Forward-auth request did not come through a trusted proxy context."));
            return Deny(
                SecurityDecisionActionNames.DenyInvalidMetadata,
                "invalid_request_metadata",
                null,
                subject,
                explanation);
        }

        var (subjectEntity, stateEntity) = await new SecuritySubjectService(db, timeProvider)
            .ResolveOrCreateIpAsync(request.ClientIp, request.CountryCode, request.RegionCode, request.Asn, cancellationToken);
        var context = await BuildContextAsync(request, cancellationToken);
        var matchedState = BuildMatchedState(subjectEntity, stateEntity);

        explanation.Add(new SecurityDecisionExplanation(
            "subject",
            "resolved",
            $"{subject.SubjectType}:{subject.NormalizedValue}"));

        var manualEntries = await LoadMatchingManualEntriesAsync(request, context, now, cancellationToken);
        var manualBlock = manualEntries.FirstOrDefault(x => x.EntryType == ManualSecurityEntryTypeNames.Block);
        var legacyManualBlock = manualBlock is null
            ? await LoadMatchingLegacyManualBlocklistAsync(request, now, cancellationToken)
            : null;
        if (legacyManualBlock is not null)
        {
            legacyManualBlock.LastHitAtUtc = now;
            stateEntity.ManualBlockActive = true;
            stateEntity.UpdatedAtUtc = now;
            subjectEntity.CurrentState = SecuritySubjectStateNames.ManuallyBlocked;
            await db.SaveChangesAsync(cancellationToken);
            explanation.Add(new SecurityDecisionExplanation("manual_block", "matched", legacyManualBlock.Reason));
            return Deny(
                SecurityDecisionActionNames.DenyManualBlock,
                "manual_block",
                context.Resource?.Id,
                subject,
                explanation,
                matchedBlocklistEntryIds: [legacyManualBlock.Id],
                matchedState: BuildMatchedState(subjectEntity, stateEntity));
        }

        if (manualBlock is not null)
        {
            manualBlock.LastHitAtUtc = now;
            stateEntity.ManualBlockActive = true;
            stateEntity.UpdatedAtUtc = now;
            subjectEntity.CurrentState = SecuritySubjectStateNames.ManuallyBlocked;
            await db.SaveChangesAsync(cancellationToken);
            explanation.Add(new SecurityDecisionExplanation("manual_block", "matched", manualBlock.Reason ?? manualBlock.Id.ToString()));
            return Deny(
                SecurityDecisionActionNames.DenyManualBlock,
                "manual_block",
                context.Resource?.Id,
                subject,
                explanation,
                matchedManualEntryIds: [manualBlock.Id],
                matchedState: BuildMatchedState(subjectEntity, stateEntity));
        }

        var manualAllow = manualEntries.FirstOrDefault(x => x.EntryType == ManualSecurityEntryTypeNames.Allow);
        var legacyManualAllow = manualAllow is null
            && await LoadMatchingLegacyManualAllowSubjectAsync(request, cancellationToken);
        var manualAllowBypassesBlocking = manualAllow?.BypassBlocking == true || legacyManualAllow;
        var manualAllowBypassesChallenge = manualAllow?.BypassChallenge == true;
        var matchedManualAllowIds = manualAllow is null ? Array.Empty<Guid>() : [manualAllow.Id];
        if (manualAllow is not null)
        {
            manualAllow.LastHitAtUtc = now;
            stateEntity.ManualAllowActive = true;
            stateEntity.UpdatedAtUtc = now;
            await db.SaveChangesAsync(cancellationToken);
            explanation.Add(new SecurityDecisionExplanation("manual_allow", "matched", manualAllow.Reason ?? manualAllow.Id.ToString()));
        }
        else if (legacyManualAllow)
        {
            stateEntity.ManualAllowActive = true;
            stateEntity.UpdatedAtUtc = now;
            await db.SaveChangesAsync(cancellationToken);
            explanation.Add(new SecurityDecisionExplanation("manual_allow", "matched", "legacy firewall allowed subject"));
        }

        if (!manualAllowBypassesBlocking && IsFirewallBlocked(stateEntity, subjectEntity, now))
        {
            explanation.Add(new SecurityDecisionExplanation("firewall_block", "matched", "Active firewall block state."));
            return Deny(
                SecurityDecisionActionNames.DenyFirewallBlocked,
                "firewall_block",
                context.Resource?.Id,
                subject,
                explanation,
                matchedManualEntryIds: matchedManualAllowIds,
                matchedState: BuildMatchedState(subjectEntity, stateEntity));
        }

        var firewallSubjectBlock = await LoadMatchingFirewallSubjectBlockAsync(request, cancellationToken);
        if (firewallSubjectBlock is not null)
        {
            explanation.Add(new SecurityDecisionExplanation("firewall_block", "matched", firewallSubjectBlock.Reason));
            return Deny(
                SecurityDecisionActionNames.DenyFirewallBlocked,
                "firewall_block",
                context.Resource?.Id,
                subject,
                explanation,
                matchedManualEntryIds: matchedManualAllowIds,
                matchedState: BuildMatchedState(subjectEntity, stateEntity));
        }

        var blocklistMatch = await LoadMatchingBlocklistEntryAsync(request, now, firewallOnly: true, cancellationToken);
        if (blocklistMatch is not null && !manualAllowBypassesBlocking)
        {
            blocklistMatch.LastHitAtUtc = now;
            await db.SaveChangesAsync(cancellationToken);
            explanation.Add(new SecurityDecisionExplanation("blocklist_firewall", "matched", blocklistMatch.Reason));
            return Deny(
                SecurityDecisionActionNames.DenyBlocklist,
                "blocklist_firewall_enforced",
                context.Resource?.Id,
                subject,
                explanation,
                matchedBlocklistEntryIds: [blocklistMatch.Id],
                matchedState: BuildMatchedState(subjectEntity, stateEntity));
        }

        if (context.IsPublicChallengeResource)
        {
            explanation.Add(new SecurityDecisionExplanation("captcha_public_resource", "allow", "Public challenge resource bypasses SSO, CAPTCHA, adaptive challenge, and soft blocks."));
            return Allow(context.Resource?.Id, subject, explanation, matchedManualEntryIds: matchedManualAllowIds);
        }

        blocklistMatch = await LoadMatchingBlocklistEntryAsync(request, now, firewallOnly: false, cancellationToken);
        if (blocklistMatch is not null && !manualAllowBypassesBlocking)
        {
            blocklistMatch.LastHitAtUtc = now;
            await db.SaveChangesAsync(cancellationToken);
            explanation.Add(new SecurityDecisionExplanation("blocklist", "matched", blocklistMatch.Reason));
            return Deny(
                SecurityDecisionActionNames.DenyBlocklist,
                "blocklist",
                context.Resource?.Id,
                subject,
                explanation,
                matchedBlocklistEntryIds: [blocklistMatch.Id],
                matchedState: BuildMatchedState(subjectEntity, stateEntity));
        }

        if (!manualAllowBypassesBlocking && IsSoftBlocked(stateEntity, subjectEntity, now))
        {
            explanation.Add(new SecurityDecisionExplanation("soft_block", "matched", "Active soft block state."));
            return Deny(
                SecurityDecisionActionNames.DenySoftBlock,
                "soft_block",
                context.Resource?.Id,
                subject,
                explanation,
                matchedState: BuildMatchedState(subjectEntity, stateEntity));
        }

        if (!manualAllowBypassesBlocking && IsRateLimited(stateEntity, subjectEntity, now))
        {
            explanation.Add(new SecurityDecisionExplanation("rate_limit", "matched", "Subject has exceeded request rate limits."));
            return RateLimited(
                "rate_limit_exceeded",
                context.Resource?.Id,
                subject,
                explanation,
                matchedState: BuildMatchedState(subjectEntity, stateEntity));
        }

        var abuseBucket = await db.AbuseBuckets.AsNoTracking()
            .SingleOrDefaultAsync(x => x.ClientIp == subject.NormalizedValue, cancellationToken);
        var abuseState = SecuritySubjectStateNames.Normalize(abuseBucket?.State);
        if (!manualAllowBypassesBlocking
            && abuseState is SecuritySubjectStateNames.FirewallBlocked
                or SecuritySubjectStateNames.SoftBlocked
                or SecuritySubjectStateNames.ManuallyBlocked)
        {
            explanation.Add(new SecurityDecisionExplanation("legacy_abuse_bucket", "matched", abuseState));
            return Deny(
                abuseState == SecuritySubjectStateNames.FirewallBlocked
                    ? SecurityDecisionActionNames.DenyFirewallBlocked
                    : SecurityDecisionActionNames.DenySoftBlock,
                abuseState,
                context.Resource?.Id,
                subject,
                explanation,
                matchedState: BuildMatchedState(subjectEntity, stateEntity));
        }

        if (IsChallengeRequired(stateEntity, subjectEntity, abuseState) && !manualAllowBypassesChallenge)
        {
            if (captcha is not null)
            {
                await captcha.CountProtectedHitWhileChallengedAsync(
                    subjectEntity,
                    stateEntity,
                    context.Resource?.Id,
                    stateEntity.ChallengeReason ?? abuseState,
                    cancellationToken);
            }
            else
            {
                stateEntity.RequestsWhileChallenged++;
                stateEntity.UpdatedAtUtc = now;
                await db.SaveChangesAsync(cancellationToken);
            }

            explanation.Add(new SecurityDecisionExplanation("challenge_state", "matched", stateEntity.ChallengeReason ?? abuseState));
            return Challenge(
                request,
                context,
                subject,
                "active_challenge",
                explanation,
                matchedState: BuildMatchedState(subjectEntity, stateEntity));
        }

        if (IsRateLimited(stateEntity, subjectEntity, now))
        {
            explanation.Add(new SecurityDecisionExplanation("rate_limit", "matched", "Subject has exceeded request rate limit."));
            return RateLimit(
                request,
                context,
                subject,
                "rate_limited",
                explanation,
                matchedState: BuildMatchedState(subjectEntity, stateEntity));
        }

        if (context.Resource is not null)
        {
            var effectiveGeoIp = geoIp ?? new NullGeoIpLookupService();
            var ruleResult = await EvaluateResourceRulesAsync(request, context, subject, explanation, effectiveGeoIp, cancellationToken);
            if (ruleResult is not null)
            {
                return ruleResult;
            }
        }

        var globalRuleResult = await EvaluateGlobalEdgeRulesAsync(request, context, subject, explanation, cancellationToken);
        if (globalRuleResult is not null)
        {
            return globalRuleResult;
        }

        if ((manualAllow is not null || legacyManualAllow) && context.Resource is null)
        {
            explanation.Add(new SecurityDecisionExplanation("default", "allow", "Manual allow has no SSO or CAPTCHA bypass by default."));
            return Allow(context.Resource?.Id, subject, explanation, matchedManualEntryIds: matchedManualAllowIds);
        }

        return DefaultResourceDecision(request, context, subject, explanation, matchedManualAllowIds);
    }

    private async Task<SecurityDecisionContext> BuildContextAsync(SecurityDecisionRequest request, CancellationToken cancellationToken)
    {
        var normalizedHost = NormalizeForwardedHost(request.Host);
        var rootDomain = (await db.AppSettings.AsNoTracking().SingleOrDefaultAsync(cancellationToken))?.RootDomain;
        var resources = await db.Resources.AsNoTracking()
            .Where(x => x.Enabled)
            .ToListAsync(cancellationToken);
        var resource = resources.FirstOrDefault(x => string.Equals(
            ResourceDomainResolver.Resolve(x.DomainMode, x.Domain, x.Slug, rootDomain),
            normalizedHost,
            StringComparison.OrdinalIgnoreCase));
        var hasOidcProvider = await db.OidcProviders.AsNoTracking().AnyAsync(x => x.Enabled, cancellationToken);
        var hasValidSession = await oidc.ValidateSessionAsync(request.EdgeSessionKey, cancellationToken);
        var captchaSettings = await db.CaptchaSettings.AsNoTracking().SingleOrDefaultAsync(cancellationToken);
        var isPublicChallengeResource = captchaSettings?.Enabled == true
            && resource is not null
            && captchaSettings.PublicChallengeResourceId == resource.Id;
        return new SecurityDecisionContext(resource, normalizedHost, hasOidcProvider, hasValidSession, rootDomain, isPublicChallengeResource);
    }

    private async Task<SecurityDecisionResult?> EvaluateResourceRulesAsync(
        SecurityDecisionRequest request,
        SecurityDecisionContext context,
        NormalizedSecuritySubject subject,
        List<SecurityDecisionExplanation> explanation,
        GeoIpLookupService geoIp,
        CancellationToken cancellationToken)
    {
        var resource = context.Resource ?? throw new InvalidOperationException("Resource rule evaluation requires a resolved resource.");
        var rules = await db.ResourceRules.AsNoTracking()
            .Where(x => x.ResourceId == resource.Id && x.Enabled)
            .OrderByDescending(x => x.Priority)
            .ToListAsync(cancellationToken);

        foreach (var rule in rules)
        {
            var ruleRequiresGeo = rule.MatchType is "country" or "region" or "asn";
            if (ruleRequiresGeo && !geoIp.IsAvailable)
            {
                explanation.Add(new SecurityDecisionExplanation("resource_rule_geoip_unavailable", "fail_closed", $"GeoIP rule '{rule.MatchType}:{rule.MatchValue}' cannot be evaluated — GeoIP database unavailable. Treating as deny (fail-closed)."));
                return Deny(
                    SecurityDecisionActionNames.DenyResourceRule,
                    "geoip_unavailable_fail_closed",
                    resource.Id,
                    subject,
                    explanation,
                    matchedResourceRuleIds: [rule.Id]);
            }

            if (!MatchesResourceRule(rule, request))
            {
                continue;
            }

            if (!SecurityResourceRuleActionNames.TryNormalize(rule.Action, out var action))
            {
                explanation.Add(new SecurityDecisionExplanation("resource_rule", "invalid_action", rule.Action));
                return Deny(
                    SecurityDecisionActionNames.DenyResourceRule,
                    "invalid_resource_rule_action",
                    resource.Id,
                    subject,
                    explanation,
                    matchedResourceRuleIds: [rule.Id]);
            }

            explanation.Add(new SecurityDecisionExplanation("resource_rule", "matched", $"{rule.MatchType}:{rule.MatchValue}:{action}"));
            if (action == SecurityResourceRuleActionNames.RequireChallenge && captcha is not null)
            {
                await captcha.MarkChallengeRequiredAsync(
                    request.ClientIp,
                    resource.Id,
                    "resource_rule_challenge",
                    cancellationToken);
            }

            return action switch
            {
                SecurityResourceRuleActionNames.Allow => Allow(resource.Id, subject, explanation, matchedResourceRuleIds: [rule.Id]),
                SecurityResourceRuleActionNames.Deny => Deny(SecurityDecisionActionNames.DenyResourceRule, "resource_rule", resource.Id, subject, explanation, matchedResourceRuleIds: [rule.Id]),
                SecurityResourceRuleActionNames.RequireSso => Sso(request, context, subject, explanation, [rule.Id]),
                SecurityResourceRuleActionNames.RequireChallenge => Challenge(request, context, subject, "resource_rule_challenge", explanation, matchedResourceRuleIds: [rule.Id]),
                SecurityResourceRuleActionNames.SoftBlock => Deny(SecurityDecisionActionNames.DenySoftBlock, "resource_rule_soft_block", resource.Id, subject, explanation, matchedResourceRuleIds: [rule.Id]),
                SecurityResourceRuleActionNames.FirewallBlock => Deny(SecurityDecisionActionNames.DenyFirewallBlocked, "resource_rule_firewall_block", resource.Id, subject, explanation, matchedResourceRuleIds: [rule.Id]),
                SecurityResourceRuleActionNames.BypassBlocking => null,
                _ => null,
            };
        }

        return null;
    }

    private async Task<SecurityDecisionResult?> EvaluateGlobalEdgeRulesAsync(
        SecurityDecisionRequest request,
        SecurityDecisionContext context,
        NormalizedSecuritySubject subject,
        List<SecurityDecisionExplanation> explanation,
        CancellationToken cancellationToken)
    {
        var rules = await db.EdgeAuthRules.AsNoTracking()
            .Where(x => x.Enabled)
            .OrderBy(x => x.Priority)
            .ToListAsync(cancellationToken);

        foreach (var rule in rules)
        {
            if (!Matches(rule.MatchJson, request))
            {
                continue;
            }

            explanation.Add(new SecurityDecisionExplanation("edge_auth_rule", "matched", rule.Action));
            return rule.Action.Trim().ToLowerInvariant() switch
            {
                "deny" or "block" => Deny(SecurityDecisionActionNames.DenyResourceRule, "edge_auth_rule", context.Resource?.Id, subject, explanation, matchedResourceRuleIds: [rule.Id]),
                "redirect" or "auth" or "require_sso" => Sso(request, context, subject, explanation, [rule.Id]),
                _ => Allow(context.Resource?.Id, subject, explanation, matchedResourceRuleIds: [rule.Id]),
            };
        }

        return null;
    }

    private SecurityDecisionResult DefaultResourceDecision(
        SecurityDecisionRequest request,
        SecurityDecisionContext context,
        NormalizedSecuritySubject subject,
        List<SecurityDecisionExplanation> explanation,
        IReadOnlyList<Guid> matchedManualEntryIds)
    {
        if (string.Equals(request.Mode, "observe", StringComparison.OrdinalIgnoreCase))
        {
            explanation.Add(new SecurityDecisionExplanation("mode", "allow", "Forward-auth observe mode."));
            return Allow(context.Resource?.Id, subject, explanation, matchedManualEntryIds);
        }

        var policy = context.Resource is null
            ? ForwardAuthPolicy.Adaptive
            : ForwardAuthPolicyMapping.Parse(context.Resource.ForwardAuthPolicy);
        if (string.Equals(request.Mode, "strict", StringComparison.OrdinalIgnoreCase))
        {
            policy = ForwardAuthPolicy.SsoRequired;
        }

        if (policy == ForwardAuthPolicy.Off || policy == ForwardAuthPolicy.Observe)
        {
            explanation.Add(new SecurityDecisionExplanation("resource_policy", "allow", policy.ToString()));
            return Allow(context.Resource?.Id, subject, explanation, matchedManualEntryIds);
        }

        if (context.HasValidSession)
        {
            explanation.Add(new SecurityDecisionExplanation("edge_session", "allow", "Valid edge SSO session."));
            return Allow(context.Resource?.Id, subject, explanation, matchedManualEntryIds);
        }

        if (policy == ForwardAuthPolicy.SsoRequired)
        {
            return Sso(request, context, subject, explanation, null, matchedManualEntryIds);
        }

        explanation.Add(new SecurityDecisionExplanation("resource_policy", "allow", "Adaptive policy has no active challenge or block."));
        return Allow(context.Resource?.Id, subject, explanation, matchedManualEntryIds);
    }

    private SecurityDecisionResult Sso(
        SecurityDecisionRequest request,
        SecurityDecisionContext context,
        NormalizedSecuritySubject subject,
        List<SecurityDecisionExplanation> explanation,
        IReadOnlyList<Guid>? matchedResourceRuleIds = null,
        IReadOnlyList<Guid>? matchedManualEntryIds = null)
    {
        if (context.HasValidSession)
        {
            explanation.Add(new SecurityDecisionExplanation("sso", "allow", "Valid edge SSO session."));
            return Allow(context.Resource?.Id, subject, explanation, matchedManualEntryIds, matchedResourceRuleIds);
        }

        if (!context.HasOidcProvider)
        {
            explanation.Add(new SecurityDecisionExplanation("sso", "deny", "No enabled OIDC provider is configured."));
            return Deny(
                SecurityDecisionActionNames.RequireSso,
                "sso_provider_missing",
                context.Resource?.Id,
                subject,
                explanation,
                matchedManualEntryIds,
                matchedResourceRuleIds: matchedResourceRuleIds);
        }

        explanation.Add(new SecurityDecisionExplanation("sso", "redirect", "SSO is required before upstream access."));
        return SecurityDecisionResult.Create(
            SecurityDecisionActionNames.RequireSso,
            SecurityDecisionResponseModeNames.Redirect,
            StatusCodes.Status302Found,
            BuildLoginUrl(request.Host, request.Path),
            "challenge",
            "sso_required",
            context.Resource?.Id,
            subject,
            explanation,
            matchedManualEntryIds,
            matchedResourceRuleIds: matchedResourceRuleIds);
    }

    private SecurityDecisionResult Challenge(
        SecurityDecisionRequest request,
        SecurityDecisionContext context,
        NormalizedSecuritySubject subject,
        string reason,
        List<SecurityDecisionExplanation> explanation,
        IReadOnlyList<Guid>? matchedResourceRuleIds = null,
        SecurityDecisionMatchedState? matchedState = null)
    {
        var browserLike = IsBrowserLike(request);
        explanation.Add(new SecurityDecisionExplanation("challenge", browserLike ? "redirect" : "api_challenge", "Challenge state blocks upstream access."));
        return SecurityDecisionResult.Create(
            SecurityDecisionActionNames.RequireChallenge,
            browserLike ? SecurityDecisionResponseModeNames.Redirect : SecurityDecisionResponseModeNames.ApiChallenge,
            browserLike ? StatusCodes.Status302Found : StatusCodes.Status403Forbidden,
            browserLike ? BuildChallengeUrl(request.Host, request.Path) : null,
            "challenge",
            reason,
            context.Resource?.Id,
            subject,
            explanation,
            matchedResourceRuleIds: matchedResourceRuleIds,
            matchedState: matchedState);
    }

    private static SecurityDecisionResult Allow(
        Guid? resourceId,
        NormalizedSecuritySubject subject,
        IReadOnlyList<SecurityDecisionExplanation> explanation,
        IReadOnlyList<Guid>? matchedManualEntryIds = null,
        IReadOnlyList<Guid>? matchedResourceRuleIds = null)
        => SecurityDecisionResult.Create(
            SecurityDecisionActionNames.AllowUpstream,
            SecurityDecisionResponseModeNames.Allow,
            StatusCodes.Status204NoContent,
            null,
            "allow",
            "allowed",
            resourceId,
            subject,
            explanation,
            matchedManualEntryIds,
            matchedResourceRuleIds: matchedResourceRuleIds);

    private static SecurityDecisionResult Deny(
        string action,
        string reason,
        Guid? resourceId,
        NormalizedSecuritySubject subject,
        IReadOnlyList<SecurityDecisionExplanation> explanation,
        IReadOnlyList<Guid>? matchedManualEntryIds = null,
        IReadOnlyList<Guid>? matchedBlocklistEntryIds = null,
        IReadOnlyList<Guid>? matchedResourceRuleIds = null,
        SecurityDecisionMatchedState? matchedState = null)
        => SecurityDecisionResult.Create(
            action,
            SecurityDecisionResponseModeNames.Deny,
            StatusCodes.Status403Forbidden,
            null,
            "deny",
            reason,
            resourceId,
            subject,
            explanation,
            matchedManualEntryIds,
            matchedBlocklistEntryIds,
            matchedResourceRuleIds,
            matchedState);

    private async Task<IReadOnlyList<ManualSecurityEntryEntity>> LoadMatchingManualEntriesAsync(
        SecurityDecisionRequest request,
        SecurityDecisionContext context,
        DateTimeOffset now,
        CancellationToken cancellationToken)
    {
        var entries = await db.ManualSecurityEntries
            .Where(x => x.Enabled)
            .Where(x => x.IsPermanent || x.ExpiresAtUtc == null || x.ExpiresAtUtc > now)
            .ToListAsync(cancellationToken);
        return entries
            .Where(x => MatchesManualEntry(x, request, context))
            .OrderBy(x => x.EntryType == ManualSecurityEntryTypeNames.Block ? 0 : 1)
            .ThenByDescending(x => x.CreatedAtUtc)
            .ToList();
    }

    private static bool MatchesManualEntry(
        ManualSecurityEntryEntity entry,
        SecurityDecisionRequest request,
        SecurityDecisionContext context)
    {
        if (!MatchesScope(entry.ScopeType, entry.ScopeId, context))
        {
            return false;
        }

        var normalizedValue = string.IsNullOrWhiteSpace(entry.NormalizedValue)
            && SecuritySubjectNormalizer.TryNormalize(entry.SubjectType, entry.SubjectValue, out var normalized)
                ? normalized.NormalizedValue
                : entry.NormalizedValue;
        return SecuritySubjectNormalizer.Matches(
            entry.SubjectType,
            normalizedValue,
            request.ClientIp,
            request.CountryCode,
            request.RegionCode,
            request.Asn);
    }

    private async Task<FirewallBlockSubjectEntity?> LoadMatchingFirewallSubjectBlockAsync(
        SecurityDecisionRequest request,
        CancellationToken cancellationToken)
    {
        var blocks = await db.FirewallBlockSubjects.AsNoTracking()
            .Where(x => x.Enabled)
            .ToListAsync(cancellationToken);
        return blocks.FirstOrDefault(x => SecuritySubjectNormalizer.Matches(
            NormalizeFirewallSubjectKind(x.SubjectKind),
            NormalizeSubjectValue(NormalizeFirewallSubjectKind(x.SubjectKind), x.SubjectValue),
            request.ClientIp,
            request.CountryCode,
            request.RegionCode,
            request.Asn));
    }

    private async Task<BlocklistEntryEntity?> LoadMatchingBlocklistEntryAsync(
        SecurityDecisionRequest request,
        DateTimeOffset now,
        bool firewallOnly,
        CancellationToken cancellationToken)
    {
        var query = db.BlocklistEntries
            .Where(x => x.Enabled)
            .Where(x => x.Source != BlocklistSourceNames.Manual)
            .Where(x => x.ExpiresAtUtc == null || x.ExpiresAtUtc > now);
        if (firewallOnly)
        {
            query = query.Where(x => x.SyncedToFirewall || x.EnforcementMode == BlocklistEnforcementModeNames.Firewall);
        }
        else
        {
            query = query.Where(x => x.EnforcementMode != BlocklistEnforcementModeNames.Observe);
        }

        var entries = await query.ToListAsync(cancellationToken);
        return entries.FirstOrDefault(x => SecuritySubjectNormalizer.Matches(
            NormalizeBlockType(x),
            NormalizeBlockValue(x),
            request.ClientIp,
            request.CountryCode,
            request.RegionCode,
            request.Asn));
    }

    private async Task<BlocklistEntryEntity?> LoadMatchingLegacyManualBlocklistAsync(
        SecurityDecisionRequest request,
        DateTimeOffset now,
        CancellationToken cancellationToken)
    {
        var entries = await db.BlocklistEntries
            .Where(x => x.Enabled)
            .Where(x => x.Source == BlocklistSourceNames.Manual)
            .Where(x => x.ExpiresAtUtc == null || x.ExpiresAtUtc > now)
            .ToListAsync(cancellationToken);
        return entries.FirstOrDefault(x => SecuritySubjectNormalizer.Matches(
            NormalizeBlockType(x),
            NormalizeBlockValue(x),
            request.ClientIp,
            request.CountryCode,
            request.RegionCode,
            request.Asn));
    }

    private async Task<bool> LoadMatchingLegacyManualAllowSubjectAsync(
        SecurityDecisionRequest request,
        CancellationToken cancellationToken)
    {
        var subjects = await db.FirewallAllowedSubjects.AsNoTracking()
            .Where(x => x.Enabled)
            .ToListAsync(cancellationToken);
        return subjects.Any(x => SecuritySubjectNormalizer.Matches(
            NormalizeFirewallSubjectKind(x.SubjectKind),
            NormalizeSubjectValue(NormalizeFirewallSubjectKind(x.SubjectKind), x.SubjectValue),
            request.ClientIp,
            request.CountryCode,
            request.RegionCode,
            request.Asn));
    }

    private static bool MatchesScope(string scopeType, string? scopeId, SecurityDecisionContext context)
    {
        var normalizedScope = scopeType.Trim().ToLowerInvariant();
        if (normalizedScope == ManualSecurityScopeTypeNames.Global)
        {
            return true;
        }

        if (normalizedScope == ManualSecurityScopeTypeNames.Resource)
        {
            return context.Resource is not null
                && string.Equals(scopeId, context.Resource.Id.ToString(), StringComparison.OrdinalIgnoreCase);
        }

        if (normalizedScope == ManualSecurityScopeTypeNames.RootDomain)
        {
            return !string.IsNullOrWhiteSpace(scopeId)
                && (string.Equals(scopeId, context.RootDomain, StringComparison.OrdinalIgnoreCase)
                    || context.NormalizedHost.EndsWith("." + scopeId.Trim().TrimStart('.'), StringComparison.OrdinalIgnoreCase));
        }

        return false;
    }

    private static bool IsFirewallBlocked(
        SecuritySubjectStateEntity state,
        SecuritySubjectEntity subject,
        DateTimeOffset now)
        => state.FirewallBlockedUntilUtc is null
            ? SecuritySubjectStateNames.Normalize(subject.CurrentState) == SecuritySubjectStateNames.FirewallBlocked
            : state.FirewallBlockedUntilUtc > now;

    private static bool IsSoftBlocked(
        SecuritySubjectStateEntity state,
        SecuritySubjectEntity subject,
        DateTimeOffset now)
        => state.SoftBlockedUntilUtc is null
            ? SecuritySubjectStateNames.Normalize(subject.CurrentState) == SecuritySubjectStateNames.SoftBlocked
            : state.SoftBlockedUntilUtc > now;

    private static bool IsChallengeRequired(SecuritySubjectStateEntity state, SecuritySubjectEntity subject, string legacyAbuseState)
        => state.ChallengeRequired
            || SecuritySubjectStateNames.Normalize(subject.CurrentState) == SecuritySubjectStateNames.Challenged
            || legacyAbuseState is SecuritySubjectStateNames.Suspect or SecuritySubjectStateNames.Challenged;

    private static bool IsRateLimited(SecuritySubjectStateEntity state, SecuritySubjectEntity subject, DateTimeOffset now)
        => state.RateLimitedUntilUtc is not null && state.RateLimitedUntilUtc > now;

    private SecurityDecisionResult RateLimited(
        string reason,
        Guid? resourceId,
        NormalizedSecuritySubject subject,
        List<SecurityDecisionExplanation> explanation,
        SecurityDecisionMatchedState? matchedState = null)
    {
        explanation.Add(new SecurityDecisionExplanation("rate_limit", "deny", "Subject exceeded request rate limits."));
        return SecurityDecisionResult.Create(
            SecurityDecisionActionNames.DenyRateLimited,
            SecurityDecisionResponseModeNames.RateLimited,
            StatusCodes.Status429TooManyRequests,
            null,
            "deny",
            reason,
            resourceId,
            subject,
            explanation,
            matchedState: matchedState);
    }

    private static bool IsRateLimited(SecuritySubjectStateEntity state, SecuritySubjectEntity subject, DateTimeOffset now)
    {
        if (state.RateLimitedUntilUtc is not null && state.RateLimitedUntilUtc > now)
        {
            return true;
        }

        return false;
    }

    private static SecurityDecisionResult RateLimit(
        SecurityDecisionRequest request,
        SecurityDecisionContext context,
        NormalizedSecuritySubject subject,
        string reason,
        List<SecurityDecisionExplanation> explanation,
        IReadOnlyList<Guid>? matchedResourceRuleIds = null,
        SecurityDecisionMatchedState? matchedState = null)
    {
        explanation.Add(new SecurityDecisionExplanation("rate_limit", "limited", "Request rate limit exceeded."));
        return SecurityDecisionResult.Create(
            SecurityDecisionActionNames.RateLimited,
            SecurityDecisionResponseModeNames.RateLimit,
            StatusCodes.Status429TooManyRequests,
            null,
            "deny",
            reason,
            context.Resource?.Id,
            subject,
            explanation,
            matchedResourceRuleIds: matchedResourceRuleIds,
            matchedState: matchedState);
    }

    private static SecurityDecisionMatchedState BuildMatchedState(
        SecuritySubjectEntity subject,
        SecuritySubjectStateEntity state)
        => new(
            subject.Id,
            state.Id,
            subject.CurrentState,
            state.ChallengeRequired,
            state.RequestsWhileChallenged,
            state.SoftBlockedUntilUtc,
            state.FirewallBlockedUntilUtc);

    private static string NormalizeForwardedHost(string host)
    {
        var normalized = host.Trim().ToLowerInvariant();
        var colonIndex = normalized.IndexOf(':', StringComparison.Ordinal);
        return colonIndex > 0
            ? normalized[..colonIndex]
            : normalized;
    }

    private static string BuildLoginUrl(string host, string path)
    {
        var returnUrl = Uri.EscapeDataString($"https://{NormalizeForwardedHost(host)}{EnsureLeadingSlash(path)}");
        return $"/api/edge-auth/login?returnUrl={returnUrl}";
    }

    private static string BuildChallengeUrl(string host, string path)
    {
        var returnUrl = Uri.EscapeDataString($"https://{NormalizeForwardedHost(host)}{EnsureLeadingSlash(path)}");
        return $"/api/edge-challenge/start?returnUrl={returnUrl}";
    }

    private static string EnsureLeadingSlash(string path)
        => string.IsNullOrWhiteSpace(path)
            ? "/"
            : path.StartsWith('/') ? path : "/" + path;

    private static bool IsBrowserLike(SecurityDecisionRequest request)
    {
        if (string.Equals(request.Method, "GET", StringComparison.OrdinalIgnoreCase)
            || string.Equals(request.Method, "HEAD", StringComparison.OrdinalIgnoreCase))
        {
            return string.IsNullOrWhiteSpace(request.AcceptHeader)
                || request.AcceptHeader.Contains("text/html", StringComparison.OrdinalIgnoreCase)
                || request.AcceptHeader.Contains("*/*", StringComparison.OrdinalIgnoreCase);
        }

        return false;
    }

    private static bool MatchesResourceRule(ResourceRuleEntity rule, SecurityDecisionRequest request)
        => rule.MatchType.ToLowerInvariant() switch
        {
            ResourceRuleMatchTypeNames.Ip => string.Equals(SecuritySubjectNormalizer.NormalizeIp(request.ClientIp).NormalizedValue, NormalizeSubjectValue(SecuritySubjectTypeNames.Ip, rule.MatchValue), StringComparison.OrdinalIgnoreCase),
            ResourceRuleMatchTypeNames.Cidr => SecuritySubjectNormalizer.IsInCidr(request.ClientIp, rule.MatchValue),
            ResourceRuleMatchTypeNames.Path => EnsureLeadingSlash(request.Path).StartsWith(EnsureLeadingSlash(rule.MatchValue), StringComparison.OrdinalIgnoreCase),
            ResourceRuleMatchTypeNames.Country => SecuritySubjectNormalizer.TryNormalize(SecuritySubjectTypeNames.Country, request.CountryCode, out var country)
                && string.Equals(country.NormalizedValue, NormalizeSubjectValue(SecuritySubjectTypeNames.Country, rule.MatchValue), StringComparison.OrdinalIgnoreCase),
            ResourceRuleMatchTypeNames.Region => SecuritySubjectNormalizer.TryNormalize(SecuritySubjectTypeNames.Region, request.RegionCode, out var region)
                && string.Equals(region.NormalizedValue, NormalizeSubjectValue(SecuritySubjectTypeNames.Region, rule.MatchValue), StringComparison.OrdinalIgnoreCase),
            ResourceRuleMatchTypeNames.Asn => SecuritySubjectNormalizer.TryNormalize(SecuritySubjectTypeNames.Asn, request.Asn, out var asn)
                && string.Equals(asn.NormalizedValue, NormalizeSubjectValue(SecuritySubjectTypeNames.Asn, rule.MatchValue), StringComparison.OrdinalIgnoreCase),
            _ => false,
        };

    private static bool Matches(string matchJson, SecurityDecisionRequest request)
    {
        using var doc = JsonDocument.Parse(matchJson);
        var root = doc.RootElement;
        if (root.TryGetProperty("host", out var hostMatch)
            && !request.Host.Contains(hostMatch.GetString() ?? string.Empty, StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        if (root.TryGetProperty("pathPrefix", out var pathMatch)
            && !EnsureLeadingSlash(request.Path).StartsWith(EnsureLeadingSlash(pathMatch.GetString() ?? "/"), StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        if (root.TryGetProperty("cidr", out var cidrMatch)
            && !SecuritySubjectNormalizer.IsInCidr(request.ClientIp, cidrMatch.GetString() ?? string.Empty))
        {
            return false;
        }

        if (root.TryGetProperty("country", out var countryMatch)
            && !string.Equals(request.CountryCode, countryMatch.GetString(), StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        if (root.TryGetProperty("region", out var regionMatch)
            && !string.Equals(request.RegionCode, regionMatch.GetString(), StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        if (root.TryGetProperty("asn", out var asnMatch)
            && !string.Equals(request.Asn, asnMatch.GetString(), StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        return true;
    }

    private static string NormalizeBlockType(BlocklistEntryEntity entry)
    {
        if (!string.IsNullOrWhiteSpace(entry.Type))
        {
            return entry.Type.Trim().ToLowerInvariant() switch
            {
                BlocklistTypeNames.Ip => SecuritySubjectTypeNames.Ip,
                BlocklistTypeNames.Asn => SecuritySubjectTypeNames.Asn,
                BlocklistTypeNames.Country => SecuritySubjectTypeNames.Country,
                BlocklistTypeNames.Region => SecuritySubjectTypeNames.Region,
                var normalized => normalized,
            };
        }

        return string.IsNullOrWhiteSpace(entry.SubjectType)
            ? SecuritySubjectTypeNames.Ip
            : entry.SubjectType.Trim().ToLowerInvariant();
    }

    private static string NormalizeBlockValue(BlocklistEntryEntity entry)
    {
        var type = NormalizeBlockType(entry);
        if (!string.IsNullOrWhiteSpace(entry.NormalizedValue))
        {
            return entry.NormalizedValue;
        }

        var value = !string.IsNullOrWhiteSpace(entry.Value) ? entry.Value : entry.ClientIp;
        return NormalizeSubjectValue(type, value);
    }

    private static string NormalizeFirewallSubjectKind(string subjectKind)
        => subjectKind.Trim().ToLowerInvariant() switch
        {
            FirewallSubjectKindNames.Ip => SecuritySubjectTypeNames.Ip,
            FirewallSubjectKindNames.Cidr => SecuritySubjectTypeNames.Cidr,
            FirewallSubjectKindNames.Country => SecuritySubjectTypeNames.Country,
            FirewallSubjectKindNames.Asn => SecuritySubjectTypeNames.Asn,
            _ => subjectKind.Trim().ToLowerInvariant(),
        };

    private static string NormalizeSubjectValue(string subjectType, string value)
        => SecuritySubjectNormalizer.TryNormalize(subjectType, value, out var subject)
            ? subject.NormalizedValue
            : value.Trim();
}

public static class SecurityResourceRuleActionNames
{
    public const string Allow = "allow";
    public const string Deny = "deny";
    public const string RequireSso = "require_sso";
    public const string RequireChallenge = "require_challenge";
    public const string SoftBlock = "soft_block";
    public const string FirewallBlock = "firewall_block";
    public const string BypassBlocking = "bypass_blocking";

    private static readonly IReadOnlyDictionary<string, string> Aliases = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
    {
        [Allow] = Allow,
        ["bypass_auth"] = Allow,
        ["bypass"] = Allow,
        [Deny] = Deny,
        ["block"] = Deny,
        ["block_access"] = Deny,
        [RequireSso] = RequireSso,
        ["auth"] = RequireSso,
        ["pass_to_auth"] = RequireSso,
        ["require_auth"] = RequireSso,
        [RequireChallenge] = RequireChallenge,
        ["challenge"] = RequireChallenge,
        ["adaptive_challenge"] = RequireChallenge,
        ["require_adaptive_challenge"] = RequireChallenge,
        [SoftBlock] = SoftBlock,
        ["soft-block"] = SoftBlock,
        [FirewallBlock] = FirewallBlock,
        ["firewall-block"] = FirewallBlock,
        [BypassBlocking] = BypassBlocking,
    };

    public static bool TryNormalize(string? action, out string normalized)
    {
        if (!string.IsNullOrWhiteSpace(action)
            && Aliases.TryGetValue(action.Trim().ToLowerInvariant(), out var value))
        {
            normalized = value;
            return true;
        }

        normalized = string.Empty;
        return false;
    }

    public static string Normalize(string? action)
        => TryNormalize(action, out var normalized)
            ? normalized
            : throw new InvalidOperationException($"Resource rule action must be one of: {string.Join(", ", Aliases.Keys.Distinct(StringComparer.OrdinalIgnoreCase).OrderBy(x => x))}.");
}

internal sealed class NullGeoIpLookupService : GeoIpLookupService
{
    public NullGeoIpLookupService() : base(new NullConfiguration(), new NullLogger<GeoIpLookupService>())
    {
    }

    private sealed class NullConfiguration : IConfiguration
    {
        public string? this[string key] => null;
        public IEnumerable<IConfigurationSection> GetChildren() => [];
        public IConfigurationSection GetSection(string key) => new NullConfigurationSection();
    }

    private sealed class NullConfigurationSection : IConfigurationSection
    {
        public string Key => string.Empty;
        public string Path => string.Empty;
        public string? Value { get => null; set { } }
        public string? Get(string key) => null;
        public IEnumerable<IConfigurationSection> GetChildren() => [];
        public IConfigurationSection GetSection(string key) => new NullConfigurationSection();
    }
}

internal sealed class NullLogger<T> : ILogger<T> where T : class
{
    public IDisposable? BeginScope<TState>(TState state) where TState : notnull => null;
    public bool IsEnabled(LogLevel logLevel) => false;
    public void Log<TState>(LogLevel logLevel, EventId eventId, TState state, Exception? exception, Func<TState, Exception?, string> formatter)
    {
    }
}
