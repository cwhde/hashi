using Hashi.Contracts.Api;
using Hashi.Core.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class InternalAgentDnsSettingsService(
    HashiDbContext db,
    AuditService audit,
    AdGuardSyncService adguard)
{
    public async Task<InternalAgentDnsSettingsResponse> GetAsync(CancellationToken cancellationToken = default)
    {
        var settings = await GetOrCreateSettingsAsync(cancellationToken);
        await EnsureAgentSettingsAsync(cancellationToken);
        return await ToResponseAsync(settings.Id, cancellationToken);
    }

    public async Task<InternalAgentDnsSettingsResponse> UpdateAsync(
        InternalAgentDnsSettingsRequest request,
        CancellationToken cancellationToken = default)
    {
        var settings = await GetOrCreateSettingsAsync(cancellationToken);
        var oldDomain = settings.Domain;

        settings.Enabled = request.Enabled;
        settings.Domain = InternalAgentDnsName.NormalizeDomain(request.Domain);
        settings.KeepLastRewriteWhenAgentStale = request.KeepLastRewriteWhenAgentStale ?? true;
        settings.AdGuardConnectionId = request.AdGuardConnectionId;
        settings.UpdatedAtUtc = DateTimeOffset.UtcNow;

        if (settings.AdGuardConnectionId is Guid connectionId)
        {
            var exists = await db.AdGuardConnections.AnyAsync(x => x.Id == connectionId, cancellationToken);
            if (!exists)
            {
                throw new InvalidOperationException("Selected AdGuard connection was not found.");
            }
        }

        await EnsureAgentSettingsAsync(cancellationToken);
        var existingByAgent = await db.InternalAgentDnsAgentSettings
            .ToDictionaryAsync(x => x.PulseAgentId, cancellationToken);

        foreach (var agentRequest in request.Agents ?? [])
        {
            if (!existingByAgent.TryGetValue(agentRequest.PulseAgentId, out var agentSettings))
            {
                var agentExists = await db.PulseAgents.AnyAsync(x => x.Id == agentRequest.PulseAgentId, cancellationToken);
                if (!agentExists)
                {
                    continue;
                }

                agentSettings = new InternalAgentDnsAgentSettingsEntity
                {
                    PulseAgentId = agentRequest.PulseAgentId,
                };
                db.InternalAgentDnsAgentSettings.Add(agentSettings);
                existingByAgent[agentRequest.PulseAgentId] = agentSettings;
            }

            agentSettings.Enabled = agentRequest.Enabled;
            agentSettings.NameOverride = string.IsNullOrWhiteSpace(agentRequest.NameOverride)
                ? null
                : InternalAgentDnsName.NormalizeLabel(agentRequest.NameOverride);
            agentSettings.IpMode = NormalizeIpMode(agentRequest.IpMode);
            agentSettings.KeepLastRewriteWhenStale = agentRequest.KeepLastRewriteWhenStale ?? settings.KeepLastRewriteWhenAgentStale;
            agentSettings.UpdatedAtUtc = DateTimeOffset.UtcNow;
        }

        await EnsureNoCollisionsAsync(settings.Domain, cancellationToken);
        await db.SaveChangesAsync(cancellationToken);

        await audit.WriteAsync(
            "settings",
            "internal_agent_dns_updated",
            subjectType: "settings",
            subjectId: settings.Id.ToString(),
            metadata: new { settings.Enabled, oldDomain, settings.Domain, settings.AdGuardConnectionId },
            cancellationToken: cancellationToken);

        return await ToResponseAsync(settings.Id, cancellationToken);
    }

    public async Task<AdGuardRewritePlanResponse> PreviewSyncAsync(CancellationToken cancellationToken = default)
    {
        var settings = await GetOrCreateSettingsAsync(cancellationToken);
        if (settings.AdGuardConnectionId is not Guid connectionId)
        {
            throw new InvalidOperationException("Select an AdGuard connection before previewing internal agent DNS.");
        }

        return await adguard.PlanSyncAsync(
            connectionId,
            updateInternalAgentDnsDesiredState: true,
            cancellationToken: cancellationToken);
    }

    public async Task<AdGuardRewriteApplyResponse> ApplySyncAsync(
        AdGuardRewriteApplyRequest request,
        CancellationToken cancellationToken = default)
    {
        var settings = await GetOrCreateSettingsAsync(cancellationToken);
        if (settings.AdGuardConnectionId is not Guid connectionId)
        {
            throw new InvalidOperationException("Select an AdGuard connection before applying internal agent DNS.");
        }

        var result = await adguard.ApplyPlanAsync(
            connectionId,
            request,
            updateInternalAgentDnsDesiredState: true,
            cancellationToken: cancellationToken);
        settings.LastSyncStatus = result.Status;
        settings.UpdatedAtUtc = DateTimeOffset.UtcNow;
        await db.SaveChangesAsync(cancellationToken);
        return result;
    }

    private async Task<InternalAgentDnsSettingsEntity> GetOrCreateSettingsAsync(CancellationToken cancellationToken)
    {
        var settings = await db.InternalAgentDnsSettings.SingleOrDefaultAsync(cancellationToken);
        if (settings is not null)
        {
            return settings;
        }

        settings = new InternalAgentDnsSettingsEntity();
        db.InternalAgentDnsSettings.Add(settings);
        await db.SaveChangesAsync(cancellationToken);
        return settings;
    }

    private async Task EnsureAgentSettingsAsync(CancellationToken cancellationToken)
    {
        var existing = await db.InternalAgentDnsAgentSettings
            .Select(x => x.PulseAgentId)
            .ToHashSetAsync(cancellationToken);
        var missingAgents = await db.PulseAgents
            .Where(x => !existing.Contains(x.Id))
            .Select(x => x.Id)
            .ToListAsync(cancellationToken);

        foreach (var agentId in missingAgents)
        {
            db.InternalAgentDnsAgentSettings.Add(new InternalAgentDnsAgentSettingsEntity
            {
                PulseAgentId = agentId,
            });
        }

        if (missingAgents.Count > 0)
        {
            await db.SaveChangesAsync(cancellationToken);
        }
    }

    private async Task EnsureNoCollisionsAsync(string domain, CancellationToken cancellationToken)
    {
        var agents = await db.PulseAgents.AsNoTracking().ToDictionaryAsync(x => x.Id, cancellationToken);
        var agentSettings = await db.InternalAgentDnsAgentSettings
            .Where(x => x.Enabled)
            .ToListAsync(cancellationToken);
        var seen = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);

        foreach (var item in agentSettings)
        {
            if (!agents.TryGetValue(item.PulseAgentId, out var agent))
            {
                continue;
            }

            var label = InternalAgentDnsName.NormalizeLabel(item.NameOverride ?? agent.Name);
            var fqdn = $"{label}.{domain}";
            if (seen.TryGetValue(fqdn, out var existing))
            {
                throw new InvalidOperationException(
                    $"Internal agent DNS name collision for {fqdn}. Set an override for '{existing}' or '{agent.Name}'.");
            }

            seen[fqdn] = agent.Name;
        }
    }

    private async Task<InternalAgentDnsSettingsResponse> ToResponseAsync(int settingsId, CancellationToken cancellationToken)
    {
        var settings = await db.InternalAgentDnsSettings.AsNoTracking()
            .SingleAsync(x => x.Id == settingsId, cancellationToken);
        var agents = await db.InternalAgentDnsAgentSettings.AsNoTracking()
            .OrderBy(x => x.PulseAgent.Name)
            .Select(x => new InternalAgentDnsAgentSettingsResponse(
                x.Id,
                x.PulseAgentId,
                x.Enabled,
                x.NameOverride,
                x.IpMode,
                x.KeepLastRewriteWhenStale,
                x.UpdatedAtUtc))
            .ToListAsync(cancellationToken);

        return new InternalAgentDnsSettingsResponse(
            settings.Enabled,
            settings.Domain,
            settings.KeepLastRewriteWhenAgentStale,
            settings.AdGuardConnectionId,
            settings.LastSyncStatus,
            settings.LastAppliedHash,
            settings.UpdatedAtUtc,
            agents);
    }

    private static string NormalizeIpMode(string? mode)
        => (mode ?? string.Empty).Trim().ToLowerInvariant() switch
        {
            PulseTargetIpModeNames.Public => PulseTargetIpModeNames.Public,
            PulseTargetIpModeNames.Private => PulseTargetIpModeNames.PrivateSelected,
            PulseTargetIpModeNames.PrivateSelected => PulseTargetIpModeNames.PrivateSelected,
            PulseTargetIpModeNames.PrivateCandidate => PulseTargetIpModeNames.PrivateCandidate,
            _ => PulseTargetIpModeNames.Selected,
        };
}
