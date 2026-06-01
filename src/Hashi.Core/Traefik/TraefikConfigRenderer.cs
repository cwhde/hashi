using Hashi.Core.Hosting;
using Hashi.Core.Resources;
using Hashi.Core.Security;

namespace Hashi.Core.Traefik;

internal static class HashiInternalUrlDefaults
{
    public const string BaseUrl = "http://127.0.0.1:" + HashiPorts.DefaultAdminText;
    public const string ForwardAuthUrl = BaseUrl + "/api/edge-auth/forward";
    public const string HealthUrl = BaseUrl + "/api/health";
}

public sealed record TraefikDynamicFiles(
    string CoreYaml,
    string HttpResourcesYaml,
    string StreamResourcesYaml,
    string UserMiddlewaresYaml,
    string SecurityYaml,
    string HealthYaml);

public sealed record TraefikRenderResult(
    string StaticConfigYaml,
    TraefikDynamicFiles DynamicFiles,
    string ContentHash);

public sealed record TraefikRenderOptions(
    string? AcmeEmail = null,
    string? AcmeEabKeyId = null,
    string? AcmeEabHmac = null,
    string? DnsProviderName = null,
    int DnsChallengeDelaySeconds = 30,
    IReadOnlyList<string>? AcmeResolvers = null,
    string AdminDomain = "hashi.local",
    string HashiForwardAuthUrl = HashiInternalUrlDefaults.ForwardAuthUrl,
    string HashiHealthUrl = HashiInternalUrlDefaults.HealthUrl,
    IReadOnlySet<(int Port, string Protocol)>? ConfirmedStreamPorts = null);

public static class TraefikConfigRenderer
{
    public static TraefikRenderResult Render(
        IReadOnlyList<ResourceDefinition> resources,
        TraefikRenderOptions? options = null,
        string? userMiddlewaresYaml = null)
    {
        options ??= new TraefikRenderOptions();
        var enabled = resources.Where(r => r.Enabled).ToList();
        var httpResources = enabled.Where(r => r.Kind is ResourceKind.Http or ResourceKind.Https or ResourceKind.H2c).ToList();
        var streamResources = enabled.Where(r => r.Kind is ResourceKind.Tcp or ResourceKind.Udp).ToList();
        var confirmedPorts = options.ConfirmedStreamPorts;
        if (confirmedPorts is not null)
        {
            streamResources = streamResources
                .Where(r => confirmedPorts.Contains((r.EffectivePublicPort, r.Kind == ResourceKind.Udp ? "udp" : "tcp")))
                .ToList();
        }
        var userMiddlewares = NormalizeUserMiddlewaresYaml(userMiddlewaresYaml);

        var staticYaml = RenderStaticConfig(options, streamResources);
        var dynamic = new TraefikDynamicFiles(
            RenderCoreMiddlewares(options),
            RenderHttpResources(httpResources, options),
            RenderStreamResources(streamResources),
            userMiddlewares,
            RenderSecurity(httpResources),
            RenderHealth(options));

        var hashInput = staticYaml
            + dynamic.CoreYaml
            + dynamic.HttpResourcesYaml
            + dynamic.StreamResourcesYaml
            + dynamic.UserMiddlewaresYaml
            + dynamic.SecurityYaml
            + dynamic.HealthYaml;
        var hash = Convert.ToHexString(System.Security.Cryptography.SHA256.HashData(
            System.Text.Encoding.UTF8.GetBytes(hashInput))).ToLowerInvariant();

        return new TraefikRenderResult(staticYaml, dynamic, hash);
    }

    private static string RenderStaticConfig(TraefikRenderOptions options, IReadOnlyList<ResourceDefinition> streamResources)
    {
        var entryPoints = """
              web:
                address: ":80"
                http:
                  redirections:
                    entryPoint:
                      to: websecure
                      scheme: https
              websecure:
                address: ":443"
            """;

        foreach (var resource in streamResources)
        {
            var protocol = resource.Kind == ResourceKind.Udp ? "udp" : "tcp";
            var entryName = resource.Kind == ResourceKind.Udp ? $"{resource.Slug}-udp" : $"{resource.Slug}-tcp";
            entryPoints += $"""

                  {entryName}:
                    address: ":{resource.EffectivePublicPort}/{protocol}"
                """;
        }

        var acme = string.IsNullOrWhiteSpace(options.AcmeEmail)
            ? string.Empty
            : RenderAcmeBlock(options);

        return $$"""
            entryPoints:
            {{entryPoints}}
            providers:
              file:
                directory: /etc/hashi/traefik/dynamic
                watch: true
            {{acme}}
            log:
              level: INFO
              format: json
              filePath: /var/log/hashi/traefik/traefik.log
            accessLog:
              format: json
              filePath: /var/log/hashi/traefik/access.log
              filters:
                statusCodes:
                  - "200-599"
              fields:
                headers:
                  defaultMode: drop
                  names:
                    User-Agent: keep
                    X-Forwarded-For: keep
            ping:
              entryPoint: web
            api:
              dashboard: false
            experimental:
              plugins:
                coraza:
                  moduleName: github.com/jcchavezs/coraza-http-wasm-traefik
                  version: v0.6.0
            """;
    }

    private static string RenderCoreMiddlewares(TraefikRenderOptions options) => $$"""
        http:
          middlewares:
            hashi-redirect-https:
              redirectScheme:
                scheme: https
                permanent: true
            hashi-security-headers:
              headers:
                stsSeconds: 31536000
                stsIncludeSubdomains: true
                contentTypeNosniff: true
                frameDeny: true
                browserXssFilter: true
            hashi-compress:
              compress: {}
            hashi-forward-auth:
              forwardAuth:
                address: "{{options.HashiForwardAuthUrl}}"
                trustForwardHeader: true
                authResponseHeaders:
                  - X-Hashi-User
            hashi-forward-auth-strict:
              forwardAuth:
                address: "{{options.HashiForwardAuthUrl}}?mode=strict"
                trustForwardHeader: true
                authResponseHeaders:
                  - X-Hashi-User
            hashi-forward-auth-observe:
              forwardAuth:
                address: "{{options.HashiForwardAuthUrl}}?mode=observe"
                trustForwardHeader: true
                authResponseHeaders:
                  - X-Hashi-User
            hashi-rate-limit:
              rateLimit:
                average: 100
                burst: 200
        """;

    private static string RenderHttpResources(IReadOnlyList<ResourceDefinition> resources, TraefikRenderOptions options)
    {
        if (resources.Count == 0)
        {
            return "http:\n  routers: {}\n  services: {}\n";
        }

        var rewriteMiddlewares = resources.SelectMany(r =>
        {
            if (r.Routes is { Count: > 0 })
            {
                return r.Routes.Where(route => route.Enabled && !string.IsNullOrWhiteSpace(route.RewriteValue))
                    .Select(route => (Name: $"{r.Slug}-route-{route.Priority}-rewrite", Value: route.RewriteValue!, Mode: route.RewriteMode));
            }

            if (!string.IsNullOrWhiteSpace(r.PathRewrite))
            {
                return [(Name: $"{r.Slug}-rewrite", Value: r.PathRewrite!, Mode: (string?)null)];
            }

            return [];
        });

        var middlewareEntries = rewriteMiddlewares.Select(x => RenderRewriteMiddleware(x.Name, x.Value, x.Mode)).ToList();
        var middlewareBlock = middlewareEntries.Count > 0
            ? "  middlewares:\n" + string.Join('\n', middlewareEntries) + "\n"
            : string.Empty;

        var routers = string.Join('\n', resources.SelectMany(RenderHttpRouters));

        var serviceKeys = resources.SelectMany(r =>
            r.Routes is { Count: > 0 }
                ? r.Routes.Where(route => route.Enabled).Select(route => (Resource: r, Route: (ResourceRouteDefinition?)route))
                : [(Resource: r, Route: (ResourceRouteDefinition?)null)])
            .DistinctBy(x => x.Route is null ? x.Resource.Slug : $"{x.Resource.Slug}-{x.Route.Priority}");

        var services = string.Join('\n', serviceKeys.Select(x =>
        {
            var scheme = x.Route?.TargetScheme ?? x.Resource.TargetScheme;
            var host = x.Route?.TargetHost ?? x.Resource.TargetHost;
            var port = x.Route?.TargetPort ?? x.Resource.TargetPort;
            var serviceName = x.Route is null ? x.Resource.Slug : $"{x.Resource.Slug}-route-{x.Route.Priority}";
            return $$"""
                  {{serviceName}}:
                    loadBalancer:
                      servers:
                        - url: "{{scheme}}://{{host}}:{{port}}"
                """;
        }));

        return $"http:\n{middlewareBlock}  routers:\n{routers}\n  services:\n{services}\n";
    }

    private static IEnumerable<string> RenderHttpRouters(ResourceDefinition resource)
    {
        if (resource.Routes is { Count: > 0 })
        {
            foreach (var route in resource.Routes.Where(x => x.Enabled).OrderByDescending(x => x.Priority))
            {
                yield return RenderHttpRouter(resource, route);
            }

            yield break;
        }

        yield return RenderHttpRouter(resource, null);
    }

    private static string RenderHttpRouter(ResourceDefinition resource, ResourceRouteDefinition? route)
    {
        var middlewares = BuildResourceMiddlewares(resource, route?.ExtraMiddlewares);
        var pathPrefix = route?.PathValue ?? resource.PathPrefix;
        var pathMatchType = route?.PathMatchType ?? (string.IsNullOrWhiteSpace(resource.PathPrefix) ? null : "prefix");
        var rule = BuildPathRule(resource.Domain, pathMatchType, pathPrefix);
        var routerName = route is null ? resource.Slug : $"{resource.Slug}-route-{route.Priority}";
        var serviceName = route is null ? resource.Slug : $"{resource.Slug}-route-{route.Priority}";
        var rewriteValue = route?.RewriteValue ?? resource.PathRewrite;
        var rewriteMode = route?.RewriteMode;
        var lines = new List<string>
        {
            $"    {routerName}:",
            $"      rule: {rule}",
        };
        if (route is not null)
        {
            lines.Add($"      priority: {route.Priority}");
        }

        lines.Add("      entryPoints:");
        lines.Add("        - websecure");
        lines.Add("      middlewares:");

        lines.AddRange(middlewares.Select(m => $"        - {m}"));
        if (!string.IsNullOrWhiteSpace(rewriteValue))
        {
            lines.Add($"        - {routerName}-rewrite");
        }

        lines.Add($"      service: {serviceName}");
        if (resource.Kind == ResourceKind.Https)
        {
            lines.Add("      tls:");
            lines.Add("        certResolver: gts");
        }

        return string.Join('\n', lines);
    }

    private static string RenderRewriteMiddleware(string name, string value, string? mode)
    {
        var rewriteMode = (mode ?? "replace_path").ToLowerInvariant();
        if (rewriteMode == "regex")
        {
            var (regex, replacement) = SplitRegexRewrite(value);
            return $$"""
                  {{name}}:
                    replacePathRegex:
                      regex: "{{regex}}"
                      replacement: "{{replacement}}"
                """;
        }

        return rewriteMode == "strip_prefix"
            ? $$"""
                  {{name}}:
                    stripPrefix:
                      prefixes:
                        - "{{value}}"
                """
            : $$"""
                  {{name}}:
                    replacePath:
                      path: "{{value}}"
                """;
    }

    private static (string Regex, string Replacement) SplitRegexRewrite(string value)
    {
        var separatorIndex = value.IndexOf("=>", StringComparison.Ordinal);
        if (separatorIndex < 0)
        {
            return (value.Trim(), "/");
        }

        var regex = value[..separatorIndex].Trim();
        var replacement = value[(separatorIndex + 2)..].Trim();
        return (regex.Length == 0 ? "^/(.*)" : regex, replacement.Length == 0 ? "/" : replacement);
    }

    private static string BuildPathRule(string? domain, string? pathMatchType, string? pathValue)
    {
        var hostRule = string.IsNullOrWhiteSpace(domain) ? "HostRegexp(`{host:.+}`)" : $"Host(`{domain}`)";
        if (string.IsNullOrWhiteSpace(pathValue))
        {
            return hostRule;
        }

        var pathRule = (pathMatchType ?? "prefix").ToLowerInvariant() switch
        {
            "exact" => $"Path(`{pathValue}`)",
            "regex" => $"PathRegexp(`{pathValue}`)",
            _ => $"PathPrefix(`{pathValue}`)",
        };
        return $"{hostRule} && {pathRule}";
    }

    private static string RenderStreamResources(IReadOnlyList<ResourceDefinition> resources)
    {
        var tcpResources = resources.Where(r => r.Kind == ResourceKind.Tcp).ToList();
        var udpResources = resources.Where(r => r.Kind == ResourceKind.Udp).ToList();
        if (tcpResources.Count == 0 && udpResources.Count == 0)
        {
            return "tcp: {}\nudp: {}\n";
        }

        var tcpRouters = string.Join('\n', tcpResources.Select(r =>
            $$"""
                  {{r.Slug}}:
                    entryPoints:
                      - {{r.Slug}}-tcp
                    rule: HostSNI(`*`)
                    service: {{r.Slug}}
                """));
        var tcpServices = string.Join('\n', tcpResources.Select(r =>
            $$"""
                  {{r.Slug}}:
                    loadBalancer:
                      servers:
                        - address: "{{r.TargetHost}}:{{r.TargetPort}}"
                """));
        var udpRouters = string.Join('\n', udpResources.Select(r =>
            $$"""
                  {{r.Slug}}:
                    entryPoints:
                      - {{r.Slug}}-udp
                    service: {{r.Slug}}
                """));
        var udpServices = string.Join('\n', udpResources.Select(r =>
            $$"""
                  {{r.Slug}}:
                    loadBalancer:
                      servers:
                        - address: "{{r.TargetHost}}:{{r.TargetPort}}"
                """));

        return string.Join('\n',
            "tcp:",
            RenderMapSection("routers", tcpRouters),
            RenderMapSection("services", tcpServices),
            "udp:",
            RenderMapSection("routers", udpRouters),
            RenderMapSection("services", udpServices)) + "\n";
    }

    private static string RenderMapSection(string name, string content)
        => string.IsNullOrWhiteSpace(content)
            ? $"  {name}: {{}}"
            : $"  {name}:\n{IndentBlock(content, 2)}";

    private static string IndentBlock(string block, int spaces)
    {
        var prefix = new string(' ', spaces);
        return string.Join('\n', block.Split('\n').Select(line => prefix + line));
    }

    private static string RenderSecurity(IReadOnlyList<ResourceDefinition> resources)
    {
        var wafBlocks = string.Join('\n', resources
            .Where(r => r.WafMode != WafMode.Off)
            .Select(r => WafMiddlewareRenderer.RenderCorazaMiddleware(r.Slug, r.WafMode).TrimEnd()));
        return string.IsNullOrEmpty(wafBlocks) ? "http:\n  middlewares: {}\n" : wafBlocks + "\n";
    }

    private static IReadOnlyList<string> BuildResourceMiddlewares(
        ResourceDefinition resource,
        IReadOnlyList<string>? routeMiddlewares = null)
    {
        var chain = new List<string>
        {
            "hashi-redirect-https",
            "hashi-security-headers",
            "hashi-compress",
        };

        if (resource.WafMode != WafMode.Off)
        {
            chain.Add($"{resource.Slug}-waf");
        }

        if (resource.ForwardAuth != ForwardAuthPolicy.Off)
        {
            chain.Add(resource.ForwardAuth switch
            {
                ForwardAuthPolicy.SsoRequired => "hashi-forward-auth-strict",
                ForwardAuthPolicy.Observe => "hashi-forward-auth-observe",
                _ => "hashi-forward-auth",
            });
        }

        chain.Add("hashi-rate-limit");
        if (routeMiddlewares is not null)
        {
            foreach (var extra in routeMiddlewares.Where(x => !string.IsNullOrWhiteSpace(x)))
            {
                if (!chain.Contains(extra, StringComparer.Ordinal))
                {
                    chain.Add(extra);
                }
            }
        }

        if (resource.ExtraMiddlewares is not null)
        {
            foreach (var extra in resource.ExtraMiddlewares.Where(x => !string.IsNullOrWhiteSpace(x)))
            {
                if (!chain.Contains(extra, StringComparer.Ordinal))
                {
                    chain.Add(extra);
                }
            }
        }

        return chain;
    }

    private static string NormalizeUserMiddlewaresYaml(string? yaml)
        => TraefikUserMiddlewareParser.Parse(yaml).NormalizedYaml;

    private static string RenderAcmeBlock(TraefikRenderOptions options)
    {
        var eabBlock = string.IsNullOrWhiteSpace(options.AcmeEabKeyId) || string.IsNullOrWhiteSpace(options.AcmeEabHmac)
            ? string.Empty
            : $$"""

                      externalAccountBinding:
                        keyID: {{options.AcmeEabKeyId}}
                        hmacEncoded: {{options.AcmeEabHmac}}
                """;

        var resolverLines = (options.AcmeResolvers ?? ["1.1.1.1:53", "8.8.8.8:53"])
            .Select(r => $"                      - \"{r}\"");
        var resolvers = string.Join('\n', resolverLines);

        return $$"""
            certificatesResolvers:
              gts:
                acme:
                  email: {{options.AcmeEmail}}
                  storage: /var/lib/hashi/traefik/acme.json
                  caServer: https://dv.acme-v02.api.pki.goog/directory
                  dnsChallenge:
                    provider: hetzner
                    delayBeforeCheck: {{options.DnsChallengeDelaySeconds}}s
                    resolvers:
            {{resolvers}}{{eabBlock}}
            """;
    }

    private static string RenderHealth(TraefikRenderOptions options) => $$"""
        http:
          routers:
            hashi-health:
              rule: Host(`{{options.AdminDomain}}`) && PathPrefix(`/_hashi/health`)
              entryPoints:
                - websecure
              service: hashi-health
              tls:
                certResolver: gts
          services:
            hashi-health:
              loadBalancer:
                servers:
                  - url: "{{options.HashiHealthUrl}}"
        """;
}
