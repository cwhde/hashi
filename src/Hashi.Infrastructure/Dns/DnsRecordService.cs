using Hashi.Core.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Dns;

public sealed class DnsRecordService(HashiDbContext db, AuditService audit)
{
    private static readonly HashSet<DnsRecordType> ManualTypes =
    [
        DnsRecordType.A,
        DnsRecordType.Aaaa,
        DnsRecordType.Cname,
        DnsRecordType.Mx,
        DnsRecordType.Txt,
    ];

    public async Task<IReadOnlyList<DnsZoneEntity>> ListZonesAsync(CancellationToken cancellationToken = default)
        => await db.DnsZones.AsNoTracking()
            .Include(x => x.Connection)
            .OrderBy(x => x.Name)
            .ToListAsync(cancellationToken);

    public async Task<IReadOnlyList<DnsRecordEntity>> ListAsync(CancellationToken cancellationToken = default)
        => await db.DnsRecords.AsNoTracking()
            .OrderBy(x => x.Name)
            .ThenBy(x => x.Type)
            .ToListAsync(cancellationToken);

    public async Task<DnsRecordEntity> CreateManualAsync(
        Guid zoneId,
        string name,
        string type,
        string value,
        int? ttl,
        bool enabled,
        CancellationToken cancellationToken = default)
    {
        var normalized = await ValidateAsync(zoneId, name, type, value, ttl, null, cancellationToken);
        var record = new DnsRecordEntity
        {
            ZoneId = zoneId,
            Name = normalized.Name,
            Type = normalized.Type,
            Value = normalized.Value,
            Ttl = normalized.Ttl,
            Enabled = enabled,
            Ownership = DnsOwnershipNames.User,
        };
        db.DnsRecords.Add(record);
        db.DnsRecordOwnership.Add(new DnsRecordOwnershipEntity
        {
            ZoneId = zoneId,
            DnsRecordId = record.Id,
            Name = record.Name,
            Type = record.Type,
            Value = record.Value,
            Ownership = DnsOwnershipNames.User,
            OwnerWorkflow = "manual_dns",
            SyncState = DnsOwnershipSyncStateNames.Desired,
        });
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("dns", "manual_record_created", subjectType: "dns_record", subjectId: record.Id.ToString(), cancellationToken: cancellationToken);
        return record;
    }

    public async Task<DnsRecordEntity?> UpdateManualAsync(
        Guid recordId,
        Guid zoneId,
        string name,
        string type,
        string value,
        int? ttl,
        bool enabled,
        CancellationToken cancellationToken = default)
    {
        var record = await GetMutableUserRecordAsync(recordId, cancellationToken);
        if (record is null)
        {
            return null;
        }

        var normalized = await ValidateAsync(zoneId, name, type, value, ttl, recordId, cancellationToken);
        record.ZoneId = zoneId;
        record.Name = normalized.Name;
        record.Type = normalized.Type;
        record.Value = normalized.Value;
        record.Ttl = normalized.Ttl;
        record.Enabled = enabled;
        record.UpdatedAtUtc = DateTimeOffset.UtcNow;

        var ownership = await db.DnsRecordOwnership
            .FirstOrDefaultAsync(x => x.DnsRecordId == record.Id, cancellationToken);
        if (ownership is null)
        {
            db.DnsRecordOwnership.Add(new DnsRecordOwnershipEntity
            {
                ZoneId = zoneId,
                DnsRecordId = record.Id,
                Name = normalized.Name,
                Type = normalized.Type,
                Value = normalized.Value,
                Ownership = DnsOwnershipNames.User,
                OwnerWorkflow = "manual_dns",
                SyncState = DnsOwnershipSyncStateNames.Desired,
            });
        }
        else
        {
            ownership.ZoneId = zoneId;
            ownership.Name = normalized.Name;
            ownership.Type = normalized.Type;
            ownership.Value = normalized.Value;
            ownership.Ownership = DnsOwnershipNames.User;
            ownership.OwnerWorkflow = "manual_dns";
            ownership.SyncState = DnsOwnershipSyncStateNames.Desired;
        }

        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("dns", "manual_record_updated", subjectType: "dns_record", subjectId: record.Id.ToString(), cancellationToken: cancellationToken);
        return record;
    }

    public async Task<bool> DeleteManualAsync(Guid recordId, CancellationToken cancellationToken = default)
    {
        var record = await GetMutableUserRecordAsync(recordId, cancellationToken);
        if (record is null)
        {
            return false;
        }

        var ownership = await db.DnsRecordOwnership
            .Where(x => x.DnsRecordId == record.Id)
            .ToListAsync(cancellationToken);
        db.DnsRecordOwnership.RemoveRange(ownership);
        db.DnsRecords.Remove(record);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("dns", "manual_record_deleted", subjectType: "dns_record", subjectId: record.Id.ToString(), cancellationToken: cancellationToken);
        return true;
    }

    private async Task<DnsRecordEntity?> GetMutableUserRecordAsync(Guid recordId, CancellationToken cancellationToken)
        => await db.DnsRecords.SingleOrDefaultAsync(
            x => x.Id == recordId && x.Ownership == DnsOwnershipNames.User,
            cancellationToken);

    private async Task<ValidatedRecord> ValidateAsync(
        Guid zoneId,
        string name,
        string type,
        string value,
        int? ttl,
        Guid? existingId,
        CancellationToken cancellationToken)
    {
        if (!await db.DnsZones.AnyAsync(x => x.Id == zoneId, cancellationToken))
        {
            throw new InvalidOperationException("DNS zone not found.");
        }

        var parsedType = DnsRecordTypeMapping.Parse(type);
        if (!ManualTypes.Contains(parsedType))
        {
            throw new InvalidOperationException("Manual DNS records support A, AAAA, CNAME, MX, and TXT only.");
        }

        var normalizedName = NormalizeRequired(name, "Record name");
        var normalizedValue = NormalizeRequired(value, "Record value");
        if (ttl is <= 0)
        {
            throw new InvalidOperationException("TTL must be greater than zero.");
        }

        var apiType = DnsRecordTypeMapping.ToApiName(parsedType);
        var duplicate = await db.DnsRecords.AnyAsync(
            x => x.ZoneId == zoneId
                && x.Id != existingId
                && x.Enabled
                && x.Name == normalizedName
                && x.Type == apiType,
            cancellationToken);
        if (duplicate)
        {
            throw new InvalidOperationException("A DNS record with the same name and type already exists in this zone.");
        }

        return new ValidatedRecord(normalizedName, apiType, normalizedValue, ttl);
    }

    private static string NormalizeRequired(string value, string label)
    {
        var normalized = value.Trim();
        if (normalized.Length == 0)
        {
            throw new InvalidOperationException($"{label} is required.");
        }

        return normalized;
    }

    private sealed record ValidatedRecord(string Name, string Type, string Value, int? Ttl);
}
