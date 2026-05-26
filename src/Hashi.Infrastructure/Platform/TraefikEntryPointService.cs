using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class TraefikEntryPointService(HashiDbContext db)
{
    private static readonly HashSet<int> AlwaysConfirmedPorts = [80, 443];

    public async Task SyncForResourceAsync(ResourceEntity resource, CancellationToken cancellationToken = default)
    {
        if (resource.Kind is not ("tcp" or "udp"))
        {
            return;
        }

        var port = resource.PublicPort ?? resource.TargetPort;
        var existing = await db.TraefikEntryPoints
            .SingleOrDefaultAsync(x => x.Port == port && x.Protocol == resource.Kind, cancellationToken);
        if (existing is null)
        {
            db.TraefikEntryPoints.Add(new TraefikEntryPointEntity
            {
                Port = port,
                Protocol = resource.Kind,
                ResourceId = resource.Id,
                Label = $"{resource.Name} ({resource.Kind}/{port})",
                Confirmed = false,
            });
            return;
        }

        existing.ResourceId = resource.Id;
        existing.Label = $"{resource.Name} ({resource.Kind}/{port})";
    }

    public async Task RemoveForResourceAsync(Guid resourceId, CancellationToken cancellationToken = default)
    {
        var entries = await db.TraefikEntryPoints.Where(x => x.ResourceId == resourceId).ToListAsync(cancellationToken);
        db.TraefikEntryPoints.RemoveRange(entries);
    }

    public async Task<IReadOnlyList<TraefikEntryPointResponse>> ListPendingAsync(CancellationToken cancellationToken = default)
    {
        var items = await db.TraefikEntryPoints.AsNoTracking()
            .Where(x => !x.Confirmed)
            .OrderBy(x => x.Port)
            .ToListAsync(cancellationToken);
        return items.Select(ToResponse).ToList();
    }

    public async Task<IReadOnlyList<TraefikEntryPointResponse>> ListAllAsync(CancellationToken cancellationToken = default)
    {
        var items = await db.TraefikEntryPoints.AsNoTracking().OrderBy(x => x.Port).ToListAsync(cancellationToken);
        return items.Select(ToResponse).ToList();
    }

    public async Task<TraefikEntryPointResponse?> ConfirmAsync(Guid id, CancellationToken cancellationToken = default)
    {
        var entry = await db.TraefikEntryPoints.SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        if (entry is null)
        {
            return null;
        }

        entry.Confirmed = true;
        entry.ConfirmedAtUtc = DateTimeOffset.UtcNow;
        await db.SaveChangesAsync(cancellationToken);
        return ToResponse(entry);
    }

    public async Task<HashSet<(int Port, string Protocol)>> GetConfirmedPortKeysAsync(CancellationToken cancellationToken = default)
    {
        var confirmed = await db.TraefikEntryPoints.AsNoTracking()
            .Where(x => x.Confirmed)
            .Select(x => new { x.Port, x.Protocol })
            .ToListAsync(cancellationToken);
        var set = confirmed.Select(x => (x.Port, x.Protocol)).ToHashSet();
        set.Add((80, "tcp"));
        set.Add((443, "tcp"));
        return set;
    }

    public bool IsPortConfirmed(int port, string protocol, HashSet<(int Port, string Protocol)> confirmed)
        => AlwaysConfirmedPorts.Contains(port) || confirmed.Contains((port, protocol));

    private static TraefikEntryPointResponse ToResponse(TraefikEntryPointEntity entity) => new(
        entity.Id,
        entity.Port,
        entity.Protocol,
        entity.ResourceId,
        entity.Label,
        entity.Confirmed,
        entity.ConfirmedAtUtc);
}
