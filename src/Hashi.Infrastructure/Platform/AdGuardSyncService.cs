using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Core.Dns;
using Hashi.Core.Sync;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Hashi.Infrastructure.Sync;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class AdGuardSyncService(
    HashiDbContext db,
    IHttpClientFactory httpClientFactory,
    SecretRecordService secrets,
    AuditService audit,
    SyncRunService syncRuns,
    ConnectionTargetResolver targetResolver)
{
    public async Task<IReadOnlyList<AdGuardConnectionResponse>> ListConnectionsAsync(CancellationToken cancellationToken = default)
    {
        var items = await db.AdGuardConnections
            .OrderBy(x => x.Name)
            .ToListAsync(cancellationToken);
        var responses = new List<AdGuardConnectionResponse>();
        foreach (var item in items)
        {
            responses.Add(await ToConnectionResponseAsync(item, cancellationToken));
        }

        return responses;
    }

    public async Task<AdGuardConnectionResponse> CreateConnectionAsync(
        CreateAdGuardConnectionRequest request,
        CancellationToken cancellationToken = default)
    {
        var secret = await secrets.StoreAsync(
            SecretPurpose.AdGuardCredential,
            $"AdGuard: {request.Name}",
            JsonSerializer.SerializeToUtf8Bytes(new { password = request.Password }),
            cancellationToken,
            serviceSyncEligible: true);
        var target = CreateTargetFromRequest(request);
        var compatibilityBaseUrl = string.IsNullOrWhiteSpace(request.BaseUrl)
            ? ConnectionTargetResolver.ToBaseUrl(target, target.StaticHost ?? target.StaticIp ?? "127.0.0.1")
            : request.BaseUrl.TrimEnd('/');
        var connection = new AdGuardConnectionEntity
        {
            Name = request.Name,
            BaseUrl = compatibilityBaseUrl,
            PasswordSecretId = secret.Id,
        };
        target.OwnerId = connection.Id;
        db.AdGuardConnections.Add(connection);
        db.ConnectionTargets.Add(target);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync(
            "adguard",
            "connection_target_saved",
            subjectType: ConnectionTargetOwnerTypeNames.AdGuardConnection,
            subjectId: connection.Id.ToString(),
            metadata: new { targetMode = target.TargetMode, target.PulseAgentId },
            cancellationToken: cancellationToken);
        return await ToConnectionResponseAsync(connection, cancellationToken);
    }

    public async Task<AdGuardConnectionTestResponse> TestConnectionAsync(Guid connectionId, CancellationToken cancellationToken = default)
    {
        ResolvedConnectionTarget? resolved = null;
        ConnectionTargetEntity? target = null;
        try
        {
            var connection = await db.AdGuardConnections.SingleOrDefaultAsync(x => x.Id == connectionId, cancellationToken)
                ?? throw new InvalidOperationException("AdGuard connection not found.");
            target = await targetResolver.GetOrCreateAdGuardTargetAsync(connection, cancellationToken);
            resolved = await targetResolver.ResolveAsync(target, cancellationToken: cancellationToken);
            if (resolved.Status == ConnectionTargetStatusNames.Failed)
            {
                return new AdGuardConnectionTestResponse(
                    false,
                    resolved.Error,
                    ToTargetResponse(target),
                    resolved.BaseUri?.ToString().TrimEnd('/'),
                    resolved.IsStale);
            }

            var client = await CreateAuthorizedClientAsync(connection, resolved, cancellationToken);
            using var response = await client.GetAsync("control/status", cancellationToken);
            response.EnsureSuccessStatusCode();
            return new AdGuardConnectionTestResponse(
                true,
                null,
                ToTargetResponse(target),
            resolved.BaseUri?.ToString().TrimEnd('/'),
                resolved.IsStale);
        }
        catch (Exception ex)
        {
            return new AdGuardConnectionTestResponse(
                false,
                ex.Message,
                target is null ? null : ToTargetResponse(target),
                resolved?.BaseUri?.ToString().TrimEnd('/'),
                resolved?.IsStale ?? false);
        }
    }

    public async Task<AdGuardRewritePlanResponse?> DeleteRewriteAsync(Guid connectionId, Guid rewriteId, CancellationToken cancellationToken = default)
    {
        var rewrite = await db.AdGuardRewrites.SingleOrDefaultAsync(
            x => x.Id == rewriteId && x.ConnectionId == connectionId,
            cancellationToken);
        if (rewrite is null)
        {
            return null;
        }

        if (!rewrite.ManagedByHashi)
        {
            throw new InvalidOperationException("Rewrite is managed manually and cannot be deleted by Hashi.");
        }

        var plan = await PlanSyncAsync(connectionId, deleteRewriteId: rewriteId, cancellationToken: cancellationToken);
        await audit.WriteAsync("adguard", "rewrite_delete_planned", subjectType: "rewrite", subjectId: rewrite.Id.ToString(), cancellationToken: cancellationToken);
        return plan;
    }

    public async Task<IReadOnlyList<AdGuardRewriteResponse>> ListRewritesAsync(Guid connectionId, CancellationToken cancellationToken = default)
    {
        var items = await db.AdGuardRewrites.AsNoTracking()
            .Where(x => x.ConnectionId == connectionId)
            .OrderBy(x => x.Domain)
            .ToListAsync(cancellationToken);
        return items.Select(ToResponse).ToList();
    }

    public async Task<AdGuardRewriteMutationResponse> UpsertRewriteAsync(
        Guid connectionId,
        UpsertAdGuardRewriteRequest request,
        CancellationToken cancellationToken = default)
    {
        var domain = NormalizeDomain(request.Domain);
        var answer = request.Answer.Trim();
        var rewrite = await db.AdGuardRewrites.SingleOrDefaultAsync(
            x => x.ConnectionId == connectionId && x.Domain == domain,
            cancellationToken);
        if (rewrite is null)
        {
            rewrite = new AdGuardRewriteEntity
            {
                ConnectionId = connectionId,
                Domain = domain,
                ManagedByHashi = true,
                Source = AdGuardRewriteSourceNames.Manual,
            };
            db.AdGuardRewrites.Add(rewrite);
        }
        else if (!rewrite.ManagedByHashi)
        {
            throw new InvalidOperationException("Rewrite is managed manually and cannot be changed by Hashi.");
        }

        rewrite.Answer = answer;
        await db.SaveChangesAsync(cancellationToken);
        var plan = await PlanSyncAsync(connectionId, cancellationToken: cancellationToken);
        await audit.WriteAsync("adguard", "rewrite_desired_saved", subjectType: "rewrite", subjectId: rewrite.Id.ToString(), cancellationToken: cancellationToken);
        return new AdGuardRewriteMutationResponse(
            ToResponse(rewrite),
            plan);
    }

    public async Task<AdGuardRewriteApplyResponse> SyncManagedRewritesAsync(
        Guid connectionId,
        bool confirmDestructive = false,
        CancellationToken cancellationToken = default)
    {
        var plan = await PlanSyncAsync(
            connectionId,
            updateTopologyDesiredState: true,
            updateInternalAgentDnsDesiredState: true,
            cancellationToken: cancellationToken);
        return await ApplyPlanAsync(
            connectionId,
            new AdGuardRewriteApplyRequest(plan.PlanId, confirmDestructive),
            updateTopologyDesiredState: true,
            updateInternalAgentDnsDesiredState: true,
            cancellationToken: cancellationToken);
    }

    public async Task<AdGuardRewritePlanResponse> PlanSyncAsync(
        Guid connectionId,
        Guid? deleteRewriteId = null,
        bool updateTopologyDesiredState = false,
        bool updateInternalAgentDnsDesiredState = false,
        CancellationToken cancellationToken = default)
    {
        HashSet<string>? topologyDesiredDomains = null;
        if (updateTopologyDesiredState)
        {
            topologyDesiredDomains = await SyncResourceTopologyRewritesAsync(connectionId, cancellationToken);
        }

        HashSet<string>? internalAgentDnsDesiredDomains = null;
        if (updateInternalAgentDnsDesiredState)
        {
            internalAgentDnsDesiredDomains = await SyncInternalAgentDnsRewritesAsync(connectionId, cancellationToken);
        }

        var deleteRewrite = deleteRewriteId is null
            ? null
            : await db.AdGuardRewrites.AsNoTracking().SingleOrDefaultAsync(
                x => x.Id == deleteRewriteId && x.ConnectionId == connectionId && x.ManagedByHashi,
                cancellationToken);
        var remoteRewrites = await ListRemoteRewritesAsync(connectionId, cancellationToken);
        var localManaged = await db.AdGuardRewrites
            .AsNoTracking()
            .Where(x => x.ConnectionId == connectionId && x.ManagedByHashi)
            .ToListAsync(cancellationToken);
        if (deleteRewrite is not null)
        {
            localManaged = localManaged.Where(x => x.Id != deleteRewrite.Id).ToList();
        }

        var changes = new List<AdGuardRewritePlanChange>();
        IReadOnlyList<AdGuardRewriteEntity> staleTopology = topologyDesiredDomains is null
            ? Array.Empty<AdGuardRewriteEntity>()
            : localManaged
                .Where(x => x.Source == AdGuardRewriteSourceNames.Topology && !topologyDesiredDomains.Contains(x.Domain))
                .ToList();
        var staleTopologyDomains = staleTopology
            .Select(x => x.Domain)
            .ToHashSet(StringComparer.OrdinalIgnoreCase);
        IReadOnlyList<AdGuardRewriteEntity> staleInternalAgentDns = internalAgentDnsDesiredDomains is null
            ? Array.Empty<AdGuardRewriteEntity>()
            : localManaged
                .Where(x => x.Source == AdGuardRewriteSourceNames.InternalAgentDns && !internalAgentDnsDesiredDomains.Contains(x.Domain))
                .ToList();
        var staleInternalAgentDnsDomains = staleInternalAgentDns
            .Select(x => x.Domain)
            .ToHashSet(StringComparer.OrdinalIgnoreCase);

        foreach (var rewrite in localManaged.Where(x =>
            !staleTopologyDomains.Contains(x.Domain) &&
            !staleInternalAgentDnsDomains.Contains(x.Domain)))
        {
            var remote = remoteRewrites.FirstOrDefault(x =>
                string.Equals(x.Domain, rewrite.Domain, StringComparison.OrdinalIgnoreCase));
            if (remote is null)
            {
                changes.Add(new AdGuardRewritePlanChange("create", rewrite.Domain, null, rewrite.Answer, "Add Hashi-managed rewrite."));
            }
            else if (!string.Equals(remote.Answer, rewrite.Answer, StringComparison.Ordinal))
            {
                changes.Add(new AdGuardRewritePlanChange("update", rewrite.Domain, remote.Answer, rewrite.Answer, "Update Hashi-managed rewrite answer."));
            }
        }

        if (deleteRewrite is not null)
        {
            var remote = remoteRewrites.FirstOrDefault(x =>
                string.Equals(x.Domain, deleteRewrite.Domain, StringComparison.OrdinalIgnoreCase));
            changes.Add(new AdGuardRewritePlanChange("delete", deleteRewrite.Domain, remote?.Answer ?? deleteRewrite.Answer, null, "Delete Hashi-managed rewrite."));
        }

        foreach (var rewrite in staleTopology)
        {
            var remote = remoteRewrites.FirstOrDefault(x =>
                string.Equals(x.Domain, rewrite.Domain, StringComparison.OrdinalIgnoreCase));
            changes.Add(new AdGuardRewritePlanChange("delete", rewrite.Domain, remote?.Answer ?? rewrite.Answer, null, "Delete stale topology-generated rewrite."));
        }

        foreach (var rewrite in staleInternalAgentDns)
        {
            var remote = remoteRewrites.FirstOrDefault(x =>
                string.Equals(x.Domain, rewrite.Domain, StringComparison.OrdinalIgnoreCase));
            changes.Add(new AdGuardRewritePlanChange("delete", rewrite.Domain, remote?.Answer ?? rewrite.Answer, null, "Delete stale internal-agent DNS rewrite."));
        }

        var planId = ComputePlanId(connectionId, deleteRewriteId, remoteRewrites, localManaged, changes);
        return new AdGuardRewritePlanResponse(
            planId,
            connectionId,
            changes.Any(x => x.Kind == "delete"),
            changes.Select(x => new AdGuardRewritePlanChangeResponse(x.Kind, x.Domain, x.CurrentAnswer, x.DesiredAnswer, x.Summary)).ToList());
    }

    public async Task<AdGuardRewriteApplyResponse> ApplyPlanAsync(
        Guid connectionId,
        AdGuardRewriteApplyRequest request,
        Guid? deleteRewriteId = null,
        bool updateTopologyDesiredState = false,
        bool updateInternalAgentDnsDesiredState = false,
        CancellationToken cancellationToken = default)
    {
        var plan = await PlanSyncAsync(connectionId, deleteRewriteId, updateTopologyDesiredState, updateInternalAgentDnsDesiredState, cancellationToken);
        if (plan.PlanId != request.PlanId)
        {
            throw new InvalidOperationException("AdGuard plan is stale; preview the rewrite changes again.");
        }

        var run = await syncRuns.BeginRunAsync("adguard", cancellationToken);
        var providerChanges = plan.Changes.Select(x => new ProviderChange(
            "adguard-rewrite",
            x.Domain,
            MapProviderKind(x.Kind),
            x.Summary));
        await syncRuns.AddDiffsAsync(run.Id, providerChanges, cancellationToken);
        await syncRuns.AddStepAsync(run.Id, "adguard-apply", SyncRunStatusNames.Applying, null, cancellationToken);

        if (plan.RequiresConfirmation && !request.ConfirmDestructive)
        {
            await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.AwaitingConfirmation, SyncRiskLevel.Destructive, "AdGuard delete requires confirmation.", cancellationToken);
            await audit.WriteAsync("adguard", "apply_awaiting_confirmation", subjectType: "sync_run", subjectId: run.Id.ToString(), cancellationToken: cancellationToken);
            return new AdGuardRewriteApplyResponse(run.Id, false, SyncRunStatusNames.AwaitingConfirmation, "AdGuard delete requires confirmation.");
        }

        try
        {
            foreach (var change in plan.Changes)
            {
                await ApplyChangeAsync(connectionId, change, cancellationToken);
            }

            if (deleteRewriteId is Guid rewriteId)
            {
                var rewrite = await db.AdGuardRewrites.SingleOrDefaultAsync(
                    x => x.Id == rewriteId && x.ConnectionId == connectionId && x.ManagedByHashi,
                    cancellationToken);
                if (rewrite is not null)
                {
                    db.AdGuardRewrites.Remove(rewrite);
                    await db.SaveChangesAsync(cancellationToken);
                }
            }

            var deletedDomains = plan.Changes
                .Where(x => x.Kind == "delete")
                .Select(x => x.Domain)
                .ToHashSet(StringComparer.OrdinalIgnoreCase);
            if (deletedDomains.Count > 0)
            {
                var staleGenerated = await db.AdGuardRewrites
                    .Where(x =>
                        x.ConnectionId == connectionId &&
                        x.ManagedByHashi &&
                        (x.Source == AdGuardRewriteSourceNames.Topology ||
                            x.Source == AdGuardRewriteSourceNames.InternalAgentDns))
                    .ToListAsync(cancellationToken);
                staleGenerated = staleGenerated
                    .Where(x => deletedDomains.Contains(x.Domain))
                    .ToList();
                if (staleGenerated.Count > 0)
                {
                    db.AdGuardRewrites.RemoveRange(staleGenerated);
                    await db.SaveChangesAsync(cancellationToken);
                }
            }

            if (updateInternalAgentDnsDesiredState)
            {
                var settings = await db.InternalAgentDnsSettings.SingleOrDefaultAsync(cancellationToken);
                if (settings is not null && settings.AdGuardConnectionId == connectionId)
                {
                    settings.LastSyncStatus = SyncRunStatusNames.Succeeded;
                    settings.LastAppliedHash = await ComputeInternalAgentDnsAppliedHashAsync(connectionId, cancellationToken);
                    settings.UpdatedAtUtc = DateTimeOffset.UtcNow;
                    await db.SaveChangesAsync(cancellationToken);
                }
            }

            await syncRuns.AddStepAsync(run.Id, "adguard-apply", SyncRunStatusNames.Succeeded, $"{plan.Changes.Count} changes", cancellationToken);
            await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Succeeded, plan.RequiresConfirmation ? SyncRiskLevel.Destructive : SyncRiskLevel.Low, null, cancellationToken);
            await audit.WriteAsync("adguard", "apply_succeeded", subjectType: "sync_run", subjectId: run.Id.ToString(), metadata: new { connectionId, changes = plan.Changes.Count }, cancellationToken: cancellationToken);
            return new AdGuardRewriteApplyResponse(run.Id, true, SyncRunStatusNames.Succeeded, null);
        }
        catch (Exception ex)
        {
            await syncRuns.AddStepAsync(run.Id, "adguard-apply", SyncRunStatusNames.Failed, ex.Message, cancellationToken);
            await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Failed, SyncRiskLevel.High, ex.Message, cancellationToken);
            if (updateInternalAgentDnsDesiredState)
            {
                var settings = await db.InternalAgentDnsSettings.SingleOrDefaultAsync(cancellationToken);
                if (settings is not null && settings.AdGuardConnectionId == connectionId)
                {
                    settings.LastSyncStatus = SyncRunStatusNames.Failed;
                    settings.UpdatedAtUtc = DateTimeOffset.UtcNow;
                    await db.SaveChangesAsync(cancellationToken);
                }
            }

            await audit.WriteAsync("adguard", "apply_failed", "failure", subjectType: "sync_run", subjectId: run.Id.ToString(), metadata: new { connectionId, error = ex.Message }, cancellationToken: cancellationToken);
            return new AdGuardRewriteApplyResponse(run.Id, false, SyncRunStatusNames.Failed, ex.Message);
        }
    }

    private async Task<HashSet<string>> SyncInternalAgentDnsRewritesAsync(Guid connectionId, CancellationToken cancellationToken)
    {
        var settings = await db.InternalAgentDnsSettings.SingleOrDefaultAsync(cancellationToken);
        var desiredDomains = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        if (settings is null || !settings.Enabled || settings.AdGuardConnectionId != connectionId)
        {
            return desiredDomains;
        }

        settings.Domain = InternalAgentDnsName.NormalizeDomain(settings.Domain);
        await EnsureInternalAgentSettingsAsync(cancellationToken);

        var agents = await db.PulseAgents.AsNoTracking().ToDictionaryAsync(x => x.Id, cancellationToken);
        var agentSettings = await db.InternalAgentDnsAgentSettings.AsNoTracking()
            .Where(x => x.Enabled)
            .OrderBy(x => x.PulseAgent.Name)
            .ToListAsync(cancellationToken);

        foreach (var agentSetting in agentSettings)
        {
            if (!agents.TryGetValue(agentSetting.PulseAgentId, out var agent) ||
                string.Equals(agent.Status, "revoked", StringComparison.OrdinalIgnoreCase))
            {
                continue;
            }

            var label = InternalAgentDnsName.NormalizeLabel(agentSetting.NameOverride ?? agent.Name);
            var domain = $"{label}.{settings.Domain}";
            if (!desiredDomains.Add(domain))
            {
                throw new InvalidOperationException($"Internal agent DNS name collision for {domain}.");
            }

            var answer = SelectAgentDnsAnswer(agent, agentSetting.IpMode);
            var rewrite = await db.AdGuardRewrites.SingleOrDefaultAsync(
                x => x.ConnectionId == connectionId && x.Domain == domain,
                cancellationToken);

            if (string.IsNullOrWhiteSpace(answer))
            {
                if (rewrite is null ||
                    !rewrite.ManagedByHashi ||
                    rewrite.Source != AdGuardRewriteSourceNames.InternalAgentDns ||
                    !(settings.KeepLastRewriteWhenAgentStale && agentSetting.KeepLastRewriteWhenStale))
                {
                    desiredDomains.Remove(domain);
                }

                continue;
            }

            if (rewrite is null)
            {
                rewrite = new AdGuardRewriteEntity
                {
                    ConnectionId = connectionId,
                    Domain = domain,
                    ManagedByHashi = true,
                    Source = AdGuardRewriteSourceNames.InternalAgentDns,
                };
                db.AdGuardRewrites.Add(rewrite);
            }
            else if (!rewrite.ManagedByHashi || rewrite.Source != AdGuardRewriteSourceNames.InternalAgentDns)
            {
                continue;
            }

            rewrite.Answer = answer;
        }

        await db.SaveChangesAsync(cancellationToken);
        return desiredDomains;
    }

    private async Task EnsureInternalAgentSettingsAsync(CancellationToken cancellationToken)
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

    private async Task<string> ComputeInternalAgentDnsAppliedHashAsync(Guid connectionId, CancellationToken cancellationToken)
    {
        var rewrites = await db.AdGuardRewrites.AsNoTracking()
            .Where(x =>
                x.ConnectionId == connectionId &&
                x.ManagedByHashi &&
                x.Source == AdGuardRewriteSourceNames.InternalAgentDns)
            .OrderBy(x => x.Domain)
            .ToListAsync(cancellationToken);
        var builder = new StringBuilder("internal-agent-dns-v1|");
        foreach (var rewrite in rewrites)
        {
            builder.Append(rewrite.Domain).Append('=').Append(rewrite.Answer).Append('|');
        }

        return Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(builder.ToString()))).ToLowerInvariant();
    }

    private static string? SelectAgentDnsAnswer(PulseAgentEntity agent, string? ipMode)
    {
        return NormalizePulseIpMode(ipMode) switch
        {
            PulseTargetIpModeNames.Public => agent.LastPublicIp,
            PulseTargetIpModeNames.PrivateSelected => FirstNonEmpty(agent.LastSelectedIp, agent.LastPrivateIp),
            PulseTargetIpModeNames.PrivateCandidate => FirstNonEmpty(
                DeserializeStringList(agent.LastPrivateIpv4CandidatesJson).FirstOrDefault(),
                DeserializeStringList(agent.LastPrivateIpv6CandidatesJson).FirstOrDefault(),
                agent.LastPrivateIp),
            _ => FirstNonEmpty(agent.LastSelectedIp, agent.LastPrivateIp, agent.LastPublicIp),
        };
    }

    private async Task<HashSet<string>> SyncResourceTopologyRewritesAsync(Guid connectionId, CancellationToken cancellationToken)
    {
        var settings = await db.AppSettings.AsNoTracking().SingleOrDefaultAsync(cancellationToken);
        var rootDomain = settings?.RootDomain;
        var desiredDomains = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        if (string.IsNullOrWhiteSpace(rootDomain))
        {
            return desiredDomains;
        }

        var hosts = await db.FirewallHosts.AsNoTracking().ToDictionaryAsync(x => x.Id, cancellationToken);
        var pulseAgents = await db.PulseAgents.AsNoTracking().ToDictionaryAsync(x => x.Id, cancellationToken);
        var resources = await db.Resources
            .Where(x => x.Enabled && (x.FirewallHostId != null || x.PulseAgentId != null))
            .ToListAsync(cancellationToken);

        foreach (var resource in resources)
        {
            string? answer = null;
            if (resource.FirewallHostId is Guid hostId && hosts.TryGetValue(hostId, out var host))
            {
                answer = host.InternalTraefikIp;
            }
            else if (resource.PulseAgentId is Guid pulseId && pulseAgents.TryGetValue(pulseId, out var agent))
            {
                answer = agent.LastSelectedIp ?? agent.LastPrivateIp ?? agent.LastPublicIp;
            }

            if (string.IsNullOrWhiteSpace(answer))
            {
                continue;
            }

            var domain = string.IsNullOrWhiteSpace(resource.Domain)
                ? $"{resource.Slug}.{rootDomain}".TrimEnd('.')
                : resource.Domain.TrimEnd('.');
            desiredDomains.Add(domain);

            var rewrite = await db.AdGuardRewrites.SingleOrDefaultAsync(
                x => x.ConnectionId == connectionId && x.Domain == domain,
                cancellationToken);
            if (rewrite is null)
            {
                rewrite = new AdGuardRewriteEntity
                {
                    ConnectionId = connectionId,
                    Domain = domain,
                    ManagedByHashi = true,
                    Source = AdGuardRewriteSourceNames.Topology,
                };
                db.AdGuardRewrites.Add(rewrite);
            }
            else if (!rewrite.ManagedByHashi)
            {
                continue;
            }
            else if (rewrite.Source != AdGuardRewriteSourceNames.Topology)
            {
                continue;
            }

            rewrite.Answer = answer;
        }

        await db.SaveChangesAsync(cancellationToken);
        return desiredDomains;
    }

    private async Task<IReadOnlyList<RemoteAdGuardRewrite>> ListRemoteRewritesAsync(
        Guid connectionId,
        CancellationToken cancellationToken)
    {
        var client = await CreateAuthorizedClientAsync(connectionId, cancellationToken);
        using var response = await client.GetAsync("control/rewrite/list", cancellationToken);
        response.EnsureSuccessStatusCode();
        var body = await response.Content.ReadAsStringAsync(cancellationToken);
        using var doc = JsonDocument.Parse(body);
        var rewrites = new List<RemoteAdGuardRewrite>();
        if (doc.RootElement.TryGetProperty("rewrites", out var array))
        {
            foreach (var item in array.EnumerateArray())
            {
                var domain = item.GetProperty("domain").GetString() ?? string.Empty;
                var answer = item.GetProperty("answer").GetString() ?? string.Empty;
                var id = item.TryGetProperty("id", out var idElement) ? idElement.GetRawText() : null;
                rewrites.Add(new RemoteAdGuardRewrite(domain, answer, id));
            }
        }

        return rewrites;
    }

    private async Task DeleteRemoteRewriteAsync(
        Guid connectionId,
        RemoteAdGuardRewrite rewrite,
        CancellationToken cancellationToken)
    {
        var client = await CreateAuthorizedClientAsync(connectionId, cancellationToken);
        var payload = new { domain = rewrite.Domain, answer = rewrite.Answer };
        using var response = await client.PostAsJsonAsync("control/rewrite/delete", payload, cancellationToken);
        response.EnsureSuccessStatusCode();
    }

    private async Task<HttpClient> CreateAuthorizedClientAsync(Guid connectionId, CancellationToken cancellationToken)
    {
        var connection = await db.AdGuardConnections.SingleOrDefaultAsync(x => x.Id == connectionId, cancellationToken)
            ?? throw new InvalidOperationException("AdGuard connection not found.");
        var resolved = await targetResolver.ResolveAdGuardAsync(connection, cancellationToken: cancellationToken);
        if (resolved.Status == ConnectionTargetStatusNames.Failed)
        {
            throw new InvalidOperationException(resolved.Error ?? "AdGuard target could not be resolved.");
        }

        return await CreateAuthorizedClientAsync(connection, resolved, cancellationToken);
    }

    private async Task<HttpClient> CreateAuthorizedClientAsync(
        AdGuardConnectionEntity connection,
        ResolvedConnectionTarget resolved,
        CancellationToken cancellationToken)
    {
        var password = await ResolvePasswordAsync(connection.PasswordSecretId, cancellationToken);
        var client = httpClientFactory.CreateClient("adguard");
        client.BaseAddress = resolved.BaseUri
            ?? throw new InvalidOperationException("Cannot create HTTP client for unresolved target.");
        if (!string.IsNullOrEmpty(password))
        {
            client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue(
                "Basic",
                Convert.ToBase64String(Encoding.UTF8.GetBytes($"admin:{password}")));
        }

        return client;
    }

    private async Task<AdGuardConnectionResponse> ToConnectionResponseAsync(
        AdGuardConnectionEntity connection,
        CancellationToken cancellationToken)
    {
        var target = await targetResolver.GetOrCreateAdGuardTargetAsync(connection, cancellationToken);
        var resolved = await targetResolver.ResolveAsync(target, cancellationToken: cancellationToken);
        return new AdGuardConnectionResponse(
            connection.Id,
            connection.Name,
            connection.BaseUrl,
            connection.Enabled,
            ToTargetResponse(target),
            resolved.BaseUri?.ToString().TrimEnd('/'),
            resolved.Status,
            resolved.Error);
    }

    private static ConnectionTargetEntity CreateTargetFromRequest(CreateAdGuardConnectionRequest request)
    {
        if (request.Target is null)
        {
            if (string.IsNullOrWhiteSpace(request.BaseUrl))
            {
                throw new InvalidOperationException("Base URL or connection target is required.");
            }

            return ConnectionTargetResolver.FromAdGuardBaseUrl(new AdGuardConnectionEntity
            {
                Id = Guid.Empty,
                BaseUrl = request.BaseUrl,
            });
        }

        var target = request.Target;
        return new ConnectionTargetEntity
        {
            OwnerType = ConnectionTargetOwnerTypeNames.AdGuardConnection,
            TargetMode = NormalizeTargetMode(target.TargetMode),
            StaticHost = string.IsNullOrWhiteSpace(target.StaticHost) ? null : target.StaticHost.Trim(),
            StaticIp = string.IsNullOrWhiteSpace(target.StaticIp) ? null : target.StaticIp.Trim(),
            PulseAgentId = target.PulseAgentId,
            PulseIpMode = NormalizePulseIpMode(target.PulseIpMode),
            PrivateCandidateSelector = string.IsNullOrWhiteSpace(target.PrivateCandidateSelector)
                ? PulsePrivateCandidateSelectorNames.Selected
                : target.PrivateCandidateSelector.Trim(),
            Port = target.Port,
            Scheme = string.Equals(target.Scheme, "https", StringComparison.OrdinalIgnoreCase) ? "https" : "http",
            PathPrefix = string.IsNullOrWhiteSpace(target.PathPrefix) ? null : "/" + target.PathPrefix.Trim().Trim('/'),
            TlsValidationMode = string.IsNullOrWhiteSpace(target.TlsValidationMode) ? TlsValidationModeNames.System : target.TlsValidationMode.Trim(),
            ExpectedTlsHostname = string.IsNullOrWhiteSpace(target.ExpectedTlsHostname) ? null : target.ExpectedTlsHostname.Trim(),
        };
    }

    private static ConnectionTargetResponse ToTargetResponse(ConnectionTargetEntity target) => new(
        target.Id,
        target.OwnerType,
        target.OwnerId,
        target.TargetMode,
        target.StaticHost,
        target.StaticIp,
        target.PulseAgentId,
        target.PulseIpMode,
        target.PrivateCandidateSelector,
        target.Port,
        target.Scheme,
        target.PathPrefix,
        target.TlsValidationMode,
        target.ExpectedTlsHostname,
        target.ResolvedIpSnapshot,
        target.LastResolvedAtUtc,
        target.Status,
        target.LastError);

    private static string NormalizeTargetMode(string? mode)
        => (mode ?? string.Empty).Trim().ToLowerInvariant() switch
        {
            ConnectionTargetModeNames.StaticIp => ConnectionTargetModeNames.StaticIp,
            ConnectionTargetModeNames.PulseAgent => ConnectionTargetModeNames.PulseAgent,
            _ => ConnectionTargetModeNames.StaticHost,
        };

    private static string NormalizePulseIpMode(string? mode)
        => (mode ?? string.Empty).Trim().ToLowerInvariant() switch
        {
            PulseTargetIpModeNames.Public => PulseTargetIpModeNames.Public,
            PulseTargetIpModeNames.Private => PulseTargetIpModeNames.PrivateSelected,
            PulseTargetIpModeNames.PrivateSelected => PulseTargetIpModeNames.PrivateSelected,
            PulseTargetIpModeNames.PrivateCandidate => PulseTargetIpModeNames.PrivateCandidate,
            _ => PulseTargetIpModeNames.Selected,
        };

    private static string? FirstNonEmpty(params string?[] values)
        => values.FirstOrDefault(x => !string.IsNullOrWhiteSpace(x));

    private static IReadOnlyList<string> DeserializeStringList(string? json)
    {
        if (string.IsNullOrWhiteSpace(json))
        {
            return [];
        }

        try
        {
            return JsonSerializer.Deserialize<IReadOnlyList<string>>(json) ?? [];
        }
        catch (JsonException)
        {
            return [];
        }
    }

    private sealed record RemoteAdGuardRewrite(string Domain, string Answer, string? Id);

    private sealed record AdGuardRewritePlanChange(string Kind, string Domain, string? CurrentAnswer, string? DesiredAnswer, string Summary);

    private async Task ApplyChangeAsync(Guid connectionId, AdGuardRewritePlanChangeResponse change, CancellationToken cancellationToken)
    {
        switch (change.Kind)
        {
            case "create":
                await PushToAdGuardAsync(connectionId, change.Domain, change.DesiredAnswer ?? string.Empty, cancellationToken);
                break;
            case "update":
                var updateRemote = (await ListRemoteRewritesAsync(connectionId, cancellationToken))
                    .FirstOrDefault(x => string.Equals(x.Domain, change.Domain, StringComparison.OrdinalIgnoreCase));
                if (updateRemote is not null)
                {
                    await DeleteRemoteRewriteAsync(connectionId, updateRemote, cancellationToken);
                }

                await PushToAdGuardAsync(connectionId, change.Domain, change.DesiredAnswer ?? string.Empty, cancellationToken);
                break;
            case "delete":
                var deleteRemote = (await ListRemoteRewritesAsync(connectionId, cancellationToken))
                    .FirstOrDefault(x => string.Equals(x.Domain, change.Domain, StringComparison.OrdinalIgnoreCase));
                if (deleteRemote is not null)
                {
                    await DeleteRemoteRewriteAsync(connectionId, deleteRemote, cancellationToken);
                }

                break;
        }
    }

    private async Task PushToAdGuardAsync(Guid connectionId, string domain, string answer, CancellationToken cancellationToken)
    {
        var client = await CreateAuthorizedClientAsync(connectionId, cancellationToken);
        var payload = new { domain, answer };
        using var response = await client.PostAsJsonAsync("control/rewrite/add", payload, cancellationToken);
        response.EnsureSuccessStatusCode();
        var body = await response.Content.ReadAsStringAsync(cancellationToken);
        using var doc = JsonDocument.Parse(body);
        if (doc.RootElement.TryGetProperty("id", out var idElement))
        {
            var rewrite = await db.AdGuardRewrites.SingleOrDefaultAsync(
                x => x.ConnectionId == connectionId && x.Domain == domain && x.ManagedByHashi,
                cancellationToken);
            if (rewrite is not null)
            {
                rewrite.ProviderRewriteId = idElement.GetRawText();
                await db.SaveChangesAsync(cancellationToken);
            }
        }
    }

    private static ProviderResultKind MapProviderKind(string kind) => kind switch
    {
        "create" => ProviderResultKind.Created,
        "update" => ProviderResultKind.Updated,
        "delete" => ProviderResultKind.Deleted,
        _ => ProviderResultKind.NoOp,
    };

    private static string NormalizeDomain(string domain) => domain.Trim().TrimEnd('.').ToLowerInvariant();

    private static AdGuardRewriteResponse ToResponse(AdGuardRewriteEntity rewrite) => new(
        rewrite.Id,
        rewrite.Domain,
        rewrite.Answer,
        rewrite.ManagedByHashi,
        rewrite.Source);

    private static Guid ComputePlanId(
        Guid connectionId,
        Guid? deleteRewriteId,
        IReadOnlyList<RemoteAdGuardRewrite> remote,
        IReadOnlyList<AdGuardRewriteEntity> desired,
        IReadOnlyList<AdGuardRewritePlanChange> changes)
    {
        var builder = new StringBuilder()
            .Append("adguard-plan-v1|")
            .Append(connectionId).Append('|')
            .Append(deleteRewriteId).Append('|');
        foreach (var item in remote.OrderBy(x => x.Domain, StringComparer.OrdinalIgnoreCase).ThenBy(x => x.Answer, StringComparer.Ordinal))
        {
            builder.Append("r:").Append(NormalizeDomain(item.Domain)).Append('=').Append(item.Answer).Append('|');
        }

        foreach (var item in desired.OrderBy(x => x.Domain, StringComparer.OrdinalIgnoreCase))
        {
            builder.Append("d:").Append(NormalizeDomain(item.Domain)).Append('=').Append(item.Answer).Append('|');
        }

        foreach (var change in changes.OrderBy(x => x.Domain, StringComparer.OrdinalIgnoreCase).ThenBy(x => x.Kind, StringComparer.Ordinal))
        {
            builder.Append("c:").Append(change.Kind).Append(':').Append(NormalizeDomain(change.Domain)).Append('|');
        }

        var hash = SHA256.HashData(Encoding.UTF8.GetBytes(builder.ToString()));
        return new Guid(hash[..16]);
    }

    private async Task<string?> ResolvePasswordAsync(Guid secretId, CancellationToken cancellationToken)
    {
        var payload = await secrets.DecryptForPurposeAsync(secretId, cancellationToken);
        if (payload is null)
        {
            return null;
        }

        using var doc = JsonDocument.Parse(payload);
        return doc.RootElement.TryGetProperty("password", out var password) ? password.GetString() : null;
    }
}
