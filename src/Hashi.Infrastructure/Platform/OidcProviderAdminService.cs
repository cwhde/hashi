using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class OidcProviderAdminService(
    HashiDbContext db,
    SecretRecordService secrets,
    AuditService audit,
    GeoIpLookupService geoIp)
{
    public async Task<IReadOnlyList<OidcProviderResponse>> ListProvidersAsync(CancellationToken cancellationToken = default)
    {
        var items = await db.OidcProviders.AsNoTracking().OrderBy(x => x.Name).ToListAsync(cancellationToken);
        return items.Select(ToProviderResponse).ToList();
    }

    public async Task<OidcProviderResponse> CreateProviderAsync(
        CreateOidcProviderRequest request,
        CancellationToken cancellationToken = default)
    {
        var secret = await secrets.StoreAsync(
            SecretPurpose.OidcClientSecret,
            $"OIDC: {request.Name}",
            System.Text.Encoding.UTF8.GetBytes(request.ClientSecret),
            cancellationToken);
        var entity = new OidcProviderEntity
        {
            Name = request.Name,
            Issuer = request.Issuer.TrimEnd('/'),
            ClientId = request.ClientId,
            ClientSecretId = secret.Id,
            Scopes = request.Scopes ?? "openid profile email",
            Enabled = request.Enabled,
        };
        db.OidcProviders.Add(entity);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("edge_sso", "oidc_provider_created", subjectType: "oidc_provider", subjectId: entity.Id.ToString(), cancellationToken: cancellationToken);
        return ToProviderResponse(entity);
    }

    public async Task<OidcProviderResponse?> UpdateProviderAsync(
        Guid id,
        UpdateOidcProviderRequest request,
        CancellationToken cancellationToken = default)
    {
        var entity = await db.OidcProviders.SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        if (entity is null)
        {
            return null;
        }

        if (request.Name is not null)
        {
            entity.Name = request.Name;
        }

        if (request.Issuer is not null)
        {
            entity.Issuer = request.Issuer.TrimEnd('/');
        }

        if (request.ClientId is not null)
        {
            entity.ClientId = request.ClientId;
        }

        if (request.Scopes is not null)
        {
            entity.Scopes = request.Scopes;
        }

        if (request.Enabled is bool enabled)
        {
            entity.Enabled = enabled;
        }

        if (!string.IsNullOrWhiteSpace(request.ClientSecret))
        {
            var secret = await secrets.StoreAsync(
                SecretPurpose.OidcClientSecret,
                $"OIDC: {entity.Name}",
                System.Text.Encoding.UTF8.GetBytes(request.ClientSecret),
                cancellationToken);
            entity.ClientSecretId = secret.Id;
        }

        await db.SaveChangesAsync(cancellationToken);
        return ToProviderResponse(entity);
    }

    public async Task<bool> DeleteProviderAsync(Guid id, CancellationToken cancellationToken = default)
    {
        var entity = await db.OidcProviders.SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        if (entity is null)
        {
            return false;
        }

        db.OidcProviders.Remove(entity);
        await db.SaveChangesAsync(cancellationToken);
        return true;
    }

    public async Task<IReadOnlyList<EdgeAuthRuleResponse>> ListRulesAsync(CancellationToken cancellationToken = default)
    {
        var items = await db.EdgeAuthRules.AsNoTracking().OrderBy(x => x.Priority).ToListAsync(cancellationToken);
        return items.Select(ToRuleResponse).ToList();
    }

    public async Task<EdgeAuthRuleResponse> CreateRuleAsync(CreateEdgeAuthRuleRequest request, CancellationToken cancellationToken = default)
    {
        ValidateRule(request.MatchJson, request.Enabled);
        var entity = new EdgeAuthRuleEntity
        {
            Name = request.Name,
            Priority = request.Priority,
            MatchJson = request.MatchJson,
            Action = request.Action,
            Enabled = request.Enabled,
        };
        db.EdgeAuthRules.Add(entity);
        await db.SaveChangesAsync(cancellationToken);
        return ToRuleResponse(entity);
    }

    public async Task<EdgeAuthRuleResponse?> UpdateRuleAsync(Guid id, UpdateEdgeAuthRuleRequest request, CancellationToken cancellationToken = default)
    {
        var entity = await db.EdgeAuthRules.SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        if (entity is null)
        {
            return null;
        }

        if (request.Name is not null)
        {
            entity.Name = request.Name;
        }

        if (request.Priority is int priority)
        {
            entity.Priority = priority;
        }

        if (request.MatchJson is not null)
        {
            ValidateRule(request.MatchJson, request.Enabled ?? entity.Enabled);
            entity.MatchJson = request.MatchJson;
        }

        if (request.Action is not null)
        {
            entity.Action = request.Action;
        }

        if (request.Enabled is bool enabled)
        {
            ValidateRule(request.MatchJson ?? entity.MatchJson, enabled);
            entity.Enabled = enabled;
        }

        await db.SaveChangesAsync(cancellationToken);
        return ToRuleResponse(entity);
    }

    public async Task<bool> DeleteRuleAsync(Guid id, CancellationToken cancellationToken = default)
    {
        var entity = await db.EdgeAuthRules.SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        if (entity is null)
        {
            return false;
        }

        db.EdgeAuthRules.Remove(entity);
        await db.SaveChangesAsync(cancellationToken);
        return true;
    }

    private static OidcProviderResponse ToProviderResponse(OidcProviderEntity entity) => new(
        entity.Id,
        entity.Name,
        entity.Issuer,
        entity.ClientId,
        entity.Scopes,
        entity.Enabled);

    private static EdgeAuthRuleResponse ToRuleResponse(EdgeAuthRuleEntity entity) => new(
        entity.Id,
        entity.Name,
        entity.Priority,
        entity.MatchJson,
        entity.Action,
        entity.Enabled);

    private void ValidateRule(string matchJson, bool enabled)
    {
        if (!enabled)
        {
            return;
        }

        var errors = geoIp.ValidateGeoMatchRules(matchJson);
        if (errors.Count > 0)
        {
            throw new InvalidOperationException(string.Join(" ", errors));
        }
    }
}
