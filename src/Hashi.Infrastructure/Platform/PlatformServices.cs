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
    TraefikEntryPointService entryPoints,
    GeoIpLookupService geoIp)
{
    private static readonly HashSet<string> GeoIpRuleTypes = new(StringComparer.OrdinalIgnoreCase)
    {
        "country",
        "region",
        "asn",
    };

    public async Task<IReadOnlyList<ResourceEntity>> ListAsync(CancellationToken cancellationToken = default)
        => await db.Resources.AsNoTracking().OrderBy(x => x.Name).ToListAsync(cancellationToken);

    public async Task<ResourceEntity> CreateAsync(CreateResourceRequest request, CancellationToken cancellationToken = default)
    {
        ValidateGeoIpRules(request.Rules);
        var domainMode = NormalizeCreateDomainMode(request.DomainMode, request.Domain);
        var rootDomain = await GetRootDomainAsync(cancellationToken);
        ValidateDomain(domainMode, request.Domain, request.Name, rootDomain);
        ValidateRewrite(request.PathRewriteMode, request.PathRewrite, request.PathPrefix, request.Routes);
        var monitoringHint = NormalizeMonitoringProtocolHint(request.MonitoringProtocolHint);

        if (request.Kind is "tcp" or "udp")
        {
            await EnsureStreamPortConfirmedOrPendingAsync(request.PublicPort ?? request.TargetPort, request.Kind, cancellationToken);
        }

        var entity = new ResourceEntity
        {
            Name = request.Name,
            Slug = ResourceSlug.Normalize(request.Name),
            Kind = request.Kind,
            DomainMode = domainMode,
            Domain = NormalizeStoredDomain(domainMode, request.Domain),
            TargetScheme = request.TargetScheme,
            TargetHost = request.TargetHost,
            TargetPort = request.TargetPort,
            PublicPort = request.PublicPort,
            TcpProxyProtocolEnabled = NormalizeTcpProxyProtocolEnabled(request.Kind, request.TcpProxyProtocolEnabled),
            MonitoringProtocolHint = monitoringHint,
            DashboardEnabled = request.DashboardEnabled,
            StatusEnabled = request.StatusEnabled,
            FirewallHostId = request.FirewallHostId,
            PulseAgentId = request.PulseAgentId,
            PathPrefix = request.PathPrefix,
            PathRewriteMode = NormalizeRewriteMode(request.PathRewriteMode),
            PathRewrite = request.PathRewrite,
            ForwardAuthPolicy = request.ForwardAuthPolicy ?? "adaptive",
            WafMode = request.WafMode ?? "detect_only",
            ExtraMiddlewaresJson = SerializeExtraMiddlewares(request.ExtraMiddlewares),
            WafExclusionsJson = SerializeWafExclusions(request.WafExclusions),
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

        ValidateGeoIpRules(request.Rules);
        var rootDomain = await GetRootDomainAsync(cancellationToken);

        if (request.Name is not null)
        {
            entity.Name = request.Name;
        }

        if (request.Enabled is bool enabled)
        {
            entity.Enabled = enabled;
        }

        var nextDomainMode = NormalizeUpdateDomainMode(request.DomainMode, entity.DomainMode);
        var nextDomain = request.ClearDomain
            ? null
            : request.Domain is not null
                ? request.Domain
                : entity.Domain;
        ValidateDomain(nextDomainMode, nextDomain, entity.Name, rootDomain);
        entity.DomainMode = nextDomainMode;
        if (request.ClearDomain || request.Domain is not null || request.DomainMode is not null)
        {
            entity.Domain = NormalizeStoredDomain(nextDomainMode, nextDomain);
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

        if (request.TcpProxyProtocolEnabled is bool tcpProxyProtocol)
        {
            entity.TcpProxyProtocolEnabled = NormalizeTcpProxyProtocolEnabled(entity.Kind, tcpProxyProtocol);
        }

        if (request.ClearMonitoringProtocolHint)
        {
            entity.MonitoringProtocolHint = null;
        }
        else if (request.MonitoringProtocolHint is not null)
        {
            entity.MonitoringProtocolHint = NormalizeMonitoringProtocolHint(request.MonitoringProtocolHint);
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

        var nextPathRewriteMode = request.ClearPathRewriteMode
            ? null
            : request.PathRewriteMode is not null
                ? request.PathRewriteMode
                : entity.PathRewriteMode;
        var nextPathRewrite = request.ClearPathRewrite
            ? null
            : request.PathRewrite is not null
                ? request.PathRewrite
                : entity.PathRewrite;
        var nextPathPrefix = entity.PathPrefix;
        ValidateRewrite(nextPathRewriteMode, nextPathRewrite, nextPathPrefix, request.Routes);
        if (request.ClearPathRewriteMode)
        {
            entity.PathRewriteMode = null;
        }
        else if (request.PathRewriteMode is not null)
        {
            entity.PathRewriteMode = NormalizeRewriteMode(request.PathRewriteMode);
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

        if (request.ClearWafExclusions)
        {
            entity.WafExclusionsJson = "[]";
        }
        else if (request.WafExclusions is not null)
        {
            entity.WafExclusionsJson = SerializeWafExclusions(request.WafExclusions);
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
            entity.DomainMode,
            entity.Domain,
            ResourceDomainResolver.Resolve(entity.DomainMode, entity.Domain, entity.Slug, await GetRootDomainAsync(cancellationToken)),
            entity.TargetScheme,
            entity.TargetHost,
            entity.TargetPort,
            entity.PublicPort,
            entity.TcpProxyProtocolEnabled,
            entity.MonitoringProtocolHint,
            entity.DashboardEnabled,
            entity.StatusEnabled,
            entity.FirewallHostId,
            entity.PulseAgentId,
            entity.PathPrefix,
            entity.PathRewriteMode,
            entity.PathRewrite,
            entity.ForwardAuthPolicy,
            entity.WafMode,
            TraefikUserMiddlewareService.ParseExtraMiddlewares(entity.ExtraMiddlewaresJson),
            routes.Select(ToRouteResponse).ToList(),
            rules.Select(ToRuleResponse).ToList(),
            ParseWafExclusions(entity.WafExclusionsJson));
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
                RewriteMode = NormalizeRewriteMode(route.RewriteMode),
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

    private void ValidateGeoIpRules(IReadOnlyList<ResourceRuleRequest>? rules)
    {
        if (rules is null || geoIp.IsAvailable)
        {
            return;
        }

        var requiresGeoIp = rules.Any(rule => rule.Enabled && GeoIpRuleTypes.Contains(rule.MatchType));
        if (requiresGeoIp)
        {
            throw new InvalidOperationException("Enabled country, region, and ASN resource rules require a GeoIP database under /data/geoip.");
        }
    }

    private async Task<string?> GetRootDomainAsync(CancellationToken cancellationToken)
    {
        var settings = await db.AppSettings.AsNoTracking().SingleOrDefaultAsync(cancellationToken);
        return settings?.RootDomain;
    }

    private static string NormalizeCreateDomainMode(string? requestedMode, string? domain)
    {
        if (!string.IsNullOrWhiteSpace(requestedMode))
        {
            return ResourceDomainResolver.NormalizeMode(requestedMode);
        }

        var normalizedDomain = ResourceDomainResolver.NormalizeDomain(domain);
        if (string.IsNullOrWhiteSpace(normalizedDomain))
        {
            return ResourceDomainModeNames.Subdomain;
        }

        return normalizedDomain == "@"
            ? ResourceDomainModeNames.Root
            : ResourceDomainModeNames.Custom;
    }

    private static string NormalizeUpdateDomainMode(string? requestedMode, string existingMode)
        => string.IsNullOrWhiteSpace(requestedMode)
            ? ResourceDomainResolver.NormalizeMode(existingMode)
            : ResourceDomainResolver.NormalizeMode(requestedMode);

    private static string? NormalizeStoredDomain(string domainMode, string? domain)
    {
        var normalized = ResourceDomainResolver.NormalizeDomain(domain);
        return domainMode switch
        {
            ResourceDomainModeNames.Root => null,
            ResourceDomainModeNames.Subdomain => normalized,
            _ => normalized,
        };
    }

    private static void ValidateDomain(string domainMode, string? domain, string name, string? rootDomain)
    {
        if (!ResourceDomainModeNames.IsValid(domainMode))
        {
            throw new InvalidOperationException($"Domain mode must be one of: {string.Join(", ", ResourceDomainModeNames.All)}.");
        }

        var normalizedDomain = ResourceDomainResolver.NormalizeDomain(domain);
        var normalizedRoot = ResourceDomainResolver.NormalizeDomain(rootDomain);
        if (domainMode is ResourceDomainModeNames.Root or ResourceDomainModeNames.Subdomain
            && string.IsNullOrWhiteSpace(normalizedRoot))
        {
            throw new InvalidOperationException("Root domain must be configured before using root or subdomain resource domain modes.");
        }

        if (domainMode == ResourceDomainModeNames.Root
            && normalizedDomain is not null
            && normalizedDomain != "@"
            && !string.Equals(normalizedDomain, normalizedRoot, StringComparison.OrdinalIgnoreCase))
        {
            throw new InvalidOperationException("Root domain mode does not accept a custom domain value.");
        }

        if (domainMode == ResourceDomainModeNames.Subdomain
            && normalizedDomain is not null
            && normalizedDomain.Contains('.', StringComparison.Ordinal))
        {
            throw new InvalidOperationException("Subdomain mode accepts only a subdomain label. Use custom mode for a full domain.");
        }

        if (domainMode == ResourceDomainModeNames.Custom && string.IsNullOrWhiteSpace(normalizedDomain))
        {
            throw new InvalidOperationException("Custom domain mode requires a full domain.");
        }

        if (domainMode == ResourceDomainModeNames.Custom && normalizedDomain == "@")
        {
            throw new InvalidOperationException("Use root domain mode for the root domain.");
        }

        if (domainMode == ResourceDomainModeNames.Subdomain && string.IsNullOrWhiteSpace(ResourceSlug.Normalize(name)))
        {
            throw new InvalidOperationException("Subdomain mode requires a resource name that can produce a slug.");
        }
    }

    private static void ValidateRewrite(
        string? pathRewriteMode,
        string? pathRewrite,
        string? pathPrefix,
        IReadOnlyList<ResourceRouteRequest>? routes)
    {
        var normalizedMode = NormalizeRewriteMode(pathRewriteMode);
        if (normalizedMode is not null && string.IsNullOrWhiteSpace(pathRewrite))
        {
            throw new InvalidOperationException("Path rewrite mode requires a path rewrite value.");
        }

        if (normalizedMode == ResourceRewriteModeNames.ReplacePrefix && string.IsNullOrWhiteSpace(pathPrefix))
        {
            throw new InvalidOperationException("Replace-prefix rewrites require a path prefix.");
        }

        if (routes is null)
        {
            return;
        }

        foreach (var route in routes)
        {
            var routeMode = NormalizeRewriteMode(route.RewriteMode);
            if (routeMode is not null && string.IsNullOrWhiteSpace(route.RewriteValue))
            {
                throw new InvalidOperationException("Route rewrite mode requires a route rewrite value.");
            }

            if (routeMode == ResourceRewriteModeNames.ReplacePrefix && string.IsNullOrWhiteSpace(route.PathValue))
            {
                throw new InvalidOperationException("Replace-prefix route rewrites require a route path value.");
            }
        }
    }

    private static string? NormalizeRewriteMode(string? rewriteMode)
    {
        if (string.IsNullOrWhiteSpace(rewriteMode))
        {
            return null;
        }

        var normalized = rewriteMode.Trim().ToLowerInvariant();
        if (!ResourceRewriteModeNames.IsValid(normalized))
        {
            throw new InvalidOperationException($"Rewrite mode must be one of: {string.Join(", ", ResourceRewriteModeNames.All)}.");
        }

        return normalized;
    }

    private static string SerializeExtraMiddlewares(IReadOnlyList<string>? middlewares)
        => JsonSerializer.Serialize(middlewares ?? []);

    private static string SerializeWafExclusions(IReadOnlyList<string>? exclusions)
        => JsonSerializer.Serialize(NormalizeWafExclusions(exclusions));

    private static IReadOnlyList<string> ParseWafExclusions(string? json)
    {
        if (string.IsNullOrWhiteSpace(json))
        {
            return [];
        }

        try
        {
            var values = JsonSerializer.Deserialize<IReadOnlyList<string>>(json);
            return NormalizeWafExclusions(values);
        }
        catch (JsonException)
        {
            return [];
        }
    }

    private static IReadOnlyList<string> NormalizeWafExclusions(IReadOnlyList<string>? exclusions)
        => exclusions?
            .Select(x => x.Trim())
            .Where(x => !string.IsNullOrWhiteSpace(x))
            .Distinct(StringComparer.Ordinal)
            .ToList()
            ?? [];

    public static async Task<IReadOnlyList<ResourceDefinition>> BuildDefinitionsAsync(
        HashiDbContext db,
        CancellationToken cancellationToken = default)
    {
        var resources = await db.Resources.AsNoTracking().ToListAsync(cancellationToken);
        var routes = await db.ResourceRoutes.AsNoTracking().ToListAsync(cancellationToken);
        var rules = await db.ResourceRules.AsNoTracking().ToListAsync(cancellationToken);
        var rootDomain = (await db.AppSettings.AsNoTracking().SingleOrDefaultAsync(cancellationToken))?.RootDomain;
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
                ResourceDomainResolver.Resolve(entity.DomainMode, entity.Domain, entity.Slug, rootDomain),
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
                resourceRules.Count > 0 ? resourceRules : null,
                ParseWafExclusions(entity.WafExclusionsJson),
                entity.DomainMode,
                entity.PathRewriteMode,
                entity.TcpProxyProtocolEnabled,
                entity.MonitoringProtocolHint);
        }).ToList();
    }

    private static bool? NormalizeTcpProxyProtocolEnabled(string kind, bool? enabled)
    {
        if (enabled is null)
        {
            return null;
        }

        if (string.Equals(kind, "tcp", StringComparison.OrdinalIgnoreCase))
        {
            return enabled;
        }

        if (enabled.Value)
        {
            throw new InvalidOperationException("TCP proxy protocol can only be enabled for TCP resources.");
        }

        return null;
    }

    private static string? NormalizeMonitoringProtocolHint(string? hint)
    {
        var normalized = ResourceMonitoringProtocolHintNames.Normalize(hint);
        if (normalized is null)
        {
            return null;
        }

        if (!ResourceMonitoringProtocolHintNames.IsValid(normalized))
        {
            throw new InvalidOperationException($"Monitoring protocol hint must be one of: {string.Join(", ", ResourceMonitoringProtocolHintNames.All)}.");
        }

        return normalized;
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
    TraefikEntryPointService entryPoints,
    HashiInternalUrlResolver internalUrls)
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
            HashiForwardAuthUrl = internalUrls.ResolveUrl(appSettings, "/api/edge-auth/forward"),
            HashiHealthUrl = internalUrls.ResolveUrl(appSettings, "/api/health"),
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
