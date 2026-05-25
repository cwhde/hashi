using System.Text.Json;
using Hashi.Core.Auth;
using Hashi.Core.Dns;
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
            cancellationToken);

        var connection = new ConnectionEntity
        {
            Name = name,
            Type = ConnectionTypeNames.DnsProvider,
            HealthState = ConnectionHealthStateNames.Healthy,
            LastValidatedAtUtc = DateTimeOffset.UtcNow,
            SecretId = secret.Id,
            SettingsJson = JsonSerializer.Serialize(new { provider = DnsProviderTypeNames.Hetzner, zoneName, defaultTtl }),
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
            .Where(x => !DnsSafetyRules.IsProtectedType(x.Type))
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

    public async Task<DnsSyncPlan> PlanSyncAsync(Guid connectionId, CancellationToken cancellationToken = default)
    {
        var connection = await GetDnsConnectionAsync(connectionId, cancellationToken);
        var zone = await GetZoneAsync(connection.Id, cancellationToken);
        var provider = await CreateProviderAsync(connection, cancellationToken);
        var current = await provider.ListRecordsAsync(zone.ProviderZoneId, cancellationToken);
        var desired = await db.DnsRecords
            .Where(x => x.ZoneId == zone.Id && x.Enabled)
            .Select(x => new DnsRecordSnapshot(
                x.ProviderRecordId,
                x.Name,
                DnsRecordTypeMapping.Parse(x.Type),
                x.Value,
                x.Ttl,
                true))
            .ToListAsync(cancellationToken);

        var changes = DnsPlanner.BuildPlan(current, desired);
        var requiresConfirmation = changes.Any(x => x.Kind == DnsChangeKind.Delete);
        return new DnsSyncPlan(Guid.NewGuid(), connection.Id, zone.Name, changes, requiresConfirmation);
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

        foreach (var change in plan.Changes.Where(x => x.Kind != DnsChangeKind.NoOp))
        {
            if (change.Kind == DnsChangeKind.Delete && DnsSafetyRules.IsProtectedType(change.Type))
            {
                continue;
            }

            switch (change.Kind)
            {
                case DnsChangeKind.Create:
                    await provider.CreateRecordAsync(
                        zone.ProviderZoneId,
                        change.Name,
                        change.Type,
                        change.DesiredValue ?? string.Empty,
                        change.Ttl,
                        cancellationToken);
                    break;
                case DnsChangeKind.Update:
                    var existing = (await provider.ListRecordsAsync(zone.ProviderZoneId, cancellationToken))
                        .First(x => string.Equals(x.Name, change.Name, StringComparison.OrdinalIgnoreCase)
                            && x.Type == change.Type);
                    await provider.UpdateRecordAsync(existing.ProviderRecordId, change.DesiredValue ?? string.Empty, change.Ttl, cancellationToken);
                    break;
                case DnsChangeKind.Delete:
                    var toDelete = (await provider.ListRecordsAsync(zone.ProviderZoneId, cancellationToken))
                        .First(x => string.Equals(x.Name, change.Name, StringComparison.OrdinalIgnoreCase)
                            && x.Type == change.Type);
                    await provider.DeleteRecordAsync(toDelete.ProviderRecordId, cancellationToken);
                    break;
            }
        }

        await audit.WriteAsync("dns", "sync_applied", subjectType: "connection", subjectId: connection.Id.ToString(), cancellationToken: cancellationToken);
    }

    private async Task<ConnectionEntity> GetDnsConnectionAsync(Guid connectionId, CancellationToken cancellationToken)
    {
        return await db.Connections.SingleOrDefaultAsync(
            x => x.Id == connectionId && x.Type == ConnectionTypeNames.DnsProvider,
            cancellationToken) ?? throw new InvalidOperationException("DNS connection not found.");
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

        var tokenBytes = await secrets.DecryptForAdminAsync(connection.SecretId.Value, cancellationToken)
            ?? throw new InvalidOperationException("Vault must be unlocked to use DNS credentials.");
        var token = System.Text.Encoding.UTF8.GetString(tokenBytes);
        var settings = JsonSerializer.Deserialize<DnsConnectionSettings>(connection.SettingsJson)
            ?? new DnsConnectionSettings(DnsProviderTypeNames.Hetzner, string.Empty, 3600);
        return providerFactory.Create(settings.Provider, token);
    }

    private sealed record DnsConnectionSettings(string Provider, string ZoneName, int DefaultTtl);
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
