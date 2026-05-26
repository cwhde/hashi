using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class AdGuardSyncService(HashiDbContext db, IHttpClientFactory httpClientFactory, SecretRecordService secrets)
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

    public async Task<bool> DeleteRewriteAsync(Guid connectionId, Guid rewriteId, CancellationToken cancellationToken = default)
    {
        var rewrite = await db.AdGuardRewrites.SingleOrDefaultAsync(
            x => x.Id == rewriteId && x.ConnectionId == connectionId,
            cancellationToken);
        if (rewrite is null)
        {
            return false;
        }

        if (!rewrite.ManagedByHashi)
        {
            throw new InvalidOperationException("Rewrite is managed manually and cannot be deleted by Hashi.");
        }

        try
        {
            var remoteRewrites = await ListRemoteRewritesAsync(connectionId, cancellationToken);
            var remote = remoteRewrites.FirstOrDefault(x =>
                string.Equals(x.Domain, rewrite.Domain, StringComparison.OrdinalIgnoreCase));
            if (remote is not null)
            {
                await DeleteRemoteRewriteAsync(connectionId, remote, cancellationToken);
            }
        }
        catch
        {
            // Best effort remote delete; local row is still removed.
        }

        db.AdGuardRewrites.Remove(rewrite);
        await db.SaveChangesAsync(cancellationToken);
        return true;
    }

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
        await SyncResourceTopologyRewritesAsync(connectionId, cancellationToken);

        var remoteRewrites = await ListRemoteRewritesAsync(connectionId, cancellationToken);
        var localManaged = await db.AdGuardRewrites
            .Where(x => x.ConnectionId == connectionId && x.ManagedByHashi)
            .ToListAsync(cancellationToken);

        foreach (var rewrite in localManaged)
        {
            var remote = remoteRewrites.FirstOrDefault(x =>
                string.Equals(x.Domain, rewrite.Domain, StringComparison.OrdinalIgnoreCase));
            if (remote is null || !string.Equals(remote.Answer, rewrite.Answer, StringComparison.Ordinal))
            {
                await PushToAdGuardAsync(connectionId, rewrite, cancellationToken);
            }
        }

        var managedDomains = localManaged
            .Select(x => x.Domain)
            .ToHashSet(StringComparer.OrdinalIgnoreCase);
        foreach (var remote in remoteRewrites)
        {
            if (managedDomains.Contains(remote.Domain))
            {
                continue;
            }

            var tracked = await db.AdGuardRewrites.AsNoTracking()
                .AnyAsync(x => x.ConnectionId == connectionId && x.ProviderRewriteId == remote.Id, cancellationToken);
            if (tracked)
            {
                await DeleteRemoteRewriteAsync(connectionId, remote, cancellationToken);
            }
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

    private async Task PushToAdGuardAsync(Guid connectionId, AdGuardRewriteEntity rewrite, CancellationToken cancellationToken)
    {
        var client = await CreateAuthorizedClientAsync(connectionId, cancellationToken);
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
