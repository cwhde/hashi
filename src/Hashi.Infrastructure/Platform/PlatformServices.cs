using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Firewall;
using Hashi.Core.Resources;
using Hashi.Core.Security;
using Hashi.Core.Traefik;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class ResourceService(HashiDbContext db, AuditService audit)
{
    public async Task<IReadOnlyList<ResourceEntity>> ListAsync(CancellationToken cancellationToken = default)
        => await db.Resources.AsNoTracking().OrderBy(x => x.Name).ToListAsync(cancellationToken);

    public async Task<ResourceEntity> CreateAsync(CreateResourceRequest request, CancellationToken cancellationToken = default)
    {
        var entity = new ResourceEntity
        {
            Name = request.Name,
            Slug = ResourceSlug.Normalize(request.Name),
            Kind = request.Kind,
            Domain = request.Domain,
            TargetScheme = request.TargetScheme,
            TargetHost = request.TargetHost,
            TargetPort = request.TargetPort,
            DashboardEnabled = request.DashboardEnabled,
            StatusEnabled = request.StatusEnabled,
            FirewallHostId = request.FirewallHostId,
            PulseAgentId = request.PulseAgentId,
            PathPrefix = request.PathPrefix,
            PathRewrite = request.PathRewrite,
            ExtraMiddlewaresJson = SerializeExtraMiddlewares(request.ExtraMiddlewares),
        };
        db.Resources.Add(entity);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("resources", "resource_created", subjectType: "resource", subjectId: entity.Id.ToString(), cancellationToken: cancellationToken);
        return entity;
    }

    public async Task<ResourceEntity?> UpdateAsync(Guid id, UpdateResourceRequest request, CancellationToken cancellationToken = default)
    {
        var entity = await db.Resources.SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        if (entity is null)
        {
            return null;
        }

        if (entity.IsSystem && request.Enabled == false)
        {
            throw new InvalidOperationException("System resources cannot be disabled.");
        }

        if (request.Name is not null)
        {
            entity.Name = request.Name;
        }

        if (request.Enabled is bool enabled)
        {
            entity.Enabled = enabled;
        }

        if (request.Domain is not null)
        {
            entity.Domain = request.Domain;
        }

        if (request.TargetScheme is not null)
        {
            entity.TargetScheme = request.TargetScheme;
        }

        if (request.TargetHost is not null)
        {
            entity.TargetHost = request.TargetHost;
        }

        if (request.TargetPort is int port)
        {
            entity.TargetPort = port;
        }

        if (request.DashboardEnabled is bool dashboard)
        {
            entity.DashboardEnabled = dashboard;
        }

        if (request.StatusEnabled is bool status)
        {
            entity.StatusEnabled = status;
        }

        if (request.ClearFirewallHostId)
        {
            entity.FirewallHostId = null;
        }
        else if (request.FirewallHostId is Guid hostId)
        {
            entity.FirewallHostId = hostId;
        }

        if (request.ClearPulseAgentId)
        {
            entity.PulseAgentId = null;
        }
        else if (request.PulseAgentId is Guid pulseAgentId)
        {
            entity.PulseAgentId = pulseAgentId;
        }

        if (request.ClearPathPrefix)
        {
            entity.PathPrefix = null;
        }
        else if (request.PathPrefix is not null)
        {
            entity.PathPrefix = request.PathPrefix;
        }

        if (request.ClearPathRewrite)
        {
            entity.PathRewrite = null;
        }
        else if (request.PathRewrite is not null)
        {
            entity.PathRewrite = request.PathRewrite;
        }

        if (request.ClearExtraMiddlewares)
        {
            entity.ExtraMiddlewaresJson = "[]";
        }
        else if (request.ExtraMiddlewares is not null)
        {
            entity.ExtraMiddlewaresJson = SerializeExtraMiddlewares(request.ExtraMiddlewares);
        }

        entity.UpdatedAtUtc = DateTimeOffset.UtcNow;
        await db.SaveChangesAsync(cancellationToken);
        return entity;
    }

    public async Task<bool> DeleteAsync(Guid id, CancellationToken cancellationToken = default)
    {
        var entity = await db.Resources.SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        if (entity is null)
        {
            return false;
        }

        if (entity.IsSystem)
        {
            throw new InvalidOperationException("System resources cannot be deleted.");
        }

        db.Resources.Remove(entity);
        await db.SaveChangesAsync(cancellationToken);
        return true;
    }

    public static ResourceResponse ToResponse(ResourceEntity entity) => new(
        entity.Id,
        entity.Name,
        entity.Slug,
        entity.Kind,
        entity.Enabled,
        entity.IsSystem,
        entity.Domain,
        entity.TargetScheme,
        entity.TargetHost,
        entity.TargetPort,
        entity.DashboardEnabled,
        entity.StatusEnabled,
        entity.FirewallHostId,
        entity.PulseAgentId,
        entity.PathPrefix,
        entity.PathRewrite,
        TraefikUserMiddlewareService.ParseExtraMiddlewares(entity.ExtraMiddlewaresJson));

    private static string SerializeExtraMiddlewares(IReadOnlyList<string>? middlewares)
        => JsonSerializer.Serialize(middlewares ?? []);
}

public sealed class TraefikPlatformService(
    HashiDbContext db,
    AppSettingsService settings,
    TraefikUserMiddlewareService userMiddlewares)
{
    public async Task<TraefikRenderResult> RenderAsync(CancellationToken cancellationToken = default)
    {
        var resources = await db.Resources.AsNoTracking().ToListAsync(cancellationToken);
        var defs = resources.Select(x => new ResourceDefinition(
            x.Id, x.Name, x.Slug, Enum.Parse<ResourceKind>(x.Kind, ignoreCase: true),
            x.Enabled, x.IsSystem, x.Domain, x.TargetScheme, x.TargetHost, x.TargetPort,
            x.PathPrefix, x.PathRewrite,
            ForwardAuthPolicyMapping.Parse(x.ForwardAuthPolicy),
            ParseWafMode(x.WafMode),
            TraefikUserMiddlewareService.ParseExtraMiddlewares(x.ExtraMiddlewaresJson))).ToList();
        var appSettings = await settings.GetOrCreateAsync(cancellationToken);
        var options = new TraefikRenderOptions(
            AdminDomain: appSettings.AdminDomain ?? "hashi.local");
        var userYaml = await userMiddlewares.GetAppliedYamlAsync(cancellationToken);
        return TraefikConfigRenderer.Render(defs, options, userYaml);
    }

    public async Task<TraefikHostStateResponse> GetHostStateAsync(
        Guid connectionId,
        CancellationToken cancellationToken = default)
    {
        var render = await RenderAsync(cancellationToken);
        var middleware = await userMiddlewares.GetAsync(cancellationToken);
        var state = await db.TraefikHostStates.AsNoTracking()
            .SingleOrDefaultAsync(x => x.ConnectionId == connectionId, cancellationToken);
        var hasPending = state?.LastAppliedContentHash is null
            || !string.Equals(state.LastAppliedContentHash, render.ContentHash, StringComparison.Ordinal);
        return new TraefikHostStateResponse(
            connectionId,
            state?.LastAppliedContentHash,
            render.ContentHash,
            state?.LastAppliedAtUtc,
            !string.IsNullOrWhiteSpace(state?.LastBackupStaticYaml),
            hasPending,
            middleware.LastParseError);
    }

    private static WafMode ParseWafMode(string value) => value.ToLowerInvariant() switch
    {
        "off" => WafMode.Off,
        "on" or "block" => WafMode.On,
        _ => WafMode.DetectOnly,
    };
}

public sealed class FirewallPlatformService
{
    public FirewallRenderResponse Render(FirewallRenderRequest request)
    {
        var script = FirewallScriptRenderer.Render(new FirewallHostDefinition(
            Guid.NewGuid(),
            request.Name,
            request.Domain,
            request.ManagedSubnets,
            request.LinkedTraefikHost,
            request.InternalTraefikIp));
        return new FirewallRenderResponse(script);
    }
}
