using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using Hashi.Core.Auth;
using Hashi.Core.Dns;
using Hashi.Core.Sync;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Providers.Dns;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Dns;

public sealed class DnsConnectionService(
    HashiDbContext db,
    IDnsProviderFactory providerFactory,
    SecretRecordService secrets,
    AuditService audit)
{
    public async Task<ConnectionEntity> CreateHetznerConnectionAsync(
        string name,
        string apiToken,
        string zoneName,
        int defaultTtl,
        CancellationToken cancellationToken = default)
    {
        var provider = providerFactory.Create(DnsProviderTypeNames.Hetzner, apiToken);
        var zones = await provider.ListZonesAsync(cancellationToken);
        var zone = zones.SingleOrDefault(x => string.Equals(x.Name, zoneName, StringComparison.OrdinalIgnoreCase))
            ?? throw new InvalidOperationException($"Zone '{zoneName}' was not found at the provider.");

        var secret = await secrets.StoreAsync(
            SecretPurpose.DnsProviderToken,
            $"DNS: {name}",
            System.Text.Encoding.UTF8.GetBytes(apiToken),
            cancellationToken,
            serviceSyncEligible: true);

        var connection = new ConnectionEntity
        {
            Name = name,
            Type = ConnectionTypeNames.DnsProvider,
            HealthState = ConnectionHealthStateNames.Healthy,
            LastValidatedAtUtc = DateTimeOffset.UtcNow,
            SecretId = secret.Id,
            SettingsJson = JsonSerializer.Serialize(
                new DnsConnectionSettings(DnsProviderTypeNames.Hetzner, zoneName, defaultTtl)),
            DeletionPolicy = ConnectionDeletionPolicyNames.Required,
        };
        db.Connections.Add(connection);
        db.DnsZones.Add(new DnsZoneEntity
        {
            ConnectionId = connection.Id,
            ProviderZoneId = zone.ProviderZoneId,
            Name = zone.Name,
            DefaultTtl = defaultTtl,
        });
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("dns", "connection_created", subjectType: "connection", subjectId: connection.Id.ToString(), cancellationToken: cancellationToken);
        return connection;
    }

    public async Task<(bool Valid, string? Error)> ValidateConnectionAsync(Guid connectionId, CancellationToken cancellationToken = default)
    {
        var connection = await GetDnsConnectionAsync(connectionId, cancellationToken);
        var provider = await CreateProviderAsync(connection, cancellationToken);
        try
        {
            await provider.ListZonesAsync(cancellationToken);
            connection.HealthState = ConnectionHealthStateNames.Healthy;
            connection.LastValidationMessage = null;
            connection.LastValidatedAtUtc = DateTimeOffset.UtcNow;
            await db.SaveChangesAsync(cancellationToken);
            return (true, null);
        }
        catch (Exception ex)
        {
            connection.HealthState = ConnectionHealthStateNames.Failed;
            connection.LastValidationMessage = ex.Message;
            connection.LastValidatedAtUtc = DateTimeOffset.UtcNow;
            await db.SaveChangesAsync(cancellationToken);
            return (false, ex.Message);
        }
    }

    public async Task<(bool Valid, string? Error)> ValidateWriteAsync(
        Guid connectionId,
        bool confirmDryRun,
        CancellationToken cancellationToken = default)
    {
        if (!confirmDryRun)
        {
            return (false, "Confirm dry-run write validation before creating a _hashi-test record.");
        }

        var connection = await GetDnsConnectionAsync(connectionId, cancellationToken);
        var zone = await GetZoneAsync(connection.Id, cancellationToken);
        var provider = await CreateProviderAsync(connection, cancellationToken);
        var testName = "_hashi-test." + zone.Name.TrimEnd('.');
        try
        {
            var created = await provider.CreateRecordAsync(
                zone.ProviderZoneId,
                testName,
                DnsRecordType.Txt,
                "hashi-write-validation",
                60,
                cancellationToken);
            await provider.DeleteRecordAsync(created.ProviderRecordId, cancellationToken);
            connection.HealthState = ConnectionHealthStateNames.Healthy;
            connection.LastValidationMessage = "Write validation succeeded.";
            connection.LastValidatedAtUtc = DateTimeOffset.UtcNow;
            await db.SaveChangesAsync(cancellationToken);
            await audit.WriteAsync("dns", "write_validation_succeeded", subjectType: "connection", subjectId: connection.Id.ToString(), cancellationToken: cancellationToken);
            return (true, null);
        }
        catch (Exception ex)
        {
            connection.HealthState = ConnectionHealthStateNames.Failed;
            connection.LastValidationMessage = ex.Message;
            connection.LastValidatedAtUtc = DateTimeOffset.UtcNow;
            await db.SaveChangesAsync(cancellationToken);
            return (false, ex.Message);
        }
    }

    public async Task<IReadOnlyList<DnsRecordSnapshot>> ListProviderRecordsAsync(
        Guid connectionId,
        CancellationToken cancellationToken = default)
    {
        var connection = await GetDnsConnectionAsync(connectionId, cancellationToken);
        var zone = await GetZoneAsync(connection.Id, cancellationToken);
        var provider = await CreateProviderAsync(connection, cancellationToken);
        return await provider.ListRecordsAsync(zone.ProviderZoneId, cancellationToken);
    }

    public async Task<IReadOnlyList<DnsImportDecisionEntity>> BuildImportPreviewAsync(
        Guid connectionId,
        CancellationToken cancellationToken = default)
    {
        var connection = await GetDnsConnectionAsync(connectionId, cancellationToken);
        var zone = await GetZoneAsync(connection.Id, cancellationToken);
        var provider = await CreateProviderAsync(connection, cancellationToken);
        var records = await provider.ListRecordsAsync(zone.ProviderZoneId, cancellationToken);

        db.DnsImportDecisions.RemoveRange(db.DnsImportDecisions.Where(x => x.ZoneId == zone.Id));
        var decisions = records
            .Select(x => new DnsImportDecisionEntity
            {
                ZoneId = zone.Id,
                ProviderRecordId = x.ProviderRecordId,
                Name = x.Name,
                Type = DnsRecordTypeMapping.ToApiName(x.Type),
                Value = x.Value,
            })
            .ToList();
        db.DnsImportDecisions.AddRange(decisions);
        await db.SaveChangesAsync(cancellationToken);
        return decisions;
    }

    public async Task ApplyImportAsync(
        Guid connectionId,
        IReadOnlyList<Guid> selectedDecisionIds,
        CancellationToken cancellationToken = default)
    {
        var connection = await GetDnsConnectionAsync(connectionId, cancellationToken);
        var zone = await GetZoneAsync(connection.Id, cancellationToken);
        var decisions = await db.DnsImportDecisions
            .Where(x => x.ZoneId == zone.Id)
            .ToListAsync(cancellationToken);

        foreach (var decision in decisions)
        {
            decision.SelectedForImport = selectedDecisionIds.Contains(decision.Id);
            if (!decision.SelectedForImport)
            {
                continue;
            }

            if (DnsSafetyRules.IsProtectedType(DnsRecordTypeMapping.Parse(decision.Type)))
            {
                decision.SelectedForImport = false;
                continue;
            }

            if (await db.DnsRecords.AnyAsync(
                    x => x.ZoneId == zone.Id && x.ProviderRecordId == decision.ProviderRecordId,
                    cancellationToken))
            {
                continue;
            }

            db.DnsRecords.Add(new DnsRecordEntity
            {
                ZoneId = zone.Id,
                ProviderRecordId = decision.ProviderRecordId,
                Name = decision.Name,
                Type = decision.Type,
                Value = decision.Value,
                Ownership = DnsOwnershipNames.Imported,
            });
        }

        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("dns", "import_applied", subjectType: "connection", subjectId: connection.Id.ToString(), cancellationToken: cancellationToken);
    }

    public async Task<DnsSyncPlan> BuildPrunePreviewAsync(Guid connectionId, CancellationToken cancellationToken = default)
    {
        var connection = await GetDnsConnectionAsync(connectionId, cancellationToken);
        var zone = await GetZoneAsync(connection.Id, cancellationToken);
        var candidates = await db.DnsImportDecisions
            .Where(x => x.ZoneId == zone.Id && !x.SelectedForImport)
            .ToListAsync(cancellationToken);
        if (candidates.Count == 0)
        {
            return new DnsSyncPlan(Guid.NewGuid(), connection.Id, zone.Name, [], RequiresConfirmation: true);
        }

        var changes = candidates
            .Where(x => !DnsSafetyRules.IsProtectedType(DnsRecordTypeMapping.Parse(x.Type)))
            .Select(x => new DnsPlanChange(
                DnsChangeKind.Delete,
                x.Name,
                DnsRecordTypeMapping.Parse(x.Type),
                x.Value,
                null,
                zone.DefaultTtl,
                "Prune provider record not imported into Hashi"))
            .ToList();
        return new DnsSyncPlan(Guid.NewGuid(), connection.Id, zone.Name, changes, RequiresConfirmation: true);
    }

    public async Task ApplyPruneAsync(
        Guid connectionId,
        bool confirmDestructive,
        CancellationToken cancellationToken = default)
    {
        var plan = await BuildPrunePreviewAsync(connectionId, cancellationToken);
        if (plan.Changes.Count == 0)
        {
            return;
        }

        await ApplyPlanAsync(plan, confirmDestructive, cancellationToken);
        await audit.WriteAsync("dns", "prune_applied", subjectType: "connection", subjectId: connectionId.ToString(), cancellationToken: cancellationToken);
    }

    public async Task<DnsSyncPlan> PlanSyncAsync(Guid connectionId, CancellationToken cancellationToken = default)
    {
        var connection = await GetDnsConnectionAsync(connectionId, cancellationToken);
        var zone = await GetZoneAsync(connection.Id, cancellationToken);
        var provider = await CreateProviderAsync(connection, cancellationToken);
        var current = await BuildCurrentSnapshotAsync(provider, zone.Id, zone.ProviderZoneId, cancellationToken);
        var desired = await DnsDesiredStateBuilder.BuildAsync(db, zone.Id, zone.DefaultTtl, cancellationToken);

        var changes = DnsPlanner.BuildPlan(current, desired);
        var requiresConfirmation = changes.Any(x => x.Kind == DnsChangeKind.Delete);
        var planId = ComputePlanId(connection.Id, current, desired, changes);
        return new DnsSyncPlan(planId, connection.Id, zone.Name, changes, requiresConfirmation);
    }

    public async Task ApplyPlanAsync(DnsSyncPlan plan, bool confirmDestructive, CancellationToken cancellationToken = default)
    {
        if (plan.RequiresConfirmation && !confirmDestructive)
        {
            throw new InvalidOperationException("Destructive DNS changes require confirmation.");
        }

        var connection = await GetDnsConnectionAsync(plan.ConnectionId, cancellationToken);
        var zone = await GetZoneAsync(connection.Id, cancellationToken);
        var provider = await CreateProviderAsync(connection, cancellationToken);
        var current = await BuildCurrentSnapshotAsync(provider, zone.Id, zone.ProviderZoneId, cancellationToken);

        foreach (var change in plan.Changes.Where(x => x.Kind != DnsChangeKind.NoOp))
        {
            if (change.Kind == DnsChangeKind.Delete && DnsSafetyRules.IsProtectedType(change.Type))
            {
                continue;
            }

            switch (change.Kind)
            {
                case DnsChangeKind.Create:
                    if (current.Any(x => IsSameRecordKey(x, change)))
                    {
                        throw new InvalidOperationException("DNS plan is stale; provider record now exists for requested create.");
                    }

                    var created = await provider.CreateRecordAsync(
                        zone.ProviderZoneId,
                        change.Name,
                        change.Type,
                        change.DesiredValue ?? string.Empty,
                        change.Ttl,
                        cancellationToken);
                    created = created with { IsManagedByHashi = true };
                    await PersistCreatedProviderRecordAsync(zone.Id, change, created, cancellationToken);
                    current = current.Append(created).ToList();
                    break;
                case DnsChangeKind.Update:
                    var existing = FindApplyTarget(change, current)
                        ?? throw new InvalidOperationException("DNS plan is stale; owned provider record was not found.");
                    if (!DnsSafetyRules.CanModify(existing, DnsChangeKind.Update))
                    {
                        throw new InvalidOperationException("DNS plan attempted to update a protected or unowned provider record.");
                    }

                    var updated = await provider.UpdateRecordAsync(existing.ProviderRecordId, change.DesiredValue ?? string.Empty, change.Ttl, cancellationToken);
                    updated = updated with { IsManagedByHashi = true };
                    await PersistUpdatedProviderRecordAsync(zone.Id, change, updated, cancellationToken);
                    current = current.Select(x => x.ProviderRecordId == updated.ProviderRecordId ? updated : x).ToList();
                    break;
                case DnsChangeKind.Delete:
                    var toDelete = FindApplyTarget(change, current)
                        ?? throw new InvalidOperationException("DNS plan is stale; owned provider record was not found.");
                    if (!DnsSafetyRules.CanDelete(toDelete))
                    {
                        throw new InvalidOperationException("DNS plan attempted to delete a protected or unowned provider record.");
                    }

                    await provider.DeleteRecordAsync(toDelete.ProviderRecordId, cancellationToken);
                    current = current.Where(x => x.ProviderRecordId != toDelete.ProviderRecordId).ToList();
                    break;
            }
        }

        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("dns", "sync_applied", subjectType: "connection", subjectId: connection.Id.ToString(), cancellationToken: cancellationToken);
    }

    public async Task<Guid> ApplyPlanWithSyncRunAsync(
        DnsSyncPlan plan,
        bool confirmDestructive,
        CancellationToken cancellationToken = default)
    {
        var run = new SyncRunEntity
        {
            Subsystem = "dns",
            Status = SyncRunStatusNames.Applying,
            RiskLevel = GetRiskLevel(plan).ToString(),
        };
        db.SyncRuns.Add(run);
        db.SyncSteps.Add(new SyncStepEntity
        {
            SyncRunId = run.Id,
            Name = $"dns-apply-{plan.ConnectionId}",
            Status = SyncRunStatusNames.Applying,
        });
        foreach (var change in plan.Changes)
        {
            db.SyncDiffs.Add(new SyncDiffEntity
            {
                SyncRunId = run.Id,
                ResourceType = "dns",
                ResourceKey = $"{change.Name}/{DnsRecordTypeMapping.ToApiName(change.Type)}",
                ChangeKind = change.Kind.ToString(),
                Summary = change.RiskReason,
                BeforeJson = JsonSerializer.Serialize(new { value = change.CurrentValue, ttl = change.Ttl }),
                AfterJson = JsonSerializer.Serialize(new { value = change.DesiredValue, ttl = change.Ttl }),
            });
        }

        await db.SaveChangesAsync(cancellationToken);
        try
        {
            await ApplyPlanAsync(plan, confirmDestructive, cancellationToken);
            run.Status = SyncRunStatusNames.Succeeded;
            run.CompletedAtUtc = DateTimeOffset.UtcNow;
            db.SyncSteps.Add(new SyncStepEntity
            {
                SyncRunId = run.Id,
                Name = $"dns-apply-{plan.ConnectionId}-complete",
                Status = SyncRunStatusNames.Succeeded,
                CompletedAtUtc = DateTimeOffset.UtcNow,
                Message = "Applied",
            });
            await db.SaveChangesAsync(cancellationToken);
            return run.Id;
        }
        catch (Exception ex)
        {
            run.Status = plan.RequiresConfirmation && !confirmDestructive
                ? SyncRunStatusNames.AwaitingConfirmation
                : SyncRunStatusNames.Failed;
            run.CompletedAtUtc = DateTimeOffset.UtcNow;
            run.ErrorSummary = ex.Message;
            db.SyncSteps.Add(new SyncStepEntity
            {
                SyncRunId = run.Id,
                Name = $"dns-apply-{plan.ConnectionId}-failed",
                Status = SyncRunStatusNames.Failed,
                CompletedAtUtc = DateTimeOffset.UtcNow,
                Message = ex.Message,
            });
            await db.SaveChangesAsync(cancellationToken);
            throw;
        }
    }

    private async Task<IReadOnlyList<DnsRecordSnapshot>> BuildCurrentSnapshotAsync(
        IDnsProvider provider,
        Guid zoneId,
        string providerZoneId,
        CancellationToken cancellationToken)
    {
        var providerRecords = await provider.ListRecordsAsync(providerZoneId, cancellationToken);
        var ownedProviderIds = await db.DnsRecords.AsNoTracking()
            .Where(x => x.ZoneId == zoneId
                && x.Enabled
                && x.ProviderRecordId != string.Empty
                && x.Ownership != DnsOwnershipNames.Unknown)
            .Select(x => x.ProviderRecordId)
            .ToListAsync(cancellationToken);
        var ownedProviderIdSet = ownedProviderIds.ToHashSet(StringComparer.OrdinalIgnoreCase);

        return providerRecords
            .Select(x => x with { IsManagedByHashi = x.IsManagedByHashi || ownedProviderIdSet.Contains(x.ProviderRecordId) })
            .ToList();
    }

    private static DnsRecordSnapshot? FindApplyTarget(
        DnsPlanChange change,
        IReadOnlyList<DnsRecordSnapshot> current)
    {
        if (!string.IsNullOrWhiteSpace(change.ProviderRecordId))
        {
            var byProviderId = current.SingleOrDefault(x =>
                string.Equals(x.ProviderRecordId, change.ProviderRecordId, StringComparison.OrdinalIgnoreCase));
            return byProviderId is { IsManagedByHashi: true } ? byProviderId : null;
        }

        var matches = current
            .Where(x => x.IsManagedByHashi && IsSameRecordKey(x, change))
            .ToList();
        return matches.Count == 1 ? matches[0] : null;
    }

    private async Task PersistCreatedProviderRecordAsync(
        Guid zoneId,
        DnsPlanChange change,
        DnsRecordSnapshot created,
        CancellationToken cancellationToken)
    {
        var record = await db.DnsRecords
            .Where(x => x.ZoneId == zoneId
                && x.Enabled
                && x.Name == change.Name
                && x.Type == DnsRecordTypeMapping.ToApiName(change.Type))
            .Where(x => !IsMultiValue(change.Type) || x.Value == (change.DesiredValue ?? string.Empty))
            .OrderBy(x => x.ProviderRecordId == string.Empty ? 0 : 1)
            .FirstOrDefaultAsync(cancellationToken);
        if (record is null)
        {
            return;
        }

        record.ProviderRecordId = created.ProviderRecordId;
        record.Value = created.Value;
        record.Ttl = created.Ttl;
        record.Ownership = record.Ownership == DnsOwnershipNames.Unknown
            ? DnsOwnershipNames.Managed
            : record.Ownership;
        record.UpdatedAtUtc = DateTimeOffset.UtcNow;
    }

    private async Task PersistUpdatedProviderRecordAsync(
        Guid zoneId,
        DnsPlanChange change,
        DnsRecordSnapshot updated,
        CancellationToken cancellationToken)
    {
        var record = await db.DnsRecords
            .Where(x => x.ZoneId == zoneId
                && x.Enabled
                && (x.ProviderRecordId == updated.ProviderRecordId
                    || (x.Name == change.Name
                        && x.Type == DnsRecordTypeMapping.ToApiName(change.Type)
                        && (!IsMultiValue(change.Type) || x.Value == (change.CurrentValue ?? string.Empty)))))
            .FirstOrDefaultAsync(cancellationToken);
        if (record is null)
        {
            return;
        }

        record.ProviderRecordId = updated.ProviderRecordId;
        record.Value = updated.Value;
        record.Ttl = updated.Ttl;
        record.UpdatedAtUtc = DateTimeOffset.UtcNow;
    }

    private static Guid ComputePlanId(
        Guid connectionId,
        IReadOnlyList<DnsRecordSnapshot> current,
        IReadOnlyList<DnsRecordSnapshot> desired,
        IReadOnlyList<DnsPlanChange> changes)
    {
        // This deterministic id is the staleness token until DNS sync plans get a durable table.
        var canonical = new StringBuilder()
            .Append("dns-sync-plan-v1|")
            .Append(connectionId)
            .AppendLine();

        AppendRecords(canonical, "current", current);
        AppendRecords(canonical, "desired", desired);
        AppendChanges(canonical, changes);

        var hash = SHA256.HashData(Encoding.UTF8.GetBytes(canonical.ToString()));
        return new Guid(hash[..16]);
    }

    private static void AppendRecords(StringBuilder builder, string label, IReadOnlyList<DnsRecordSnapshot> records)
    {
        builder.Append(label).AppendLine();
        foreach (var record in records
            .OrderBy(x => x.ProviderRecordId, StringComparer.OrdinalIgnoreCase)
            .ThenBy(x => x.Name, StringComparer.OrdinalIgnoreCase)
            .ThenBy(x => DnsRecordTypeMapping.ToApiName(x.Type), StringComparer.OrdinalIgnoreCase)
            .ThenBy(x => x.Value, StringComparer.OrdinalIgnoreCase))
        {
            builder
                .Append(record.ProviderRecordId).Append('|')
                .Append(record.Name).Append('|')
                .Append(DnsRecordTypeMapping.ToApiName(record.Type)).Append('|')
                .Append(record.Value).Append('|')
                .Append(record.Ttl?.ToString(System.Globalization.CultureInfo.InvariantCulture) ?? string.Empty).Append('|')
                .Append(record.IsManagedByHashi)
                .AppendLine();
        }
    }

    private static void AppendChanges(StringBuilder builder, IReadOnlyList<DnsPlanChange> changes)
    {
        builder.Append("changes").AppendLine();
        foreach (var change in changes
            .OrderBy(x => x.ProviderRecordId, StringComparer.OrdinalIgnoreCase)
            .ThenBy(x => x.Name, StringComparer.OrdinalIgnoreCase)
            .ThenBy(x => DnsRecordTypeMapping.ToApiName(x.Type), StringComparer.OrdinalIgnoreCase)
            .ThenBy(x => x.Kind))
        {
            builder
                .Append(change.ProviderRecordId).Append('|')
                .Append(change.Kind).Append('|')
                .Append(change.Name).Append('|')
                .Append(DnsRecordTypeMapping.ToApiName(change.Type)).Append('|')
                .Append(change.CurrentValue).Append('|')
                .Append(change.DesiredValue).Append('|')
                .Append(change.Ttl?.ToString(System.Globalization.CultureInfo.InvariantCulture) ?? string.Empty).Append('|')
                .Append(change.RiskReason)
                .AppendLine();
        }
    }

    private static bool IsSameRecordKey(DnsRecordSnapshot record, DnsPlanChange change)
    {
        if (!string.Equals(record.Name, change.Name, StringComparison.OrdinalIgnoreCase)
            || record.Type != change.Type)
        {
            return false;
        }

        if (!IsMultiValue(record.Type))
        {
            return true;
        }

        var changeValue = change.DesiredValue ?? change.CurrentValue ?? string.Empty;
        return string.Equals(Normalize(record.Value), Normalize(changeValue), StringComparison.OrdinalIgnoreCase);
    }

    private static bool IsMultiValue(DnsRecordType type)
        => type is DnsRecordType.Mx or DnsRecordType.Txt;

    private static string Normalize(string value) => value.Trim().TrimEnd('.');

    private static SyncRiskLevel GetRiskLevel(DnsSyncPlan plan)
    {
        if (plan.Changes.Any(x => x.Kind == DnsChangeKind.Delete))
        {
            return SyncRiskLevel.Destructive;
        }

        return plan.Changes.Any(x => x.Kind is DnsChangeKind.Create or DnsChangeKind.Update)
            ? SyncRiskLevel.Low
            : SyncRiskLevel.None;
    }

    public async Task ApplySafePlanAsync(DnsSyncPlan plan, CancellationToken cancellationToken = default)
    {
        var safeChanges = plan.Changes
            .Where(x => x.Kind is not DnsChangeKind.Delete and not DnsChangeKind.NoOp)
            .ToList();
        if (safeChanges.Count == 0)
        {
            return;
        }

        var safePlan = new DnsSyncPlan(plan.PlanId, plan.ConnectionId, plan.ZoneName, safeChanges, RequiresConfirmation: false);
        await ApplyPlanAsync(safePlan, confirmDestructive: true, cancellationToken);
    }

    private async Task<ConnectionEntity> GetDnsConnectionAsync(Guid connectionId, CancellationToken cancellationToken)
    {
        return await db.Connections.SingleOrDefaultAsync(
            x => x.Id == connectionId && x.Type == ConnectionTypeNames.DnsProvider,
            cancellationToken) ?? throw new InvalidOperationException("DNS connection not found.");
    }

    public async Task<DnsProviderCapabilities> GetCapabilitiesAsync(Guid connectionId, CancellationToken cancellationToken = default)
    {
        var connection = await GetDnsConnectionAsync(connectionId, cancellationToken);
        var provider = await CreateProviderAsync(connection, cancellationToken);
        return await provider.GetCapabilitiesAsync(cancellationToken);
    }

    private async Task<DnsZoneEntity> GetZoneAsync(Guid connectionId, CancellationToken cancellationToken)
    {
        return await db.DnsZones.SingleOrDefaultAsync(x => x.ConnectionId == connectionId, cancellationToken)
            ?? throw new InvalidOperationException("DNS zone not configured for connection.");
    }

    private async Task<IDnsProvider> CreateProviderAsync(ConnectionEntity connection, CancellationToken cancellationToken)
    {
        if (connection.SecretId is null)
        {
            throw new InvalidOperationException("DNS connection has no stored credentials.");
        }

        var tokenBytes = await secrets.DecryptForPurposeAsync(connection.SecretId.Value, cancellationToken)
            ?? throw new InvalidOperationException("Credentials unavailable; unlock vault or configure service-sync vault.");
        var token = System.Text.Encoding.UTF8.GetString(tokenBytes);
        var settings = JsonSerializer.Deserialize<DnsConnectionSettings>(connection.SettingsJson)
            ?? new DnsConnectionSettings(DnsProviderTypeNames.Hetzner, string.Empty, 3600);
        return providerFactory.Create(settings.Provider, token);
    }

    private sealed record DnsConnectionSettings(
        [property: JsonPropertyName("provider")] string Provider,
        [property: JsonPropertyName("zoneName")] string ZoneName,
        [property: JsonPropertyName("defaultTtl")] int DefaultTtl);
}

public static class DnsProviderValidation
{
    public static async Task<(bool Valid, string? Error)> ValidateHetznerTokenAsync(
        IHttpClientFactory httpClientFactory,
        string apiToken,
        CancellationToken cancellationToken = default)
    {
        var client = httpClientFactory.CreateClient("hetzner-dns");
        return await HetznerDnsProvider.ValidateTokenAsync(client, apiToken, cancellationToken);
    }
}
