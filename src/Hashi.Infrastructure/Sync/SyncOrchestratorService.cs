using Hashi.Contracts.Api;
using Hashi.Core.Sync;
using Hashi.Core.Traefik;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;
using Microsoft.AspNetCore.Http;
using Npgsql;

namespace Hashi.Infrastructure.Sync;

public sealed class SyncRunService(HashiDbContext db)
{
    public async Task<SyncRunEntity> BeginRunAsync(string subsystem, CancellationToken cancellationToken = default)
    {
        var run = new SyncRunEntity
        {
            Subsystem = subsystem,
            Status = SyncRunStatusNames.Planning,
        };
        db.SyncRuns.Add(run);
        await db.SaveChangesAsync(cancellationToken);
        return run;
    }

    public async Task AddStepAsync(
        Guid runId,
        string name,
        string status,
        string? message,
        CancellationToken cancellationToken = default)
    {
        db.SyncSteps.Add(new SyncStepEntity
        {
            SyncRunId = runId,
            Name = name,
            Status = status,
            Message = message,
            CompletedAtUtc = status is SyncRunStatusNames.Succeeded or SyncRunStatusNames.Failed
                ? DateTimeOffset.UtcNow
                : null,
        });
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task AddDiffsAsync(
        Guid runId,
        IEnumerable<ProviderChange> changes,
        CancellationToken cancellationToken = default)
    {
        foreach (var change in changes)
        {
            db.SyncDiffs.Add(new SyncDiffEntity
            {
                SyncRunId = runId,
                ResourceType = change.ResourceType,
                ResourceKey = change.ResourceKey,
                ChangeKind = change.Kind.ToString(),
                Summary = change.Summary,
            });
        }

        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task CompleteRunAsync(
        Guid runId,
        string status,
        SyncRiskLevel riskLevel,
        string? errorSummary,
        CancellationToken cancellationToken = default)
    {
        var run = await db.SyncRuns.SingleAsync(x => x.Id == runId, cancellationToken);
        run.Status = status;
        run.RiskLevel = riskLevel.ToString();
        run.ErrorSummary = errorSummary;
        run.CompletedAtUtc = DateTimeOffset.UtcNow;
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task<IReadOnlyList<SyncRunResponse>> ListRecentAsync(int limit, CancellationToken cancellationToken = default)
    {
        var runs = await db.SyncRuns.AsNoTracking()
            .OrderByDescending(x => x.StartedAtUtc)
            .Take(limit)
            .Include(x => x.Steps)
            .Include(x => x.Diffs)
            .ToListAsync(cancellationToken);

        return runs.Select(ToResponse).ToList();
    }

    public async Task<SyncRunResponse?> GetAsync(Guid id, CancellationToken cancellationToken = default)
    {
        var run = await db.SyncRuns.AsNoTracking()
            .Include(x => x.Steps)
            .Include(x => x.Diffs)
            .SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        return run is null ? null : ToResponse(run);
    }

    private static SyncRunResponse ToResponse(SyncRunEntity run) => new(
        run.Id,
        run.Subsystem,
        run.Status,
        run.RiskLevel,
        run.StartedAtUtc,
        run.CompletedAtUtc,
        run.ErrorSummary,
        run.Steps.Select(s => new SyncStepResponse(s.Id, s.Name, s.Status, s.StartedAtUtc, s.CompletedAtUtc, s.Message)).ToList(),
        run.Diffs.Select(d => new SyncDiffResponse(d.Id, d.ResourceType, d.ResourceKey, d.ChangeKind, d.Summary)).ToList());
}

public sealed class SyncOrchestratorService(
    HashiDbContext db,
    DnsConnectionService dns,
    TraefikPlatformService traefik,
    TraefikSyncService traefikSync,
    FirewallApplyService firewall,
    AdGuardSyncService adguard,
    SyncRunService syncRuns,
    AppSettingsService settings,
    AuditService audit,
    IHttpContextAccessor? httpContextAccessor = null,
    SyncOrchestratorHostedService? syncHost = null,
    SyncApplyCoordinator? applyCoordinator = null)
{
    private readonly SyncApplyCoordinator _applyCoordinator = applyCoordinator ?? new SyncApplyCoordinator();

    public Task TriggerImmediateSyncAsync(CancellationToken cancellationToken = default)
    {
        cancellationToken.ThrowIfCancellationRequested();
        syncHost?.SignalImmediateSync();
        return Task.CompletedTask;
    }

    public async Task<SyncPlanPreviewResponse> PlanGlobalAsync(CancellationToken cancellationToken = default)
    {
        var run = await syncRuns.BeginRunAsync("global", cancellationToken);
        var changes = new List<ProviderChange>();
        var risk = SyncRiskLevel.None;

        await syncRuns.AddStepAsync(run.Id, "dns-plan", SyncRunStatusNames.Planning, null, cancellationToken);
        var dnsConnections = await db.Connections.AsNoTracking()
            .Where(x => x.Type == ConnectionTypeNames.DnsProvider && x.Enabled)
            .ToListAsync(cancellationToken);
        foreach (var connection in dnsConnections)
        {
            try
            {
                var plan = await dns.PlanSyncAsync(connection.Id, cancellationToken);
                changes.AddRange(plan.Changes.Select(c => new ProviderChange(
                    "dns",
                    $"{c.Name}/{c.Type}",
                    MapDnsKind(c.Kind),
                    c.RiskReason)));
                if (plan.RequiresConfirmation)
                {
                    risk = MaxRisk(risk, SyncRiskLevel.Destructive);
                }
                else if (plan.Changes.Any(x => x.Kind != Core.Dns.DnsChangeKind.NoOp))
                {
                    risk = MaxRisk(risk, SyncRiskLevel.Low);
                }

                await syncRuns.AddStepAsync(run.Id, $"dns-plan-{connection.Name}", SyncRunStatusNames.Succeeded, $"{plan.Changes.Count} changes", cancellationToken);
            }
            catch (Exception ex)
            {
                await syncRuns.AddStepAsync(run.Id, $"dns-plan-{connection.Name}", SyncRunStatusNames.Failed, ex.Message, cancellationToken);
            }
        }

        await syncRuns.AddStepAsync(run.Id, "traefik-plan", SyncRunStatusNames.Planning, null, cancellationToken);
        var render = await traefik.RenderAsync(cancellationToken);
        changes.Add(new ProviderChange("traefik", "dynamic-config", ProviderResultKind.Updated, $"Hash {render.ContentHash}"));
        await syncRuns.AddStepAsync(run.Id, "traefik-plan", SyncRunStatusNames.Succeeded, render.ContentHash, cancellationToken);

        var pendingEntrypointRemovals = await db.TraefikEntryPoints.AsNoTracking()
            .Where(x => x.PendingRemoval)
            .ToListAsync(cancellationToken);
        foreach (var entryPoint in pendingEntrypointRemovals)
        {
            changes.Add(new ProviderChange(
                "entrypoint-removal",
                $"{entryPoint.Protocol}/{entryPoint.Port}",
                ProviderResultKind.Deleted,
                "Requires confirmation to close port and remove firewall rule"));
            risk = MaxRisk(risk, SyncRiskLevel.Destructive);
        }

        await syncRuns.AddStepAsync(run.Id, "firewall-plan", SyncRunStatusNames.Planning, null, cancellationToken);
        var firewallHosts = await db.FirewallHosts.AsNoTracking().ToListAsync(cancellationToken);
        string? clientIp = null;
        if (httpContextAccessor?.HttpContext is not null)
        {
            var context = httpContextAccessor.HttpContext;
            if (context.Request.Headers.TryGetValue("X-Forwarded-For", out var forwardedFor))
            {
                clientIp = forwardedFor.FirstOrDefault()?.Split(',').FirstOrDefault()?.Trim();
            }
            if (string.IsNullOrEmpty(clientIp))
            {
                clientIp = context.Connection.RemoteIpAddress?.ToString();
            }
        }

        var blockedHosts = new List<string>();
        foreach (var host in firewallHosts)
        {
            var (_, hash) = await firewall.RenderForHostAsync(host.Id, cancellationToken);
            var changed = !string.Equals(host.LastAppliedScriptHash, hash, StringComparison.Ordinal);
            changes.Add(new ProviderChange(
                "firewall",
                host.Name,
                changed ? ProviderResultKind.Updated : ProviderResultKind.NoOp,
                changed ? $"Script hash {hash}" : "Unchanged"));
            if (changed)
            {
                risk = MaxRisk(risk, SyncRiskLevel.Low);
            }

            if (!string.IsNullOrEmpty(clientIp))
            {
                var definition = await firewall.BuildHostDefinitionAsync(host, cancellationToken);
                if (!FirewallApplyService.IsIpAllowed(clientIp, definition))
                {
                    blockedHosts.Add(host.Name);
                }
            }
        }

        if (blockedHosts.Count > 0)
        {
            risk = MaxRisk(risk, SyncRiskLevel.Destructive);
        }

        await syncRuns.AddStepAsync(run.Id, "firewall-plan", SyncRunStatusNames.Succeeded, $"{firewallHosts.Count} hosts", cancellationToken);

        await syncRuns.AddStepAsync(run.Id, "adguard-plan", SyncRunStatusNames.Planning, null, cancellationToken);
        var adguardConnections = await db.AdGuardConnections.AsNoTracking()
            .Where(x => x.Enabled)
            .ToListAsync(cancellationToken);
        foreach (var connection in adguardConnections)
        {
            try
            {
                var plan = await adguard.PlanSyncAsync(
                    connection.Id,
                    updateTopologyDesiredState: true,
                    updateInternalAgentDnsDesiredState: true,
                    cancellationToken: cancellationToken);
                changes.AddRange(plan.Changes.Select(c => new ProviderChange(
                    "adguard-rewrite",
                    c.Domain,
                    c.Kind switch
                    {
                        "create" => ProviderResultKind.Created,
                        "update" => ProviderResultKind.Updated,
                        "delete" => ProviderResultKind.Deleted,
                        _ => ProviderResultKind.NoOp,
                    },
                    c.Summary)));
                if (plan.RequiresConfirmation)
                {
                    risk = MaxRisk(risk, SyncRiskLevel.Destructive);
                }
                else if (plan.Changes.Count > 0)
                {
                    risk = MaxRisk(risk, SyncRiskLevel.Low);
                }

                await syncRuns.AddStepAsync(run.Id, $"adguard-plan-{connection.Name}", SyncRunStatusNames.Succeeded, $"{plan.Changes.Count} changes", cancellationToken);
            }
            catch (Exception ex)
            {
                await syncRuns.AddStepAsync(run.Id, $"adguard-plan-{connection.Name}", SyncRunStatusNames.Failed, ex.Message, cancellationToken);
            }
        }

        await syncRuns.AddDiffsAsync(run.Id, changes, cancellationToken);

        var validationErrors = new List<string>();
        await syncRuns.AddStepAsync(run.Id, "validate", SyncRunStatusNames.Planning, null, cancellationToken);
        try
        {
            var traefikRender = await traefik.RenderAsync(cancellationToken);
            var traefikValidation = TraefikConfigValidator.ValidateRender(traefikRender);
            if (!traefikValidation.IsValid)
            {
                validationErrors.AddRange(traefikValidation.Errors.Select(e => $"Traefik: {e}"));
            }

            foreach (var host in firewallHosts)
            {
                var (_, hash) = await firewall.RenderForHostAsync(host.Id, cancellationToken);
                if (string.IsNullOrWhiteSpace(hash))
                {
                    validationErrors.Add($"Firewall host {host.Name}: rendered script hash is empty.");
                }
            }

            foreach (var connection in dnsConnections)
            {
                try
                {
                    var plan = await dns.PlanSyncAsync(connection.Id, cancellationToken);
                    if (plan.Changes.Count == 0 && plan.RequiresConfirmation)
                    {
                        validationErrors.Add($"DNS connection {connection.Name}: plan reports confirmation required but has no changes.");
                    }
                }
                catch (Exception ex)
                {
                    validationErrors.Add($"DNS connection {connection.Name}: plan validation failed — {ex.Message}");
                }
            }

            foreach (var connection in adguardConnections)
            {
                try
                {
                    var plan = await adguard.PlanSyncAsync(connection.Id, updateTopologyDesiredState: true, updateInternalAgentDnsDesiredState: true, cancellationToken: cancellationToken);
                    if (plan.Changes.Count == 0 && plan.RequiresConfirmation)
                    {
                        validationErrors.Add($"AdGuard connection {connection.Name}: plan reports confirmation required but has no changes.");
                    }
                }
                catch (Exception ex)
                {
                    validationErrors.Add($"AdGuard connection {connection.Name}: plan validation failed — {ex.Message}");
                }
            }
        }
        catch (Exception ex)
        {
            validationErrors.Add($"Validation step failed: {ex.Message}");
        }

        var hasValidationErrors = validationErrors.Count > 0;
        await syncRuns.AddStepAsync(
            run.Id,
            "validate",
            hasValidationErrors ? SyncRunStatusNames.Failed : SyncRunStatusNames.Succeeded,
            hasValidationErrors ? $"{validationErrors.Count} validation error(s)" : null,
            cancellationToken);

        if (hasValidationErrors)
        {
            risk = MaxRisk(risk, SyncRiskLevel.High);
        }

        var requiresConfirmation = risk >= SyncRiskLevel.High;
        await syncRuns.CompleteRunAsync(
            run.Id,
            hasValidationErrors
                ? SyncRunStatusNames.Failed
                : requiresConfirmation
                    ? SyncRunStatusNames.AwaitingConfirmation
                    : SyncRunStatusNames.Succeeded,
            risk,
            hasValidationErrors ? string.Join("; ", validationErrors) : null,
            cancellationToken);

        var previewMarkdown = BuildPreviewMarkdown(changes);
        if (blockedHosts.Count > 0 && !string.IsNullOrEmpty(clientIp))
        {
            var alert = $"> [!WARNING]\n> Applying this global configuration will block your current SSH connection (IP: {clientIp}) on the following host(s): **{string.Join(", ", blockedHosts)}**. Please check allowed subnets/IPs or configure NetBird.\n\n";
            previewMarkdown = alert + previewMarkdown;
        }

        return new SyncPlanPreviewResponse(
            run.Id,
            "global",
            risk.ToString(),
            requiresConfirmation,
            changes.Select(c => new SyncDiffResponse(Guid.Empty, c.ResourceType, c.ResourceKey, c.Kind.ToString(), c.Summary)).ToList(),
            previewMarkdown,
            validationErrors.Count > 0 ? validationErrors : null);
    }

    public async Task<SyncApplyResponse> ApplyGlobalAsync(
        Guid approvedPlanId,
        bool confirmDestructive,
        CancellationToken cancellationToken = default)
    {
        if (!await _applyCoordinator.ApplyLock.WaitAsync(0, cancellationToken))
        {
            return new SyncApplyResponse(Guid.Empty, false, SyncRunStatusNames.Failed, "Another apply is already in progress. Concurrent applies are rejected.");
        }

        SyncRunEntity? run = null;
        try
        {
            await using var databaseLock = await PostgresAdvisoryLock.TryAcquireAsync(db, cancellationToken);
            if (databaseLock is null)
            {
                return new SyncApplyResponse(Guid.Empty, false, SyncRunStatusNames.Failed, "Another apply is already in progress. Concurrent applies are rejected.");
            }

            var approvedPlan = await db.SyncRuns
                .AsNoTracking()
                .Include(x => x.Diffs)
                .SingleOrDefaultAsync(x => x.Id == approvedPlanId && x.Subsystem == "global", cancellationToken);
            if (approvedPlan is null || approvedPlan.Status == SyncRunStatusNames.Failed)
            {
                return new SyncApplyResponse(Guid.Empty, false, SyncRunStatusNames.Failed, "The approved sync plan was not found or failed validation. Create a new plan before applying.");
            }

            var currentPreview = await PlanGlobalAsync(cancellationToken);
            var currentPlan = await db.SyncRuns
                .AsNoTracking()
                .Include(x => x.Diffs)
                .SingleAsync(x => x.Id == currentPreview.PlanId, cancellationToken);
            if (!PlansMatch(approvedPlan, currentPlan))
            {
                return new SyncApplyResponse(Guid.Empty, false, SyncRunStatusNames.Failed, "The approved sync plan is stale. Preview the current changes and approve the new plan before applying.");
            }

            run = await syncRuns.BeginRunAsync("global-apply", cancellationToken);
            var applyFailures = new List<string>();
            string? clientIp = null;
            if (httpContextAccessor?.HttpContext is not null)
            {
                var context = httpContextAccessor.HttpContext;
                if (context.Request.Headers.TryGetValue("X-Forwarded-For", out var forwardedFor))
                {
                    clientIp = forwardedFor.FirstOrDefault()?.Split(',').FirstOrDefault()?.Trim();
                }
                if (string.IsNullOrEmpty(clientIp))
                {
                    clientIp = context.Connection.RemoteIpAddress?.ToString();
                }
            }

            if (!string.IsNullOrEmpty(clientIp))
            {
                var allFirewallHosts = await db.FirewallHosts.AsNoTracking().ToListAsync(cancellationToken);
                var blockedHosts = new List<string>();
                foreach (var host in allFirewallHosts)
                {
                    var definition = await firewall.BuildHostDefinitionAsync(host, cancellationToken);
                    if (!FirewallApplyService.IsIpAllowed(clientIp, definition))
                    {
                        blockedHosts.Add(host.Name);
                    }
                }

                if (blockedHosts.Count > 0 && !confirmDestructive)
                {
                    var msg = $"Applying this global configuration would block your current SSH connection (IP: {clientIp}) on host(s): {string.Join(", ", blockedHosts)}. Confirm destructive changes to override.";
                    await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.AwaitingConfirmation, SyncRiskLevel.Destructive, msg, cancellationToken);
                    return new SyncApplyResponse(run.Id, false, SyncRunStatusNames.AwaitingConfirmation, msg);
                }
            }
            if (confirmDestructive)
            {
                var pendingRemovals = await db.TraefikEntryPoints
                    .Where(x => x.PendingRemoval)
                    .ToListAsync(cancellationToken);
                if (pendingRemovals.Count > 0)
                {
                    db.TraefikEntryPoints.RemoveRange(pendingRemovals);
                    await db.SaveChangesAsync(cancellationToken);
                }
            }

            await syncRuns.AddStepAsync(run.Id, "pre-apply-validate", SyncRunStatusNames.Planning, null, cancellationToken);
            var preRender = await traefik.RenderAsync(cancellationToken);
            var preValidation = TraefikConfigValidator.ValidateRender(preRender);
            if (!preValidation.IsValid)
            {
                var validationError = $"Plan has validation errors: {string.Join("; ", preValidation.Errors)}";
                await syncRuns.AddStepAsync(run.Id, "pre-apply-validate", SyncRunStatusNames.Failed, validationError, cancellationToken);
                await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Failed, SyncRiskLevel.High, validationError, cancellationToken);
                return new SyncApplyResponse(run.Id, false, SyncRunStatusNames.Failed, validationError);
            }
            await syncRuns.AddStepAsync(run.Id, "pre-apply-validate", SyncRunStatusNames.Succeeded, "Valid", cancellationToken);

            var dnsConnections = await db.Connections
                .Where(x => x.Type == ConnectionTypeNames.DnsProvider && x.Enabled)
                .ToListAsync(cancellationToken);
            foreach (var connection in dnsConnections)
            {
                await syncRuns.AddStepAsync(run.Id, $"dns-apply-{connection.Name}", SyncRunStatusNames.Applying, null, cancellationToken);
                var plan = await dns.PlanSyncAsync(connection.Id, cancellationToken);
                if (plan.RequiresConfirmation && !confirmDestructive)
                {
                    var safeChanges = plan.Changes.Where(x => x.Kind != Core.Dns.DnsChangeKind.NoOp && x.Kind != Core.Dns.DnsChangeKind.Delete).ToList();
                    if (safeChanges.Count > 0)
                    {
                        await dns.ApplySafePlanAsync(plan, cancellationToken);
                        await syncRuns.AddStepAsync(run.Id, $"dns-apply-{connection.Name}", SyncRunStatusNames.Succeeded, $"Applied {safeChanges.Count} safe changes; destructive pending.", cancellationToken);
                        continue;
                    }
                    await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.AwaitingConfirmation, SyncRiskLevel.Destructive, "Destructive DNS changes require confirmation.", cancellationToken);
                    return new SyncApplyResponse(run.Id, false, SyncRunStatusNames.AwaitingConfirmation, "Destructive DNS changes require confirmation.");
                }

                await dns.ApplyPlanAsync(plan, confirmDestructive, cancellationToken);
                await syncRuns.AddStepAsync(run.Id, $"dns-apply-{connection.Name}", SyncRunStatusNames.Succeeded, "Applied", cancellationToken);
            }

            await syncRuns.AddStepAsync(run.Id, "traefik-apply", SyncRunStatusNames.Applying, null, cancellationToken);
            var traefikConnections = await db.Connections
                .Where(x => x.Type == ConnectionTypeNames.TraefikHost && x.Enabled)
                .ToListAsync(cancellationToken);
            foreach (var connection in traefikConnections)
            {
                try
                {
                    var result = await traefikSync.ApplyForConnectionInternalAsync(connection.Id, cancellationToken);
                    await syncRuns.AddStepAsync(
                        run.Id,
                        $"traefik-apply-{connection.Name}",
                        result.Succeeded ? SyncRunStatusNames.Succeeded : SyncRunStatusNames.Failed,
                        result.Message ?? result.ContentHash,
                        cancellationToken);
                    if (!result.Succeeded)
                    {
                        applyFailures.Add($"Traefik {connection.Name}: {result.Message ?? "apply failed"}");
                    }
                }
                catch (Exception ex)
                {
                    await syncRuns.AddStepAsync(run.Id, $"traefik-apply-{connection.Name}", SyncRunStatusNames.Failed, ex.Message, cancellationToken);
                    applyFailures.Add($"Traefik {connection.Name}: {ex.Message}");
                }
            }

            if (traefikConnections.Count == 0)
            {
                var render = await traefik.RenderAsync(cancellationToken);
                await syncRuns.AddStepAsync(run.Id, "traefik-apply", SyncRunStatusNames.Succeeded, $"Rendered locally; hash {render.ContentHash}", cancellationToken);
            }
            else
            {
                await syncRuns.AddStepAsync(run.Id, "traefik-apply", SyncRunStatusNames.Succeeded, "Applied", cancellationToken);
            }

            await syncRuns.AddStepAsync(run.Id, "firewall-apply", SyncRunStatusNames.Applying, null, cancellationToken);
            var firewallHosts = await db.FirewallHosts.ToListAsync(cancellationToken);
            foreach (var host in firewallHosts)
            {
                try
                {
                    var (_, hash) = await firewall.RenderForHostAsync(host.Id, cancellationToken);
                    if (string.Equals(host.LastAppliedScriptHash, hash, StringComparison.Ordinal))
                    {
                        await syncRuns.AddStepAsync(run.Id, $"firewall-apply-{host.Name}", SyncRunStatusNames.Succeeded, "Skipped: unchanged", cancellationToken);
                        continue;
                    }

                    var result = await firewall.ApplyForHostAsync(host.Id, cancellationToken);
                    await syncRuns.AddStepAsync(
                        run.Id,
                        $"firewall-apply-{host.Name}",
                        result.Succeeded ? SyncRunStatusNames.Succeeded : SyncRunStatusNames.Failed,
                        result.Message,
                        cancellationToken);
                    if (!result.Succeeded)
                    {
                        applyFailures.Add($"Firewall {host.Name}: {result.Message ?? "apply failed"}");
                    }
                }
                catch (Exception ex)
                {
                    await syncRuns.AddStepAsync(run.Id, $"firewall-apply-{host.Name}", SyncRunStatusNames.Failed, ex.Message, cancellationToken);
                    applyFailures.Add($"Firewall {host.Name}: {ex.Message}");
                }
            }

            await syncRuns.AddStepAsync(run.Id, "firewall-apply", SyncRunStatusNames.Succeeded, $"{firewallHosts.Count} hosts processed", cancellationToken);

            await syncRuns.AddStepAsync(run.Id, "adguard-sync", SyncRunStatusNames.Applying, null, cancellationToken);
            var adguardConnections = await db.AdGuardConnections.ToListAsync(cancellationToken);
            foreach (var connection in adguardConnections)
            {
                var plan = await adguard.PlanSyncAsync(
                    connection.Id,
                    updateTopologyDesiredState: true,
                    updateInternalAgentDnsDesiredState: true,
                    cancellationToken: cancellationToken);
                if (plan.RequiresConfirmation && !confirmDestructive)
                {
                    var nonDestructiveChanges = plan.Changes.Where(x => x.Kind != "delete").ToList();
                    if (nonDestructiveChanges.Count > 0)
                    {
                        await adguard.ApplySafePlanAsync(
                            connection.Id,
                            plan.PlanId,
                            updateTopologyDesiredState: true,
                            updateInternalAgentDnsDesiredState: true,
                            cancellationToken: cancellationToken);
                        await syncRuns.AddStepAsync(run.Id, $"adguard-sync-{connection.Name}", SyncRunStatusNames.Succeeded, $"Applied {nonDestructiveChanges.Count} safe changes; destructive pending.", cancellationToken);
                        continue;
                    }
                    await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.AwaitingConfirmation, SyncRiskLevel.Destructive, "Destructive AdGuard changes require confirmation.", cancellationToken);
                    return new SyncApplyResponse(run.Id, false, SyncRunStatusNames.AwaitingConfirmation, "Destructive AdGuard changes require confirmation.");
                }

                var result = await adguard.ApplyPlanAsync(
                    connection.Id,
                    new AdGuardRewriteApplyRequest(plan.PlanId, confirmDestructive),
                    updateTopologyDesiredState: true,
                    updateInternalAgentDnsDesiredState: true,
                    cancellationToken: cancellationToken);
                if (!result.Succeeded)
                {
                    await syncRuns.AddStepAsync(run.Id, $"adguard-sync-{connection.Name}", SyncRunStatusNames.Failed, result.Message, cancellationToken);
                    throw new InvalidOperationException(result.Message ?? "AdGuard sync failed.");
                }
            }

            await syncRuns.AddStepAsync(run.Id, "adguard-sync", SyncRunStatusNames.Succeeded, "Applied", cancellationToken);

            if (applyFailures.Count > 0)
            {
                throw new InvalidOperationException(string.Join("; ", applyFailures));
            }

            await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Succeeded, SyncRiskLevel.Low, null, cancellationToken);
            await audit.WriteAsync("sync", "global_apply", subjectType: "sync_run", subjectId: run.Id.ToString(), cancellationToken: cancellationToken);
            return new SyncApplyResponse(run.Id, true, SyncRunStatusNames.Succeeded, null);
        }
        catch (Exception ex)
        {
            if (run is not null)
            {
                await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Failed, SyncRiskLevel.High, ex.Message, cancellationToken);
            }
            return new SyncApplyResponse(run?.Id ?? Guid.Empty, false, SyncRunStatusNames.Failed, ex.Message);
        }
        finally
        {
            _applyCoordinator.ApplyLock.Release();
        }
    }

    private static bool PlansMatch(SyncRunEntity approved, SyncRunEntity current)
    {
        if (!string.Equals(approved.RiskLevel, current.RiskLevel, StringComparison.Ordinal)
            || approved.Diffs.Count != current.Diffs.Count)
        {
            return false;
        }

        static string Key(SyncDiffEntity diff)
            => string.Join('\u001f', diff.ResourceType, diff.ResourceKey, diff.ChangeKind, diff.Summary ?? string.Empty);

        return approved.Diffs.Select(Key).OrderBy(x => x, StringComparer.Ordinal)
            .SequenceEqual(current.Diffs.Select(Key).OrderBy(x => x, StringComparer.Ordinal), StringComparer.Ordinal);
    }

    public async Task<SyncReconcileResponse> ReconcileAsync(CancellationToken cancellationToken = default)
    {
        if (!await _applyCoordinator.ApplyLock.WaitAsync(0, cancellationToken))
        {
            return new SyncReconcileResponse(Guid.Empty, false, []);
        }

        try
        {
            await using var databaseLock = await PostgresAdvisoryLock.TryAcquireAsync(db, cancellationToken);
            if (databaseLock is null)
            {
                return new SyncReconcileResponse(Guid.Empty, false, []);
            }

            _ = await settings.GetOrCreateAsync(cancellationToken);
            var run = await syncRuns.BeginRunAsync("reconcile", cancellationToken);
            var subsystems = new List<string>();
            var hasPendingDestructive = false;
            var hasFailures = false;
            try
            {
                subsystems.Add("dns");
                await syncRuns.AddStepAsync(run.Id, "dns-reconcile", SyncRunStatusNames.Reconciling, null, cancellationToken);
                var dnsConnections = await db.Connections.Where(x => x.Type == ConnectionTypeNames.DnsProvider && x.Enabled).ToListAsync(cancellationToken);
                foreach (var connection in dnsConnections)
                {
                    try
                    {
                        var plan = await dns.PlanSyncAsync(connection.Id, cancellationToken);
                        var dnsChanges = plan.Changes
                            .Where(x => x.Kind != Core.Dns.DnsChangeKind.NoOp)
                            .Select(MapDnsPlanChange)
                            .ToList();
                        if (dnsChanges.Count > 0)
                        {
                            await syncRuns.AddDiffsAsync(run.Id, dnsChanges, cancellationToken);
                        }

                        if (plan.RequiresConfirmation)
                        {
                            hasPendingDestructive = true;
                            await dns.ApplySafePlanAsync(plan, cancellationToken);
                            await syncRuns.AddStepAsync(
                                run.Id,
                                $"dns-reconcile-{connection.Name}",
                                SyncRunStatusNames.Succeeded,
                                "Safe changes applied; destructive changes pending confirmation.",
                                cancellationToken);
                        }
                        else if (plan.Changes.Any(x => x.Kind != Core.Dns.DnsChangeKind.NoOp))
                        {
                            await dns.ApplyPlanAsync(plan, confirmDestructive: true, cancellationToken);
                            await syncRuns.AddStepAsync(run.Id, $"dns-reconcile-{connection.Name}", SyncRunStatusNames.Succeeded, "Applied", cancellationToken);
                        }
                        else
                        {
                            await syncRuns.AddStepAsync(run.Id, $"dns-reconcile-{connection.Name}", SyncRunStatusNames.Succeeded, "No changes", cancellationToken);
                        }
                    }
                    catch (Exception ex)
                    {
                        await syncRuns.AddStepAsync(run.Id, $"dns-reconcile-{connection.Name}", SyncRunStatusNames.Failed, ex.Message, cancellationToken);
                        hasFailures = true;
                    }
                }

                await syncRuns.AddStepAsync(run.Id, "dns-reconcile", SyncRunStatusNames.Succeeded, null, cancellationToken);

                subsystems.Add("traefik");
                await syncRuns.AddStepAsync(run.Id, "traefik-reconcile", SyncRunStatusNames.Reconciling, null, cancellationToken);
                var traefikConnections = await db.Connections.Where(x => x.Type == ConnectionTypeNames.TraefikHost && x.Enabled).ToListAsync(cancellationToken);
                foreach (var connection in traefikConnections)
                {
                    try
                    {
                        var result = await traefikSync.ApplyForConnectionInternalAsync(connection.Id, cancellationToken);
                        if (!result.Succeeded)
                        {
                            hasFailures = true;
                            await syncRuns.AddStepAsync(
                                run.Id,
                                $"traefik-reconcile-{connection.Name}",
                                SyncRunStatusNames.Failed,
                                result.Message ?? "Traefik apply failed.",
                                cancellationToken);
                        }
                    }
                    catch (Exception ex)
                    {
                        hasFailures = true;
                        await syncRuns.AddStepAsync(run.Id, $"traefik-reconcile-{connection.Name}", SyncRunStatusNames.Failed, ex.Message, cancellationToken);
                    }
                }

                if (traefikConnections.Count == 0)
                {
                    await traefik.RenderAsync(cancellationToken);
                }

                await syncRuns.AddStepAsync(run.Id, "traefik-reconcile", SyncRunStatusNames.Succeeded, null, cancellationToken);

                subsystems.Add("adguard");
                var adguardConnections = await db.AdGuardConnections.ToListAsync(cancellationToken);
                foreach (var connection in adguardConnections)
                {
                    var plan = await adguard.PlanSyncAsync(
                        connection.Id,
                        updateTopologyDesiredState: true,
                        updateInternalAgentDnsDesiredState: true,
                        cancellationToken: cancellationToken);
                    var adguardChanges = plan.Changes
                        .Select(x => new ProviderChange("adguard-rewrite", x.Domain, MapAdGuardKind(x.Kind), x.Summary))
                        .ToList();
                    if (adguardChanges.Count > 0)
                    {
                        await syncRuns.AddDiffsAsync(run.Id, adguardChanges, cancellationToken);
                    }

                    if (plan.RequiresConfirmation)
                    {
                        hasPendingDestructive = true;
                        var result = await adguard.ApplySafePlanAsync(
                            connection.Id,
                            plan.PlanId,
                            updateTopologyDesiredState: true,
                            updateInternalAgentDnsDesiredState: true,
                            cancellationToken: cancellationToken);
                        if (result.Succeeded)
                        {
                            await syncRuns.AddStepAsync(
                                run.Id,
                                $"adguard-reconcile-{connection.Name}",
                                SyncRunStatusNames.Succeeded,
                                "Destructive changes pending confirmation.",
                                cancellationToken);
                        }
                        else
                        {
                            hasFailures = true;
                            await syncRuns.AddStepAsync(
                                run.Id,
                                $"adguard-reconcile-{connection.Name}",
                                SyncRunStatusNames.Failed,
                                result.Message ?? "AdGuard safe apply failed.",
                                cancellationToken);
                        }
                    }
                    else if (plan.Changes.Count > 0)
                    {
                        var result = await adguard.ApplyPlanAsync(
                            connection.Id,
                            new AdGuardRewriteApplyRequest(plan.PlanId, ConfirmDestructive: false),
                            updateTopologyDesiredState: true,
                            updateInternalAgentDnsDesiredState: true,
                            cancellationToken: cancellationToken);
                        if (result.Succeeded)
                        {
                            await syncRuns.AddStepAsync(run.Id, $"adguard-reconcile-{connection.Name}", SyncRunStatusNames.Succeeded, "Applied", cancellationToken);
                        }
                        else
                        {
                            hasFailures = true;
                            await syncRuns.AddStepAsync(
                                run.Id,
                                $"adguard-reconcile-{connection.Name}",
                                SyncRunStatusNames.Failed,
                                result.Message ?? "AdGuard apply failed.",
                                cancellationToken);
                        }
                    }
                    else
                    {
                        await syncRuns.AddStepAsync(run.Id, $"adguard-reconcile-{connection.Name}", SyncRunStatusNames.Succeeded, "No changes", cancellationToken);
                    }
                }

                if (await db.FirewallHosts.AnyAsync(cancellationToken))
                {
                    subsystems.Add("firewall");
                    await syncRuns.AddStepAsync(run.Id, "firewall-reconcile", SyncRunStatusNames.Reconciling, null, cancellationToken);
                    var hosts = await db.FirewallHosts.ToListAsync(cancellationToken);
                    foreach (var host in hosts)
                    {
                        try
                        {
                            var result = await firewall.ApplyForHostAsync(host.Id, cancellationToken);
                            if (!result.Succeeded)
                            {
                                hasFailures = true;
                                await syncRuns.AddStepAsync(
                                    run.Id,
                                    $"firewall-reconcile-{host.Name}",
                                    SyncRunStatusNames.Failed,
                                    result.Message ?? "Firewall apply failed.",
                                    cancellationToken);
                            }
                        }
                        catch (Exception ex)
                        {
                            hasFailures = true;
                            await syncRuns.AddStepAsync(run.Id, $"firewall-reconcile-{host.Name}", SyncRunStatusNames.Failed, ex.Message, cancellationToken);
                        }
                    }

                    await syncRuns.AddStepAsync(run.Id, "firewall-reconcile", SyncRunStatusNames.Succeeded, null, cancellationToken);
                }

                await syncRuns.CompleteRunAsync(
                    run.Id,
                    hasFailures
                        ? SyncRunStatusNames.Failed
                        : hasPendingDestructive
                            ? SyncRunStatusNames.AwaitingConfirmation
                            : SyncRunStatusNames.Succeeded,
                    hasFailures ? SyncRiskLevel.High : hasPendingDestructive ? SyncRiskLevel.Destructive : SyncRiskLevel.None,
                    hasFailures
                        ? "One or more subsystems failed to reconcile."
                        : hasPendingDestructive
                            ? "Destructive changes require confirmation."
                            : null,
                    cancellationToken);
                return new SyncReconcileResponse(run.Id, !hasFailures, subsystems);
            }
            catch (Exception ex)
            {
                await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Failed, SyncRiskLevel.Medium, ex.Message, cancellationToken);
                return new SyncReconcileResponse(run.Id, false, subsystems);
            }
        }
        finally
        {
            _applyCoordinator.ApplyLock.Release();
        }
    }

    private static ProviderChange MapDnsPlanChange(Core.Dns.DnsPlanChange change) => new(
        "dns",
        $"{change.Name}/{Core.Dns.DnsRecordTypeMapping.ToApiName(change.Type)}",
        MapDnsKind(change.Kind),
        change.RiskReason ?? change.Kind.ToString());

    private static SyncRiskLevel MaxRisk(SyncRiskLevel current, SyncRiskLevel next)
        => (SyncRiskLevel)Math.Max((int)current, (int)next);

    private static ProviderResultKind MapDnsKind(Core.Dns.DnsChangeKind kind) => kind switch
    {
        Core.Dns.DnsChangeKind.Create => ProviderResultKind.Created,
        Core.Dns.DnsChangeKind.Update => ProviderResultKind.Updated,
        Core.Dns.DnsChangeKind.Delete => ProviderResultKind.Deleted,
        _ => ProviderResultKind.NoOp,
    };

    private static ProviderResultKind MapAdGuardKind(string kind) => kind switch
    {
        "create" => ProviderResultKind.Created,
        "update" => ProviderResultKind.Updated,
        "delete" => ProviderResultKind.Deleted,
        _ => ProviderResultKind.NoOp,
    };

    private static string? BuildPreviewMarkdown(IReadOnlyList<ProviderChange> changes)
    {
        if (changes.Count == 0)
        {
            return "No changes planned.";
        }

        return string.Join('\n', changes.Select(c => $"- **{c.ResourceType}** `{c.ResourceKey}`: {c.Kind} — {c.Summary}"));
    }
}

public sealed class SyncApplyCoordinator
{
    internal SemaphoreSlim ApplyLock { get; } = new(1, 1);
}

internal sealed class PostgresAdvisoryLock : IAsyncDisposable
{
    // Stable application-wide lock key. A global lock is stricter than per-provider locking
    // and prevents apply/reconcile races across multiple Hashi instances.
    private const long LockKey = 0x484153484953594E;
    private readonly NpgsqlConnection _connection;

    private PostgresAdvisoryLock(NpgsqlConnection connection)
    {
        _connection = connection;
    }

    public static async Task<PostgresAdvisoryLock?> TryAcquireAsync(
        HashiDbContext db,
        CancellationToken cancellationToken)
    {
        if (!string.Equals(
                db.Database.ProviderName,
                "Npgsql.EntityFrameworkCore.PostgreSQL",
                StringComparison.Ordinal))
        {
            return new PostgresAdvisoryLock(new NpgsqlConnection());
        }

        var connectionString = db.Database.GetConnectionString()
            ?? throw new InvalidOperationException("PostgreSQL connection string is unavailable for sync locking.");
        var connection = new NpgsqlConnection(connectionString);
        await connection.OpenAsync(cancellationToken);

        await using var command = new NpgsqlCommand("SELECT pg_try_advisory_lock(@key)", connection);
        command.Parameters.AddWithValue("key", LockKey);
        var acquired = (bool)(await command.ExecuteScalarAsync(cancellationToken) ?? false);
        if (!acquired)
        {
            await connection.DisposeAsync();
            return null;
        }

        return new PostgresAdvisoryLock(connection);
    }

    public async ValueTask DisposeAsync()
    {
        if (_connection.State != System.Data.ConnectionState.Open)
        {
            await _connection.DisposeAsync();
            return;
        }

        await using var command = new NpgsqlCommand("SELECT pg_advisory_unlock(@key)", _connection);
        command.Parameters.AddWithValue("key", LockKey);
        await command.ExecuteScalarAsync();
        await _connection.DisposeAsync();
    }
}

public sealed class SyncOrchestratorHostedService(
    IServiceScopeFactory scopeFactory,
    ILogger<SyncOrchestratorHostedService> logger) : BackgroundService
{
    private volatile TaskCompletionSource _immediateSyncSignal = new(TaskCreationOptions.RunContinuationsAsynchronously);

    public void SignalImmediateSync()
    {
        _immediateSyncSignal.TrySetResult();
    }

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        await Task.Delay(TimeSpan.FromMinutes(1), stoppingToken);
        while (!stoppingToken.IsCancellationRequested)
        {
            var intervalMinutes = 60;
            try
            {
                using var scope = scopeFactory.CreateScope();
                var settings = scope.ServiceProvider.GetRequiredService<AppSettingsService>();
                var orchestrator = scope.ServiceProvider.GetRequiredService<SyncOrchestratorService>();
                var jobs = scope.ServiceProvider.GetRequiredService<BackgroundJobService>();
                var general = await settings.GetOrCreateAsync(stoppingToken);
                intervalMinutes = Math.Max(5, general.DefaultSyncIntervalMinutes);

                var session = scope.ServiceProvider.GetRequiredService<VaultSessionState>();
                var serviceSync = scope.ServiceProvider.GetRequiredService<ServiceSyncVaultState>();
                var vaultService = scope.ServiceProvider.GetRequiredService<VaultService>();

                var available = session.IsUnlocked || serviceSync.IsUnlocked;
                if (!available)
                {
                    await vaultService.EnsureServiceSyncWrapAsync(stoppingToken);
                    available = session.IsUnlocked || serviceSync.IsUnlocked;
                }

                if (!available)
                {
                    logger.LogWarning("Service-sync vault is unavailable (locked). Sync reconcile is paused gracefully.");
                    await jobs.BeginRunAsync(BackgroundJobKeys.SyncReconcile, stoppingToken);
                    await jobs.CompleteRunAsync(
                        BackgroundJobKeys.SyncReconcile,
                        true,
                        "Paused: Vault is locked.",
                        "Vault is locked. Background sync will retry.",
                        intervalMinutes * 60,
                        stoppingToken);

                    Interlocked.Exchange(ref _immediateSyncSignal, new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously));
                    var pauseSyncTask = Task.Delay(TimeSpan.FromMinutes(intervalMinutes), stoppingToken);
                    var pauseSignalTask = _immediateSyncSignal.Task;
                    await Task.WhenAny(pauseSyncTask, pauseSignalTask);
                    continue;
                }

                await jobs.BeginRunAsync(BackgroundJobKeys.SyncReconcile, stoppingToken);
                logger.LogInformation("Starting passive sync reconcile.");
                var result = await orchestrator.ReconcileAsync(stoppingToken);
                logger.LogInformation("Passive sync reconcile completed.");
                await jobs.CompleteRunAsync(
                    BackgroundJobKeys.SyncReconcile,
                    result.Succeeded,
                    $"Reconciled: {string.Join(", ", result.SubsystemsReconciled)}",
                    result.Succeeded ? null : "Reconcile finished with errors.",
                    intervalMinutes * 60,
                    stoppingToken);
                Interlocked.Exchange(ref _immediateSyncSignal, new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously));
                var syncTask = Task.Delay(TimeSpan.FromMinutes(intervalMinutes), stoppingToken);
                var signalTask = _immediateSyncSignal.Task;
                await Task.WhenAny(syncTask, signalTask);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "Passive sync orchestrator failed.");
                try
                {
                    using var scope = scopeFactory.CreateScope();
                    var jobs = scope.ServiceProvider.GetRequiredService<BackgroundJobService>();
                    await jobs.CompleteRunAsync(
                        BackgroundJobKeys.SyncReconcile,
                        false,
                        null,
                        ex.Message,
                        intervalMinutes * 60,
                        stoppingToken);
                }
                catch
                {
                    // Best effort job status update.
                }

                await Task.Delay(TimeSpan.FromMinutes(5), stoppingToken);
            }
        }
    }
}
