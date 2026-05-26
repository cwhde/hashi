using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class AdGuardSyncService(HashiDbContext db, IHttpClientFactory httpClientFactory)
{
    public async Task<IReadOnlyList<AdGuardRewriteResponse>> ListRewritesAsync(Guid connectionId, CancellationToken cancellationToken = default)
    {
        var items = await db.AdGuardRewrites.AsNoTracking()
            .Where(x => x.ConnectionId == connectionId)
            .OrderBy(x => x.Domain)
            .ToListAsync(cancellationToken);
        return items.Select(x => new AdGuardRewriteResponse(x.Id, x.Domain, x.Answer, x.ManagedByHashi)).ToList();
    }

    public async Task<AdGuardRewriteResponse> UpsertRewriteAsync(
        Guid connectionId,
        UpsertAdGuardRewriteRequest request,
        CancellationToken cancellationToken = default)
    {
        var rewrite = await db.AdGuardRewrites.SingleOrDefaultAsync(
            x => x.ConnectionId == connectionId && x.Domain == request.Domain,
            cancellationToken);
        if (rewrite is null)
        {
            rewrite = new AdGuardRewriteEntity
            {
                ConnectionId = connectionId,
                Domain = request.Domain,
                ManagedByHashi = true,
            };
            db.AdGuardRewrites.Add(rewrite);
        }
        else if (!rewrite.ManagedByHashi)
        {
            throw new InvalidOperationException("Rewrite is managed manually and cannot be changed by Hashi.");
        }

        rewrite.Answer = request.Answer;
        await db.SaveChangesAsync(cancellationToken);
        await PushToAdGuardAsync(connectionId, rewrite, cancellationToken);
        return new AdGuardRewriteResponse(rewrite.Id, rewrite.Domain, rewrite.Answer, rewrite.ManagedByHashi);
    }

    public async Task SyncManagedRewritesAsync(Guid connectionId, CancellationToken cancellationToken = default)
    {
        var rewrites = await db.AdGuardRewrites
            .Where(x => x.ConnectionId == connectionId && x.ManagedByHashi)
            .ToListAsync(cancellationToken);
        foreach (var rewrite in rewrites)
        {
            await PushToAdGuardAsync(connectionId, rewrite, cancellationToken);
        }
    }

    private async Task PushToAdGuardAsync(Guid connectionId, AdGuardRewriteEntity rewrite, CancellationToken cancellationToken)
    {
        var connection = await db.AdGuardConnections.SingleOrDefaultAsync(x => x.Id == connectionId, cancellationToken)
            ?? throw new InvalidOperationException("AdGuard connection not found.");
        var client = httpClientFactory.CreateClient("adguard");
        client.BaseAddress = new Uri(connection.BaseUrl.TrimEnd('/') + "/");
        var payload = new { domain = rewrite.Domain, answer = rewrite.Answer };
        using var response = await client.PostAsJsonAsync("control/rewrite/add", payload, cancellationToken);
        response.EnsureSuccessStatusCode();
        var body = await response.Content.ReadAsStringAsync(cancellationToken);
        using var doc = JsonDocument.Parse(body);
        if (doc.RootElement.TryGetProperty("id", out var idElement))
        {
            rewrite.ProviderRewriteId = idElement.GetRawText();
            await db.SaveChangesAsync(cancellationToken);
        }
    }
}
