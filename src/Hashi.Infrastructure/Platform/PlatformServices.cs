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

public sealed class ResourceService(
    HashiDbContext db,
    AuditService audit,
    TraefikEntryPointService entryPoints)
{
    public async Task<IReadOnlyList<ResourceEntity>> ListAsync(CancellationToken cancellationToken = default)
        => await db.Resources.AsNoTracking().OrderBy(x => x.Name).ToListAsync(cancellationToken);

    public async Task<ResourceEntity> CreateAsync(CreateResourceRequest request, CancellationToken cancellationToken = default)
    {
        if (request.Kind is "tcp" or "udp")
        {
            await EnsureStreamPortConfirmedOrPendingAsync(request.PublicPort ?? request.TargetPort, request.Kind, cancellationToken);
        }

        var entity = new ResourceEntity
        {
            Name = request.Name,
            Slug = ResourceSlug.Normalize(request.Name),
            Kind = request.Kind,
            Domain = request.Domain,
            TargetScheme = request.TargetScheme,
            TargetHost = request.TargetHost,
            TargetPort = request.TargetPort,
            PublicPort = request.PublicPort,
            DashboardEnabled = request.DashboardEnabled,
            StatusEnabled = request.StatusEnabled,
            FirewallHostId = request.FirewallHostId,
            PulseAgentId = request.PulseAgentId,
            PathPrefix = request.PathPrefix,
            PathRewrite = request.PathRewrite,
            ForwardAuthPolicy = request.ForwardAuthPolicy ?? "adaptive",
            WafMode = request.WafMode ?? "detect_only",
            ExtraMiddlewaresJson = SerializeExtraMiddlewares(request.ExtraMiddlewares),
        };
        db.Resources.Add(entity);
        await db.SaveChangesAsync(cancellationToken);
        await UpsertRoutesAsync(entity.Id, request.Routes, cancellationToken);
        await UpsertRulesAsync(entity.Id, request.Rules, cancellationToken);
        await entryPoints.SyncForResourceAsync(entity, cancellationToken);
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

        if (entity.IsSystem)
        {
            throw new InvalidOperationException("System resources cannot be updated through the resource API.");
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

        if (request.ClearPublicPort)
        {
            entity.PublicPort = null;
        }
        else if (request.PublicPort is int publicPort)
        {
            entity.PublicPort = publicPort;
        }

        if (request.ForwardAuthPolicy is not null)
        {
            entity.ForwardAuthPolicy = request.ForwardAuthPolicy;
        }

        if (request.WafMode is not null)
        {
            entity.WafMode = request.WafMode;
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

        if (entity.Kind is "tcp" or "udp")
        {
            await EnsureStreamPortConfirmedOrPendingAsync(entity.PublicPort ?? entity.TargetPort, entity.Kind, cancellationToken);
        }

        entity.UpdatedAtUtc = DateTimeOffset.UtcNow;
        await db.SaveChangesAsync(cancellationToken);

        if (request.Routes is not null)
        {
            await UpsertRoutesAsync(entity.Id, request.Routes, cancellationToken);
        }

        if (request.Rules is not null)
        {
            await UpsertRulesAsync(entity.Id, request.Rules, cancellationToken);
        }

        await entryPoints.SyncForResourceAsync(entity, cancellationToken);
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

        await entryPoints.RemoveForResourceAsync(id, cancellationToken);
        db.ResourceRoutes.RemoveRange(await db.ResourceRoutes.Where(x => x.ResourceId == id).ToListAsync(cancellationToken));
        db.ResourceRules.RemoveRange(await db.ResourceRules.Where(x => x.ResourceId == id).ToListAsync(cancellationToken));
        db.Resources.Remove(entity);
        await db.SaveChangesAsync(cancellationToken);
        return true;
    }

    public async Task<IReadOnlyList<ResourceRouteEntity>> GetRoutesAsync(Guid resourceId, CancellationToken cancellationToken = default)
        => await db.ResourceRoutes.AsNoTracking().Where(x => x.ResourceId == resourceId).OrderByDescending(x => x.Priority).ToListAsync(cancellationToken);

    public async Task<IReadOnlyList<ResourceRuleEntity>> GetRulesAsync(Guid resourceId, CancellationToken cancellationToken = default)
        => await db.ResourceRules.AsNoTracking().Where(x => x.ResourceId == resourceId).OrderByDescending(x => x.Priority).ToListAsync(cancellationToken);

    public async Task<ResourceResponse> ToResponseAsync(ResourceEntity entity, CancellationToken cancellationToken = default)
    {
        var routes = await GetRoutesAsync(entity.Id, cancellationToken);
        var rules = await GetRulesAsync(entity.Id, cancellationToken);
        return new ResourceResponse(
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
            entity.PublicPort,
            entity.DashboardEnabled,
            entity.StatusEnabled,
            entity.FirewallHostId,
            entity.PulseAgentId,
            entity.PathPrefix,
            entity.PathRewrite,
            entity.ForwardAuthPolicy,
            entity.WafMode,
            TraefikUserMiddlewareService.ParseExtraMiddlewares(entity.ExtraMiddlewaresJson),
            routes.Select(ToRouteResponse).ToList(),
            rules.Select(ToRuleResponse).ToList());
    }

    public static ResourceRouteResponse ToRouteResponse(ResourceRouteEntity entity) => new(
        entity.Id,
        entity.Enabled,
        entity.Priority,
        entity.PathMatchType,
        entity.PathValue,
        entity.TargetScheme,
        entity.TargetHost,
        entity.TargetPort,
        entity.RewriteMode,
        entity.RewriteValue,
        TraefikUserMiddlewareService.ParseExtraMiddlewares(entity.ExtraMiddlewaresJson));

    public static ResourceRuleResponse ToRuleResponse(ResourceRuleEntity entity) => new(
        entity.Id,
        entity.Enabled,
        entity.Priority,
        entity.Action,
        entity.MatchType,
        entity.MatchValue);

    private async Task EnsureStreamPortConfirmedOrPendingAsync(int port, string protocol, CancellationToken cancellationToken)
    {
        var existing = await db.TraefikEntryPoints
            .SingleOrDefaultAsync(x => x.Port == port && x.Protocol == protocol, cancellationToken);
        if (existing is not null)
        {
            return;
        }

        db.TraefikEntryPoints.Add(new TraefikEntryPointEntity
        {
            Port = port,
            Protocol = protocol,
            Label = $"Pending {protocol}/{port}",
            Confirmed = false,
        });
    }

    private async Task UpsertRoutesAsync(Guid resourceId, IReadOnlyList<ResourceRouteRequest>? routes, CancellationToken cancellationToken)
    {
        if (routes is null)
        {
            return;
        }

        var existing = await db.ResourceRoutes.Where(x => x.ResourceId == resourceId).ToListAsync(cancellationToken);
        db.ResourceRoutes.RemoveRange(existing);
        foreach (var route in routes)
        {
            db.ResourceRoutes.Add(new ResourceRouteEntity
            {
                ResourceId = resourceId,
                Enabled = route.Enabled,
                Priority = route.Priority,
                PathMatchType = route.PathMatchType,
                PathValue = route.PathValue,
                TargetScheme = route.TargetScheme,
                TargetHost = route.TargetHost,
                TargetPort = route.TargetPort,
                RewriteMode = route.RewriteMode,
                RewriteValue = route.RewriteValue,
                ExtraMiddlewaresJson = SerializeExtraMiddlewares(route.ExtraMiddlewares),
            });
        }

        await db.SaveChangesAsync(cancellationToken);
    }

    private async Task UpsertRulesAsync(Guid resourceId, IReadOnlyList<ResourceRuleRequest>? rules, CancellationToken cancellationToken)
    {
        if (rules is null)
        {
            return;
        }

        var existing = await db.ResourceRules.Where(x => x.ResourceId == resourceId).ToListAsync(cancellationToken);
        db.ResourceRules.RemoveRange(existing);
        foreach (var rule in rules)
        {
            db.ResourceRules.Add(new ResourceRuleEntity
            {
                ResourceId = resourceId,
                Enabled = rule.Enabled,
                Priority = rule.Priority,
                Action = rule.Action,
                MatchType = rule.MatchType,
                MatchValue = rule.MatchValue,
            });
        }

        await db.SaveChangesAsync(cancellationToken);
    }

    private static string SerializeExtraMiddlewares(IReadOnlyList<string>? middlewares)
        => JsonSerializer.Serialize(middlewares ?? []);

    public static async Task<IReadOnlyList<ResourceDefinition>> BuildDefinitionsAsync(
        HashiDbContext db,
        CancellationToken cancellationToken = default)
    {
        var resources = await db.Resources.AsNoTracking().ToListAsync(cancellationToken);
        var routes = await db.ResourceRoutes.AsNoTracking().ToListAsync(cancellationToken);
        var rules = await db.ResourceRules.AsNoTracking().ToListAsync(cancellationToken);
        return resources.Select(entity =>
        {
            var resourceRoutes = routes.Where(x => x.ResourceId == entity.Id)
                .OrderByDescending(x => x.Priority)
                .Select(x => new ResourceRouteDefinition(
                    x.Priority,
                    x.PathMatchType,
                    x.PathValue,
                    x.TargetScheme,
                    x.TargetHost,
                    x.TargetPort,
                    x.Enabled,
                    x.RewriteMode,
                    x.RewriteValue,
                    TraefikUserMiddlewareService.ParseExtraMiddlewares(x.ExtraMiddlewaresJson)))
                .ToList();
            var resourceRules = rules.Where(x => x.ResourceId == entity.Id)
                .OrderByDescending(x => x.Priority)
                .Select(x => new ResourceRuleDefinition(x.Priority, x.Action, x.MatchType, x.MatchValue, x.Enabled))
                .ToList();
            return new ResourceDefinition(
                entity.Id,
                entity.Name,
                entity.Slug,
                Enum.Parse<ResourceKind>(entity.Kind, ignoreCase: true),
                entity.Enabled,
                entity.IsSystem,
                entity.Domain,
                entity.TargetScheme,
                entity.TargetHost,
                entity.TargetPort,
                entity.PublicPort,
                entity.PathPrefix,
                entity.PathRewrite,
                ForwardAuthPolicyMapping.Parse(entity.ForwardAuthPolicy),
                ParseWafMode(entity.WafMode),
                TraefikUserMiddlewareService.ParseExtraMiddlewares(entity.ExtraMiddlewaresJson),
                resourceRoutes.Count > 0 ? resourceRoutes : null,
                resourceRules.Count > 0 ? resourceRules : null);
        }).ToList();
    }

    private static WafMode ParseWafMode(string value) => value.ToLowerInvariant() switch
    {
        "off" => WafMode.Off,
        "on" or "block" => WafMode.On,
        _ => WafMode.DetectOnly,
    };
}

public sealed class TraefikPlatformService(
    HashiDbContext db,
    AppSettingsService settings,
    TraefikUserMiddlewareService userMiddlewares,
    CertificateSetupService certificateSetup,
    TraefikEntryPointService entryPoints)
{
    public async Task<TraefikRenderResult> RenderAsync(CancellationToken cancellationToken = default)
    {
        var defs = await ResourceService.BuildDefinitionsAsync(db, cancellationToken);
        var appSettings = await settings.GetOrCreateAsync(cancellationToken);
        var acmeOptions = await certificateSetup.BuildTraefikOptionsAsync(appSettings.AdminDomain ?? "hashi.local", cancellationToken);
        var confirmedPorts = await entryPoints.GetConfirmedPortKeysAsync(cancellationToken);
        var options = acmeOptions with
        {
            AdminDomain = appSettings.AdminDomain ?? "hashi.local",
            ConfirmedStreamPorts = confirmedPorts,
        };
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
        var hasBackup = !string.IsNullOrWhiteSpace(state?.LastBackupStaticYaml)
            || !string.IsNullOrWhiteSpace(state?.LastBackupDynamicYaml);
        return new TraefikHostStateResponse(
            connectionId,
            state?.LastAppliedContentHash,
            render.ContentHash,
            state?.LastAppliedAtUtc,
            hasBackup,
            hasPending,
            middleware.LastParseError);
    }
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
            request.InternalTraefikIp,
            request.PublicIp,
            request.WanInterface,
            request.LxcBridge,
            request.NetBirdEnabled ?? true,
            request.NetBirdInterface ?? "wt0",
            request.NetBirdOverlayCidrs,
            request.NetBirdRoutedCidrs,
            request.NetBirdRoutingPeer ?? false,
            PortForwards: null,
            TrustedPublicIps: null,
            BlockedIps: null,
            RollbackTimerSeconds: request.RollbackTimerSeconds ?? 300));
        return new FirewallRenderResponse(script);
    }
}
