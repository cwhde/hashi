using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
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
    SyncRunService syncRuns)
{
    public async Task<IReadOnlyList<AdGuardConnectionResponse>> ListConnectionsAsync(CancellationToken cancellationToken = default)
    {
        var items = await db.AdGuardConnections.AsNoTracking()
            .OrderBy(x => x.Name)
            .ToListAsync(cancellationToken);
        return items.Select(x => new AdGuardConnectionResponse(x.Id, x.Name, x.BaseUrl, x.Enabled)).ToList();
    }

    public async Task<AdGuardConnectionResponse> CreateConnectionAsync(
        CreateAdGuardConnectionRequest request,
        CancellationToken cancellationToken = default)
    {
        var secret = await secrets.StoreAsync(
            SecretPurpose.AdGuardCredential,
            $"AdGuard: {request.Name}",
            JsonSerializer.SerializeToUtf8Bytes(new { password = request.Password }),
            cancellationToken);
        var connection = new AdGuardConnectionEntity
        {
            Name = request.Name,
            BaseUrl = request.BaseUrl.TrimEnd('/'),
            PasswordSecretId = secret.Id,
        };
        db.AdGuardConnections.Add(connection);
        await db.SaveChangesAsync(cancellationToken);
        return new AdGuardConnectionResponse(connection.Id, connection.Name, connection.BaseUrl, connection.Enabled);
    }

    public async Task<AdGuardConnectionTestResponse> TestConnectionAsync(Guid connectionId, CancellationToken cancellationToken = default)
    {
        try
        {
            var client = await CreateAuthorizedClientAsync(connectionId, cancellationToken);
            using var response = await client.GetAsync("control/status", cancellationToken);
            response.EnsureSuccessStatusCode();
            return new AdGuardConnectionTestResponse(true, null);
        }
        catch (Exception ex)
        {
            return new AdGuardConnectionTestResponse(false, ex.Message);
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
        return items.Select(x => new AdGuardRewriteResponse(x.Id, x.Domain, x.Answer, x.ManagedByHashi)).ToList();
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
            new AdGuardRewriteResponse(rewrite.Id, rewrite.Domain, rewrite.Answer, rewrite.ManagedByHashi),
            plan);
    }

    public async Task<AdGuardRewriteApplyResponse> SyncManagedRewritesAsync(Guid connectionId, CancellationToken cancellationToken = default)
    {
        var plan = await PlanSyncAsync(connectionId, updateTopologyDesiredState: true, cancellationToken: cancellationToken);
        return await ApplyPlanAsync(connectionId, new AdGuardRewriteApplyRequest(plan.PlanId, ConfirmDestructive: true), cancellationToken: cancellationToken);
    }

    public async Task<AdGuardRewritePlanResponse> PlanSyncAsync(
        Guid connectionId,
        Guid? deleteRewriteId = null,
        bool updateTopologyDesiredState = false,
        CancellationToken cancellationToken = default)
    {
        if (updateTopologyDesiredState)
        {
            await SyncResourceTopologyRewritesAsync(connectionId, cancellationToken);
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
        foreach (var rewrite in localManaged)
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
        CancellationToken cancellationToken = default)
    {
        var plan = await PlanSyncAsync(connectionId, deleteRewriteId, cancellationToken: cancellationToken);
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

            await syncRuns.AddStepAsync(run.Id, "adguard-apply", SyncRunStatusNames.Succeeded, $"{plan.Changes.Count} changes", cancellationToken);
            await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Succeeded, plan.RequiresConfirmation ? SyncRiskLevel.Destructive : SyncRiskLevel.Low, null, cancellationToken);
            await audit.WriteAsync("adguard", "apply_succeeded", subjectType: "sync_run", subjectId: run.Id.ToString(), metadata: new { connectionId, changes = plan.Changes.Count }, cancellationToken: cancellationToken);
            return new AdGuardRewriteApplyResponse(run.Id, true, SyncRunStatusNames.Succeeded, null);
        }
        catch (Exception ex)
        {
            await syncRuns.AddStepAsync(run.Id, "adguard-apply", SyncRunStatusNames.Failed, ex.Message, cancellationToken);
            await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Failed, SyncRiskLevel.High, ex.Message, cancellationToken);
            await audit.WriteAsync("adguard", "apply_failed", "failure", subjectType: "sync_run", subjectId: run.Id.ToString(), metadata: new { connectionId, error = ex.Message }, cancellationToken: cancellationToken);
            return new AdGuardRewriteApplyResponse(run.Id, false, SyncRunStatusNames.Failed, ex.Message);
        }
    }

    private async Task SyncResourceTopologyRewritesAsync(Guid connectionId, CancellationToken cancellationToken)
    {
        var settings = await db.AppSettings.AsNoTracking().SingleOrDefaultAsync(cancellationToken);
        var rootDomain = settings?.RootDomain;
        if (string.IsNullOrWhiteSpace(rootDomain))
        {
            return;
        }

        var hosts = await db.FirewallHosts.AsNoTracking().ToDictionaryAsync(x => x.Id, cancellationToken);
        var pulseAgents = await db.PulseAgents.AsNoTracking().ToDictionaryAsync(x => x.Id, cancellationToken);
        var resources = await db.Resources
            .Where(x => x.Enabled && (x.FirewallHostId != null || x.PulseAgentId != null))
            .ToListAsync(cancellationToken);
        var desiredDomains = new HashSet<string>(StringComparer.OrdinalIgnoreCase);

        foreach (var resource in resources)
        {
            string? answer = null;
            if (resource.FirewallHostId is Guid hostId && hosts.TryGetValue(hostId, out var host))
            {
                answer = host.InternalTraefikIp;
            }
            else if (resource.PulseAgentId is Guid pulseId && pulseAgents.TryGetValue(pulseId, out var agent))
            {
                answer = agent.LastPrivateIp ?? agent.LastPublicIp;
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
                };
                db.AdGuardRewrites.Add(rewrite);
            }
            else if (!rewrite.ManagedByHashi)
            {
                continue;
            }

            rewrite.Answer = answer;
        }

        var staleManaged = await db.AdGuardRewrites
            .Where(x => x.ConnectionId == connectionId && x.ManagedByHashi)
            .ToListAsync(cancellationToken);
        foreach (var rewrite in staleManaged)
        {
            if (!desiredDomains.Contains(rewrite.Domain))
            {
                db.AdGuardRewrites.Remove(rewrite);
            }
        }

        await db.SaveChangesAsync(cancellationToken);
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
        var password = await ResolvePasswordAsync(connection.PasswordSecretId, cancellationToken);
        var client = httpClientFactory.CreateClient("adguard");
        client.BaseAddress = new Uri(connection.BaseUrl.TrimEnd('/') + "/");
        if (!string.IsNullOrEmpty(password))
        {
            client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue(
                "Basic",
                Convert.ToBase64String(Encoding.UTF8.GetBytes($"admin:{password}")));
        }

        return client;
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
