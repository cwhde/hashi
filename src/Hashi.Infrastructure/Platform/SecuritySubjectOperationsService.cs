using System.Net;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class SecuritySubjectOperationsService(
    HashiDbContext db,
    SecurityDecisionService decisions,
    FirewallApplyService firewall,
    AuditService audit,
    TimeProvider? timeProvider = null)
{
    public async Task<SecuritySubjectSearchResponse> SearchAsync(string? query, CancellationToken cancellationToken = default)
    {
        var q = query?.Trim() ?? string.Empty;
        var normalized = TryNormalizeSearch(q);
        var subjects = await db.SecuritySubjects.AsNoTracking()
            .OrderByDescending(x => x.LastSeenAtUtc)
            .Take(string.IsNullOrWhiteSpace(q) ? 25 : 200)
            .ToListAsync(cancellationToken);

        var results = subjects
            .Where(x => MatchesSubjectSearch(x, q, normalized))
            .Take(25)
            .Select(ToSubjectSummary)
            .ToList();

        if (!string.IsNullOrWhiteSpace(q) && results.Count < 25)
        {
            var eventMatches = await db.SecurityEvents.AsNoTracking()
                .Where(x =>
                    (x.NormalizedSubjectValue != null && x.NormalizedSubjectValue.Contains(q)) ||
                    (x.SubjectValue != null && x.SubjectValue.Contains(q)) ||
                    (x.ClientIp != null && x.ClientIp.Contains(q)) ||
                    (x.Host != null && x.Host.Contains(q)) ||
                    (x.Path != null && x.Path.Contains(q)) ||
                    (x.Reason != null && x.Reason.Contains(q)))
                .OrderByDescending(x => x.OccurredAtUtc)
                .Take(50)
                .ToListAsync(cancellationToken);

            foreach (var eventMatch in eventMatches)
            {
                var type = eventMatch.SubjectType ?? SecuritySubjectTypeNames.Ip;
                var value = eventMatch.NormalizedSubjectValue ?? eventMatch.SubjectValue ?? eventMatch.ClientIp;
                if (string.IsNullOrWhiteSpace(value)
                    || !SecuritySubjectNormalizer.TryNormalize(type, value, out var eventSubject))
                {
                    continue;
                }

                var subject = await EnsureSubjectAsync(eventSubject.SubjectType, eventSubject.SubjectValue, cancellationToken);
                if (results.All(x => x.Id != subject.Id))
                {
                    results.Add(ToSubjectSummary(subject));
                }

                if (results.Count >= 25)
                {
                    break;
                }
            }
        }

        return new SecuritySubjectSearchResponse(q, results);
    }

    public async Task<SecuritySubjectDetailResponse?> GetDetailAsync(Guid id, CancellationToken cancellationToken = default)
    {
        var subject = await db.SecuritySubjects.AsNoTracking().SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        if (subject is null)
        {
            return null;
        }

        var state = await db.SecuritySubjectStates.AsNoTracking()
            .SingleOrDefaultAsync(x => x.SecuritySubjectId == id, cancellationToken);
        var manualEntries = await MatchingManualEntriesAsync(subject, cancellationToken);
        var blocklistEntries = await MatchingBlocklistEntriesAsync(subject, cancellationToken);
        var resourceRules = await MatchingResourceRulesAsync(subject, cancellationToken);
        var firewallApplications = await MatchingFirewallApplicationsAsync(subject, blocklistEntries, cancellationToken);

        return new SecuritySubjectDetailResponse(
            ToSubjectSummary(subject),
            state is null ? null : ToStateResponse(state),
            manualEntries.Select(ToManualResponse).ToList(),
            blocklistEntries.Select(BlocklistSourceManagementService.ToEntryResponse).ToList(),
            resourceRules.Select(ResourceService.ToRuleResponse).ToList(),
            firewallApplications);
    }

    public async Task<IReadOnlyList<SecurityEventResponse>> ListEventsAsync(
        Guid id,
        string? eventType,
        Guid? resourceId,
        int limit = 100,
        CancellationToken cancellationToken = default)
    {
        var subject = await db.SecuritySubjects.AsNoTracking().SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        if (subject is null)
        {
            return [];
        }

        var normalized = subject.NormalizedValue;
        var query = db.SecurityEvents.AsNoTracking()
            .Where(x => x.NormalizedSubjectValue == normalized || x.ClientIp == normalized);
        if (!string.IsNullOrWhiteSpace(eventType))
        {
            query = query.Where(x => x.EventType == eventType || x.Category == eventType);
        }

        if (resourceId is Guid rid)
        {
            query = query.Where(x => x.ResourceId == rid);
        }

        return await query
            .OrderByDescending(x => x.OccurredAtUtc)
            .Take(Math.Clamp(limit, 1, 250))
            .Select(x => ToEventResponse(x))
            .ToListAsync(cancellationToken);
    }

    public async Task<IReadOnlyList<SecurityRequestBucketResponse>> ListBucketsAsync(
        Guid id,
        int hours = 24,
        CancellationToken cancellationToken = default)
    {
        var subject = await db.SecuritySubjects.AsNoTracking().SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        if (subject is null)
        {
            return [];
        }

        var since = Now().AddHours(-Math.Clamp(hours, 1, 720));
        return await db.SecurityRequestBuckets.AsNoTracking()
            .Where(x => x.NormalizedSubjectValue == subject.NormalizedValue || x.ClientIp == subject.NormalizedValue)
            .Where(x => x.BucketStartUtc >= since)
            .OrderByDescending(x => x.BucketStartUtc)
            .Take(240)
            .Select(x => ToBucketResponse(x))
            .ToListAsync(cancellationToken);
    }

    public async Task<SecurityEffectiveDecisionResponse?> GetEffectiveDecisionAsync(Guid id, CancellationToken cancellationToken = default)
    {
        var subject = await db.SecuritySubjects.AsNoTracking().SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        if (subject is null)
        {
            return null;
        }

        if (!IPAddress.TryParse(subject.NormalizedValue, out var ip))
        {
            var synthetic = await ExplainNonIpDecisionAsync(subject, cancellationToken);
            return synthetic;
        }

        var result = await decisions.DecideForwardAuthAsync(new SecurityDecisionRequest(
            "subject-preview.hashi.local",
            "/",
            ip,
            subject.LastCountry,
            subject.LastRegion,
            subject.LastAsn,
            TrustedForwardedContext: true,
            Method: "GET",
            AcceptHeader: "application/json"), cancellationToken);

        var state = await db.SecuritySubjectStates.AsNoTracking()
            .SingleOrDefaultAsync(x => x.SecuritySubjectId == subject.Id, cancellationToken);
        return new SecurityEffectiveDecisionResponse(
            subject.Id,
            result.Decision,
            result.Action,
            result.Reason,
            result.Explanation.Select(x => $"{x.Step}: {x.Outcome} - {x.Details}").ToList(),
            result.MatchedManualEntryIds,
            result.MatchedBlocklistEntryIds,
            result.MatchedResourceRuleIds,
            state is null ? null : ToStateResponse(state));
    }

    public async Task<ManualSecurityEntryResponse> CreateManualEntryAsync(
        UpsertManualSecurityEntryRequest request,
        CancellationToken cancellationToken = default)
    {
        var normalized = NormalizeSubject(request.SubjectType, request.SubjectValue);
        var entryType = NormalizeEntryType(request.EntryType);
        var now = Now();
        var entity = new ManualSecurityEntryEntity
        {
            SubjectType = normalized.SubjectType,
            SubjectValue = normalized.SubjectValue,
            NormalizedValue = normalized.NormalizedValue,
            EntryType = entryType,
            ScopeType = NormalizeScopeType(request.ScopeType),
            ScopeId = NormalizeScopeId(request.ScopeId),
            Reason = NormalizeReason(request.Reason),
            CreatedAtUtc = now,
            ExpiresAtUtc = request.ExpiresAtUtc,
            IsPermanent = request.IsPermanent ?? request.ExpiresAtUtc is null,
            Enabled = request.Enabled ?? true,
        };
        ApplyBypassFlags(entity, request.BypassBlocking, request.BypassAdaptiveEscalation, request.BypassRateLimit, request.BypassChallenge, request.BypassSso);

        db.ManualSecurityEntries.Add(entity);
        await UpsertSubjectStateAsync(entity.SubjectType, entity.SubjectValue, entryType, entity.ExpiresAtUtc, cancellationToken);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("security", $"manual_{entryType}_created", subjectType: entity.SubjectType, subjectId: entity.NormalizedValue, metadata: new { entity.ScopeType, entity.ScopeId }, cancellationToken: cancellationToken);
        await WriteSecurityEventAsync(entity, $"manual_{entryType}_created", "info", entryType, cancellationToken);
        return ToManualResponse(entity);
    }

    public async Task<ManualSecurityEntryResponse?> UpdateManualEntryAsync(
        Guid id,
        UpdateManualSecurityEntryRequest request,
        CancellationToken cancellationToken = default)
    {
        var entity = await db.ManualSecurityEntries.SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        if (entity is null)
        {
            return null;
        }

        if (request.Reason is not null)
        {
            entity.Reason = NormalizeReason(request.Reason);
        }

        if (request.ExpiresAtUtc is not null || request.IsPermanent == false)
        {
            entity.ExpiresAtUtc = request.ExpiresAtUtc;
        }

        if (request.IsPermanent is bool permanent)
        {
            entity.IsPermanent = permanent;
            if (permanent)
            {
                entity.ExpiresAtUtc = null;
            }
        }

        if (request.Enabled is bool enabled)
        {
            entity.Enabled = enabled;
        }

        ApplyBypassFlags(entity, request.BypassBlocking, request.BypassAdaptiveEscalation, request.BypassRateLimit, request.BypassChallenge, request.BypassSso);
        await UpsertSubjectStateAsync(entity.SubjectType, entity.SubjectValue, entity.EntryType, entity.ExpiresAtUtc, cancellationToken);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("security", $"manual_{entity.EntryType}_updated", subjectType: entity.SubjectType, subjectId: entity.NormalizedValue, cancellationToken: cancellationToken);
        await WriteSecurityEventAsync(entity, $"manual_{entity.EntryType}_updated", "info", entity.EntryType, cancellationToken);
        return ToManualResponse(entity);
    }

    public async Task<bool> DeleteManualEntryAsync(Guid id, CancellationToken cancellationToken = default)
    {
        var entity = await db.ManualSecurityEntries.SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        if (entity is null)
        {
            return false;
        }

        db.ManualSecurityEntries.Remove(entity);
        await db.SaveChangesAsync(cancellationToken);
        await RefreshSubjectStateFlagsAsync(entity.SubjectType, entity.NormalizedValue, cancellationToken);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("security", $"manual_{entity.EntryType}_deleted", subjectType: entity.SubjectType, subjectId: entity.NormalizedValue, cancellationToken: cancellationToken);
        await WriteSecurityEventAsync(entity, $"manual_{entity.EntryType}_deleted", "info", entity.EntryType, cancellationToken);
        return true;
    }

    public async Task<ManualSecurityEntryResponse?> ExpireManualEntryAsync(Guid id, CancellationToken cancellationToken = default)
    {
        var entity = await db.ManualSecurityEntries.SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        if (entity is null)
        {
            return null;
        }

        entity.Enabled = false;
        entity.IsPermanent = false;
        entity.ExpiresAtUtc = Now();
        await db.SaveChangesAsync(cancellationToken);
        await RefreshSubjectStateFlagsAsync(entity.SubjectType, entity.NormalizedValue, cancellationToken);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("security", $"manual_{entity.EntryType}_expired", subjectType: entity.SubjectType, subjectId: entity.NormalizedValue, cancellationToken: cancellationToken);
        await WriteSecurityEventAsync(entity, $"manual_{entity.EntryType}_expired", "info", entity.EntryType, cancellationToken);
        return ToManualResponse(entity);
    }

    public async Task<SecurityBlockMutationResponse> CreateBlockAsync(
        CreateSecurityBlockRequest request,
        CancellationToken cancellationToken = default)
    {
        var blockType = NormalizeBlockType(request.BlockType);
        var entry = await CreateManualEntryAsync(new UpsertManualSecurityEntryRequest(
            request.SubjectType,
            request.SubjectValue,
            ManualSecurityEntryTypeNames.Block,
            ManualSecurityScopeTypeNames.Global,
            null,
            request.Reason,
            request.ExpiresAtUtc,
            request.IsPermanent ?? request.ExpiresAtUtc is null,
            false,
            false,
            false,
            false,
            false,
            true), cancellationToken);

        await ApplyBlockStateAsync(entry.SubjectType, entry.SubjectValue, blockType, entry.ExpiresAtUtc, request.FirewallEnforced, cancellationToken);
        var state = await StateForSubjectValueAsync(entry.SubjectType, entry.NormalizedValue, cancellationToken);
        FirewallPlanPreviewResponse? preview = null;
        var firewallRecommended = request.FirewallEnforced && IsFirewallEligible(entry.SubjectType);
        if (firewallRecommended)
        {
            preview = await PreviewAnyFirewallPlanAsync(cancellationToken);
        }

        return new SecurityBlockMutationResponse(entry, state, firewallRecommended, preview);
    }

    public async Task<SecurityBlockMutationResponse?> UpdateBlockAsync(
        Guid id,
        UpdateSecurityBlockRequest request,
        CancellationToken cancellationToken = default)
    {
        var entry = await UpdateManualEntryAsync(id, new UpdateManualSecurityEntryRequest(
            request.Reason,
            request.ExpiresAtUtc,
            request.IsPermanent,
            false,
            false,
            false,
            false,
            false,
            request.Enabled), cancellationToken);
        if (entry is null)
        {
            return null;
        }

        if (request.FirewallEnforced is bool firewallEnforced)
        {
            await ApplyBlockStateAsync(entry.SubjectType, entry.SubjectValue, firewallEnforced ? "firewall" : "soft", entry.ExpiresAtUtc, firewallEnforced, cancellationToken);
        }

        var state = await StateForSubjectValueAsync(entry.SubjectType, entry.NormalizedValue, cancellationToken);
        return new SecurityBlockMutationResponse(entry, state, request.FirewallEnforced == true, request.FirewallEnforced == true ? await PreviewAnyFirewallPlanAsync(cancellationToken) : null);
    }

    public async Task<SecurityBlockMutationResponse?> ExtendBlockAsync(Guid id, int durationSeconds, CancellationToken cancellationToken = default)
    {
        var entity = await db.ManualSecurityEntries.SingleOrDefaultAsync(x => x.Id == id && x.EntryType == ManualSecurityEntryTypeNames.Block, cancellationToken);
        if (entity is null)
        {
            return null;
        }

        entity.IsPermanent = false;
        var basis = entity.ExpiresAtUtc is { } expiry && expiry > Now() ? expiry : Now();
        entity.ExpiresAtUtc = basis.AddSeconds(Math.Clamp(durationSeconds, 60, 31536000));
        await UpsertSubjectStateAsync(entity.SubjectType, entity.SubjectValue, entity.EntryType, entity.ExpiresAtUtc, cancellationToken);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("security", "manual_block_extended", subjectType: entity.SubjectType, subjectId: entity.NormalizedValue, cancellationToken: cancellationToken);
        await WriteSecurityEventAsync(entity, "manual_block_extended", "info", "block", cancellationToken);
        return new SecurityBlockMutationResponse(ToManualResponse(entity), await StateForSubjectValueAsync(entity.SubjectType, entity.NormalizedValue, cancellationToken), false, null);
    }

    public async Task<SecurityBlockMutationResponse?> ShortenBlockAsync(Guid id, int durationSeconds, CancellationToken cancellationToken = default)
    {
        var entity = await db.ManualSecurityEntries.SingleOrDefaultAsync(x => x.Id == id && x.EntryType == ManualSecurityEntryTypeNames.Block, cancellationToken);
        if (entity is null)
        {
            return null;
        }

        entity.IsPermanent = false;
        entity.ExpiresAtUtc = Now().AddSeconds(Math.Clamp(durationSeconds, 60, 31536000));
        await UpsertSubjectStateAsync(entity.SubjectType, entity.SubjectValue, entity.EntryType, entity.ExpiresAtUtc, cancellationToken);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("security", "manual_block_shortened", subjectType: entity.SubjectType, subjectId: entity.NormalizedValue, cancellationToken: cancellationToken);
        await WriteSecurityEventAsync(entity, "manual_block_shortened", "info", "block", cancellationToken);
        return new SecurityBlockMutationResponse(ToManualResponse(entity), await StateForSubjectValueAsync(entity.SubjectType, entity.NormalizedValue, cancellationToken), false, null);
    }

    public async Task<SecurityBlockMutationResponse?> MakePermanentAsync(Guid id, CancellationToken cancellationToken = default)
    {
        var entity = await db.ManualSecurityEntries.SingleOrDefaultAsync(x => x.Id == id && x.EntryType == ManualSecurityEntryTypeNames.Block, cancellationToken);
        if (entity is null)
        {
            return null;
        }

        entity.IsPermanent = true;
        entity.ExpiresAtUtc = null;
        await UpsertSubjectStateAsync(entity.SubjectType, entity.SubjectValue, entity.EntryType, null, cancellationToken);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("security", "manual_block_made_permanent", subjectType: entity.SubjectType, subjectId: entity.NormalizedValue, cancellationToken: cancellationToken);
        await WriteSecurityEventAsync(entity, "manual_block_made_permanent", "warning", "block", cancellationToken);
        return new SecurityBlockMutationResponse(ToManualResponse(entity), await StateForSubjectValueAsync(entity.SubjectType, entity.NormalizedValue, cancellationToken), false, null);
    }

    public async Task<SecurityBlockMutationResponse?> ExpireBlockAsync(Guid id, CancellationToken cancellationToken = default)
    {
        var entry = await ExpireManualEntryAsync(id, cancellationToken);
        if (entry is null)
        {
            return null;
        }

        await ClearBlockStateAsync(entry.SubjectType, entry.NormalizedValue, cancellationToken);
        return new SecurityBlockMutationResponse(entry, await StateForSubjectValueAsync(entry.SubjectType, entry.NormalizedValue, cancellationToken), IsFirewallEligible(entry.SubjectType), null);
    }

    public async Task<FirewallPlanPreviewResponse?> PreviewFirewallSyncAsync(Guid id, CancellationToken cancellationToken = default)
    {
        var entity = await db.ManualSecurityEntries.AsNoTracking()
            .SingleOrDefaultAsync(x => x.Id == id && x.EntryType == ManualSecurityEntryTypeNames.Block, cancellationToken);
        if (entity is null)
        {
            return null;
        }

        if (!IsFirewallEligible(entity.SubjectType))
        {
            throw new InvalidOperationException("Only IP and CIDR subjects can be firewall enforced.");
        }

        return await PreviewAnyFirewallPlanAsync(cancellationToken);
    }

    private async Task<SecurityEffectiveDecisionResponse> ExplainNonIpDecisionAsync(
        SecuritySubjectEntity subject,
        CancellationToken cancellationToken)
    {
        var state = await db.SecuritySubjectStates.AsNoTracking()
            .SingleOrDefaultAsync(x => x.SecuritySubjectId == subject.Id, cancellationToken);
        var manualEntries = await MatchingManualEntriesAsync(subject, cancellationToken);
        var blocklistEntries = await MatchingBlocklistEntriesAsync(subject, cancellationToken);
        var block = manualEntries.FirstOrDefault(x => x.EntryType == ManualSecurityEntryTypeNames.Block);
        var allow = manualEntries.FirstOrDefault(x => x.EntryType == ManualSecurityEntryTypeNames.Allow);
        var decision = block is not null || blocklistEntries.Any(x => x.Enabled) ? "deny" : "allow";
        var action = block is not null
            ? SecurityDecisionActionNames.DenyManualBlock
            : blocklistEntries.Any(x => x.Enabled)
                ? SecurityDecisionActionNames.DenyBlocklist
                : SecurityDecisionActionNames.AllowUpstream;
        var reason = block is not null
            ? "manual_block"
            : blocklistEntries.Any(x => x.Enabled)
                ? "blocklist"
                : allow is not null ? "manual_allow" : "observed";
        return new SecurityEffectiveDecisionResponse(
            subject.Id,
            decision,
            action,
            reason,
            [$"subject: resolved - {subject.SubjectType}:{subject.NormalizedValue}", $"{reason}: inferred - Non-IP subjects are evaluated in middleware/resource context."],
            manualEntries.Select(x => x.Id).ToList(),
            blocklistEntries.Select(x => x.Id).ToList(),
            [],
            state is null ? null : ToStateResponse(state));
    }

    private async Task<IReadOnlyList<ManualSecurityEntryEntity>> MatchingManualEntriesAsync(SecuritySubjectEntity subject, CancellationToken cancellationToken)
    {
        var now = Now();
        var entries = await db.ManualSecurityEntries.AsNoTracking()
            .Where(x => x.Enabled)
            .Where(x => x.IsPermanent || x.ExpiresAtUtc == null || x.ExpiresAtUtc > now)
            .ToListAsync(cancellationToken);
        return entries.Where(x => SubjectValuesOverlap(x.SubjectType, x.NormalizedValue, subject)).ToList();
    }

    private async Task<IReadOnlyList<BlocklistEntryEntity>> MatchingBlocklistEntriesAsync(SecuritySubjectEntity subject, CancellationToken cancellationToken)
    {
        var now = Now();
        var entries = await db.BlocklistEntries.AsNoTracking()
            .Where(x => x.Enabled)
            .Where(x => x.ExpiresAtUtc == null || x.ExpiresAtUtc > now)
            .ToListAsync(cancellationToken);
        return entries.Where(x => SubjectValuesOverlap(NormalizeBlockType(x), NormalizeBlockValue(x), subject)).ToList();
    }

    private async Task<IReadOnlyList<ResourceRuleEntity>> MatchingResourceRulesAsync(SecuritySubjectEntity subject, CancellationToken cancellationToken)
    {
        var rules = await db.ResourceRules.AsNoTracking()
            .Where(x => x.Enabled)
            .OrderByDescending(x => x.Priority)
            .ToListAsync(cancellationToken);
        return rules.Where(x => SubjectValuesOverlap(NormalizeRuleType(x.MatchType), NormalizeSubjectValue(NormalizeRuleType(x.MatchType), x.MatchValue), subject)).ToList();
    }

    private async Task<IReadOnlyList<SecurityFirewallApplicationResponse>> MatchingFirewallApplicationsAsync(
        SecuritySubjectEntity subject,
        IReadOnlyList<BlocklistEntryEntity> blocklistEntries,
        CancellationToken cancellationToken)
    {
        var blocklistIds = blocklistEntries.Select(x => x.Id).ToHashSet();
        var applied = await db.BlocklistAppliedHosts.AsNoTracking()
            .Where(x => blocklistIds.Contains(x.BlocklistEntryId))
            .Join(db.FirewallHosts.AsNoTracking(), x => x.FirewallHostId, x => x.Id, (apply, host) => new SecurityFirewallApplicationResponse(
                host.Id,
                host.Name,
                "blocklist",
                apply.Status,
                apply.AppliedAtUtc,
                apply.LastError))
            .ToListAsync(cancellationToken);

        var firewallSubjects = await db.FirewallBlockSubjects.AsNoTracking()
            .Where(x => x.Enabled)
            .Join(db.FirewallHosts.AsNoTracking(), x => x.FirewallHostId, x => x.Id, (block, host) => new { block, host })
            .ToListAsync(cancellationToken);

        applied.AddRange(firewallSubjects
            .Where(x => SubjectValuesOverlap(NormalizeFirewallSubjectKind(x.block.SubjectKind), NormalizeSubjectValue(NormalizeFirewallSubjectKind(x.block.SubjectKind), x.block.SubjectValue), subject))
            .Select(x => new SecurityFirewallApplicationResponse(x.host.Id, x.host.Name, "manual_firewall_subject", "enabled", null, null)));

        return applied;
    }

    private async Task<SecuritySubjectEntity> EnsureSubjectAsync(string subjectType, string subjectValue, CancellationToken cancellationToken)
    {
        var normalized = NormalizeSubject(subjectType, subjectValue);
        var subject = await db.SecuritySubjects.SingleOrDefaultAsync(
            x => x.SubjectType == normalized.SubjectType && x.NormalizedValue == normalized.NormalizedValue,
            cancellationToken);
        if (subject is not null)
        {
            subject.LastSeenAtUtc = Now();
            await db.SaveChangesAsync(cancellationToken);
            return subject;
        }

        subject = new SecuritySubjectEntity
        {
            SubjectType = normalized.SubjectType,
            SubjectValue = normalized.SubjectValue,
            NormalizedValue = normalized.NormalizedValue,
            FirstSeenAtUtc = Now(),
            LastSeenAtUtc = Now(),
        };
        db.SecuritySubjects.Add(subject);
        await db.SaveChangesAsync(cancellationToken);
        return subject;
    }

    private async Task UpsertSubjectStateAsync(
        string subjectType,
        string subjectValue,
        string entryType,
        DateTimeOffset? expiresAtUtc,
        CancellationToken cancellationToken)
    {
        var subject = await EnsureSubjectAsync(subjectType, subjectValue, cancellationToken);
        var state = await db.SecuritySubjectStates.SingleOrDefaultAsync(x => x.SecuritySubjectId == subject.Id, cancellationToken);
        if (state is null)
        {
            state = new SecuritySubjectStateEntity { SecuritySubjectId = subject.Id };
            db.SecuritySubjectStates.Add(state);
        }

        if (entryType == ManualSecurityEntryTypeNames.Allow)
        {
            state.ManualAllowActive = true;
            subject.CurrentState = SecuritySubjectStateNames.ManuallyAllowed;
        }
        else
        {
            state.ManualBlockActive = true;
            state.SoftBlockedUntilUtc = expiresAtUtc;
            subject.CurrentState = SecuritySubjectStateNames.ManuallyBlocked;
        }

        state.UpdatedAtUtc = Now();
    }

    private async Task ApplyBlockStateAsync(
        string subjectType,
        string subjectValue,
        string blockType,
        DateTimeOffset? expiresAtUtc,
        bool firewallEnforced,
        CancellationToken cancellationToken)
    {
        var subject = await EnsureSubjectAsync(subjectType, subjectValue, cancellationToken);
        var state = await db.SecuritySubjectStates.SingleOrDefaultAsync(x => x.SecuritySubjectId == subject.Id, cancellationToken);
        if (state is null)
        {
            state = new SecuritySubjectStateEntity { SecuritySubjectId = subject.Id };
            db.SecuritySubjectStates.Add(state);
        }

        state.ManualBlockActive = true;
        if (firewallEnforced || blockType == "firewall")
        {
            state.FirewallBlockedUntilUtc = expiresAtUtc;
            subject.CurrentState = SecuritySubjectStateNames.FirewallBlocked;
        }
        else
        {
            state.SoftBlockedUntilUtc = expiresAtUtc;
            subject.CurrentState = SecuritySubjectStateNames.SoftBlocked;
        }

        state.LastEscalationReason = firewallEnforced ? "manual_firewall_block" : "manual_soft_block";
        state.LastEscalationAtUtc = Now();
        state.UpdatedAtUtc = Now();
        await db.SaveChangesAsync(cancellationToken);
    }

    private async Task ClearBlockStateAsync(string subjectType, string normalizedValue, CancellationToken cancellationToken)
    {
        var subject = await db.SecuritySubjects.SingleOrDefaultAsync(x => x.SubjectType == subjectType && x.NormalizedValue == normalizedValue, cancellationToken);
        if (subject is null)
        {
            return;
        }

        var state = await db.SecuritySubjectStates.SingleOrDefaultAsync(x => x.SecuritySubjectId == subject.Id, cancellationToken);
        if (state is null)
        {
            return;
        }

        state.ManualBlockActive = false;
        state.SoftBlockedUntilUtc = null;
        state.FirewallBlockedUntilUtc = null;
        state.UpdatedAtUtc = Now();
        subject.CurrentState = state.ManualAllowActive ? SecuritySubjectStateNames.ManuallyAllowed : SecuritySubjectStateNames.Observed;
        await db.SaveChangesAsync(cancellationToken);
    }

    private async Task RefreshSubjectStateFlagsAsync(string subjectType, string normalizedValue, CancellationToken cancellationToken)
    {
        var subject = await db.SecuritySubjects.SingleOrDefaultAsync(x => x.SubjectType == subjectType && x.NormalizedValue == normalizedValue, cancellationToken);
        if (subject is null)
        {
            return;
        }

        var state = await db.SecuritySubjectStates.SingleOrDefaultAsync(x => x.SecuritySubjectId == subject.Id, cancellationToken);
        if (state is null)
        {
            return;
        }

        var now = Now();
        var activeEntries = await db.ManualSecurityEntries.AsNoTracking()
            .Where(x => x.SubjectType == subjectType && x.NormalizedValue == normalizedValue && x.Enabled)
            .Where(x => x.IsPermanent || x.ExpiresAtUtc == null || x.ExpiresAtUtc > now)
            .ToListAsync(cancellationToken);
        state.ManualAllowActive = activeEntries.Any(x => x.EntryType == ManualSecurityEntryTypeNames.Allow);
        state.ManualBlockActive = activeEntries.Any(x => x.EntryType == ManualSecurityEntryTypeNames.Block);
        state.UpdatedAtUtc = now;
        subject.CurrentState = state.ManualBlockActive
            ? SecuritySubjectStateNames.ManuallyBlocked
            : state.ManualAllowActive
                ? SecuritySubjectStateNames.ManuallyAllowed
                : SecuritySubjectStateNames.Observed;
    }

    private async Task<SecuritySubjectStateResponse?> StateForSubjectValueAsync(string subjectType, string normalizedValue, CancellationToken cancellationToken)
    {
        var subject = await db.SecuritySubjects.AsNoTracking().SingleOrDefaultAsync(x => x.SubjectType == subjectType && x.NormalizedValue == normalizedValue, cancellationToken);
        if (subject is null)
        {
            return null;
        }

        var state = await db.SecuritySubjectStates.AsNoTracking().SingleOrDefaultAsync(x => x.SecuritySubjectId == subject.Id, cancellationToken);
        return state is null ? null : ToStateResponse(state);
    }

    private async Task<FirewallPlanPreviewResponse?> PreviewAnyFirewallPlanAsync(CancellationToken cancellationToken)
    {
        var host = await db.FirewallHosts.AsNoTracking().OrderBy(x => x.Name).FirstOrDefaultAsync(cancellationToken);
        return host is null ? null : await firewall.PlanForHostAsync(host.Id, cancellationToken);
    }

    private async Task WriteSecurityEventAsync(
        ManualSecurityEntryEntity entry,
        string eventType,
        string severity,
        string decision,
        CancellationToken cancellationToken)
    {
        db.SecurityEvents.Add(new SecurityEventEntity
        {
            Category = "manual_action",
            Action = eventType,
            SubjectType = entry.SubjectType,
            SubjectValue = entry.SubjectValue,
            NormalizedSubjectValue = entry.NormalizedValue,
            EventType = eventType,
            Severity = severity,
            Decision = decision,
            Source = "admin",
            Reason = entry.Reason,
            OccurredAtUtc = Now(),
        });
        await db.SaveChangesAsync(cancellationToken);
    }

    private static bool MatchesSubjectSearch(SecuritySubjectEntity subject, string query, NormalizedSecuritySubject? normalized)
    {
        if (string.IsNullOrWhiteSpace(query))
        {
            return true;
        }

        if (normalized is not null
            && string.Equals(subject.SubjectType, normalized.SubjectType, StringComparison.OrdinalIgnoreCase)
            && string.Equals(subject.NormalizedValue, normalized.NormalizedValue, StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        return subject.SubjectValue.Contains(query, StringComparison.OrdinalIgnoreCase)
            || subject.NormalizedValue.Contains(query, StringComparison.OrdinalIgnoreCase)
            || (subject.LastCountry?.Contains(query, StringComparison.OrdinalIgnoreCase) ?? false)
            || (subject.LastRegion?.Contains(query, StringComparison.OrdinalIgnoreCase) ?? false)
            || (subject.LastAsn?.Contains(query, StringComparison.OrdinalIgnoreCase) ?? false)
            || (subject.LastAsOrg?.Contains(query, StringComparison.OrdinalIgnoreCase) ?? false)
            || subject.CurrentState.Contains(query, StringComparison.OrdinalIgnoreCase);
    }

    private static NormalizedSecuritySubject? TryNormalizeSearch(string query)
    {
        if (string.IsNullOrWhiteSpace(query))
        {
            return null;
        }

        foreach (var type in new[]
        {
            SecuritySubjectTypeNames.Ip,
            SecuritySubjectTypeNames.Cidr,
            SecuritySubjectTypeNames.Asn,
            SecuritySubjectTypeNames.Country,
            SecuritySubjectTypeNames.Region,
        })
        {
            if (SecuritySubjectNormalizer.TryNormalize(type, query, out var normalized))
            {
                return normalized;
            }
        }

        return null;
    }

    private static NormalizedSecuritySubject NormalizeSubject(string subjectType, string value)
        => SecuritySubjectNormalizer.TryNormalize(subjectType, value, out var normalized)
            ? normalized
            : throw new InvalidOperationException("Unsupported or invalid security subject.");

    private static bool SubjectValuesOverlap(string candidateType, string candidateValue, SecuritySubjectEntity subject)
    {
        if (string.Equals(candidateType, subject.SubjectType, StringComparison.OrdinalIgnoreCase)
            && string.Equals(candidateValue, subject.NormalizedValue, StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (IPAddress.TryParse(subject.NormalizedValue, out var ip)
            && string.Equals(candidateType, SecuritySubjectTypeNames.Cidr, StringComparison.OrdinalIgnoreCase))
        {
            return SecuritySubjectNormalizer.IsInCidr(ip, candidateValue);
        }

        return false;
    }

    private static void ApplyBypassFlags(
        ManualSecurityEntryEntity entity,
        bool? bypassBlocking,
        bool? bypassAdaptiveEscalation,
        bool? bypassRateLimit,
        bool? bypassChallenge,
        bool? bypassSso)
    {
        if (entity.EntryType == ManualSecurityEntryTypeNames.Block)
        {
            entity.BypassBlocking = false;
            entity.BypassAdaptiveEscalation = false;
            entity.BypassRateLimit = false;
            entity.BypassChallenge = false;
            entity.BypassSso = false;
            return;
        }

        entity.BypassBlocking = bypassBlocking ?? true;
        entity.BypassAdaptiveEscalation = bypassAdaptiveEscalation ?? true;
        entity.BypassRateLimit = bypassRateLimit ?? false;
        entity.BypassChallenge = bypassChallenge ?? false;
        entity.BypassSso = bypassSso ?? false;
    }

    private static string NormalizeEntryType(string value)
        => value.Trim().ToLowerInvariant() switch
        {
            ManualSecurityEntryTypeNames.Allow => ManualSecurityEntryTypeNames.Allow,
            ManualSecurityEntryTypeNames.Block => ManualSecurityEntryTypeNames.Block,
            _ => throw new InvalidOperationException("Manual entry type must be allow or block."),
        };

    private static string NormalizeBlockType(string? value)
        => (value ?? "soft").Trim().ToLowerInvariant() switch
        {
            "soft" or "soft_block" => "soft",
            "firewall" or "firewall_block" => "firewall",
            _ => throw new InvalidOperationException("Block type must be soft or firewall."),
        };

    private static string NormalizeScopeType(string value)
        => value.Trim().ToLowerInvariant() switch
        {
            ManualSecurityScopeTypeNames.Global => ManualSecurityScopeTypeNames.Global,
            ManualSecurityScopeTypeNames.Resource => ManualSecurityScopeTypeNames.Resource,
            ManualSecurityScopeTypeNames.RootDomain => ManualSecurityScopeTypeNames.RootDomain,
            ManualSecurityScopeTypeNames.TraefikConnection => ManualSecurityScopeTypeNames.TraefikConnection,
            ManualSecurityScopeTypeNames.FirewallHost => ManualSecurityScopeTypeNames.FirewallHost,
            _ => throw new InvalidOperationException("Unsupported manual entry scope."),
        };

    private static string? NormalizeScopeId(string? value)
        => string.IsNullOrWhiteSpace(value) ? null : value.Trim();

    private static string? NormalizeReason(string? value)
        => string.IsNullOrWhiteSpace(value) ? null : value.Trim();

    private static bool IsFirewallEligible(string subjectType)
        => subjectType is SecuritySubjectTypeNames.Ip or SecuritySubjectTypeNames.Cidr;

    private static string NormalizeRuleType(string matchType)
        => matchType.Trim().ToLowerInvariant() switch
        {
            "ip" => SecuritySubjectTypeNames.Ip,
            "cidr" => SecuritySubjectTypeNames.Cidr,
            "country" => SecuritySubjectTypeNames.Country,
            "region" => SecuritySubjectTypeNames.Region,
            "asn" => SecuritySubjectTypeNames.Asn,
            _ => matchType.Trim().ToLowerInvariant(),
        };

    private static string NormalizeBlockType(BlocklistEntryEntity entry)
        => string.IsNullOrWhiteSpace(entry.SubjectType)
            ? entry.Type.Trim().ToLowerInvariant()
            : entry.SubjectType.Trim().ToLowerInvariant();

    private static string NormalizeBlockValue(BlocklistEntryEntity entry)
        => !string.IsNullOrWhiteSpace(entry.NormalizedValue)
            ? entry.NormalizedValue
            : NormalizeSubjectValue(NormalizeBlockType(entry), !string.IsNullOrWhiteSpace(entry.Value) ? entry.Value : entry.ClientIp);

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
        => SecuritySubjectNormalizer.TryNormalize(subjectType, value, out var normalized)
            ? normalized.NormalizedValue
            : value.Trim();

    private DateTimeOffset Now() => timeProvider?.GetUtcNow() ?? DateTimeOffset.UtcNow;

    public static SecuritySubjectSummaryResponse ToSubjectSummary(SecuritySubjectEntity entity) => new(
        entity.Id,
        entity.SubjectType,
        entity.SubjectValue,
        entity.NormalizedValue,
        entity.CurrentState,
        entity.FirstSeenAtUtc,
        entity.LastSeenAtUtc,
        entity.LastCountry,
        entity.LastRegion,
        entity.LastAsn,
        entity.LastAsOrg);

    public static SecuritySubjectStateResponse ToStateResponse(SecuritySubjectStateEntity entity) => new(
        entity.SecuritySubjectId,
        entity.ChallengeRequired,
        entity.ChallengeRequiredSinceUtc,
        entity.ChallengeReason,
        entity.ChallengeResourceId,
        entity.ChallengeAttempts,
        entity.RequestsWhileChallenged,
        entity.FailedChallengeCount,
        entity.SuccessfulChallengeCount,
        entity.LastChallengeSolvedAtUtc,
        entity.SoftBlockedUntilUtc,
        entity.FirewallBlockedUntilUtc,
        entity.ManualAllowActive,
        entity.ManualBlockActive,
        entity.LastEscalationReason,
        entity.LastEscalationAtUtc,
        entity.UpdatedAtUtc);

    public static ManualSecurityEntryResponse ToManualResponse(ManualSecurityEntryEntity entity) => new(
        entity.Id,
        entity.SubjectType,
        entity.SubjectValue,
        entity.NormalizedValue,
        entity.EntryType,
        entity.ScopeType,
        entity.ScopeId,
        entity.Reason,
        entity.CreatedByAdminId,
        entity.CreatedAtUtc,
        entity.ExpiresAtUtc,
        entity.IsPermanent,
        entity.BypassBlocking,
        entity.BypassAdaptiveEscalation,
        entity.BypassRateLimit,
        entity.BypassChallenge,
        entity.BypassSso,
        entity.Enabled,
        entity.LastHitAtUtc);

    private static SecurityEventResponse ToEventResponse(SecurityEventEntity entity) => new(
        entity.Id,
        entity.OccurredAtUtc,
        entity.SubjectType,
        entity.SubjectValue,
        entity.NormalizedSubjectValue,
        entity.ResourceId,
        entity.ConnectionId,
        entity.EventType ?? entity.Category,
        entity.Severity,
        entity.Decision ?? entity.Action,
        entity.Source,
        entity.Reason,
        entity.RequestMethod,
        entity.RequestPath ?? entity.Path,
        entity.StatusCode,
        entity.RequestId,
        entity.MetadataJson ?? entity.DetailsJson);

    private static SecurityRequestBucketResponse ToBucketResponse(SecurityRequestBucketEntity entity) => new(
        entity.Id,
        entity.BucketStartUtc,
        entity.BucketSizeSeconds,
        entity.SubjectType,
        entity.NormalizedSubjectValue,
        entity.ResourceId,
        entity.RootDomain,
        entity.Country ?? entity.CountryCode,
        entity.Region ?? entity.RegionCode,
        entity.Asn,
        entity.Method,
        entity.PathPrefix,
        entity.StatusClass,
        entity.RequestCount == 0 ? entity.TotalCount : entity.RequestCount,
        entity.BlockedCount,
        entity.ChallengedCount,
        entity.ChallengeIgnoredCount,
        entity.FailedChallengeCount);
}
