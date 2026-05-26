using Hashi.Core.Resources;
using Hashi.Core.Security;

namespace Hashi.Core.Traefik;

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
    string AdminDomain = "hashi.local",
    string HashiForwardAuthUrl = "http://127.0.0.1:8080/api/edge-auth/forward");

public static class TraefikConfigRenderer
{
    public static TraefikRenderResult Render(
        IReadOnlyList<ResourceDefinition> resources,
        TraefikRenderOptions? options = null)
    {
        options ??= new TraefikRenderOptions();
        var enabled = resources.Where(r => r.Enabled).ToList();
        var httpResources = enabled.Where(r => r.Kind is ResourceKind.Http or ResourceKind.Https or ResourceKind.H2c).ToList();
        var streamResources = enabled.Where(r => r.Kind is ResourceKind.Tcp or ResourceKind.Udp).ToList();

        var staticYaml = RenderStaticConfig(options, streamResources);
        var dynamic = new TraefikDynamicFiles(
            RenderCoreMiddlewares(options),
            RenderHttpResources(httpResources, options),
            RenderStreamResources(streamResources),
            RenderUserMiddlewaresPlaceholder(),
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
            entryPoints += $"""
                  {resource.Slug}-tcp:
                    address: ":{resource.TargetPort}/tcp"
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

        var stripMiddlewares = resources
            .Where(r => !string.IsNullOrWhiteSpace(r.PathRewrite))
            .Select(r =>
                $$"""
                      {{r.Slug}}-strip:
                        replacePath:
                          path: "{{r.PathRewrite}}"
                    """);
        var middlewareBlock = stripMiddlewares.Any()
            ? "  middlewares:\n" + string.Join('\n', stripMiddlewares) + "\n"
            : string.Empty;

        var routers = string.Join('\n', resources.Select(RenderHttpRouter));

        var services = string.Join('\n', resources.Select(r =>
            $$"""
                  {{r.Slug}}:
                    loadBalancer:
                      servers:
                        - url: "{{r.TargetScheme}}://{{r.TargetHost}}:{{r.TargetPort}}"
                """));

        return $"http:\n{middlewareBlock}  routers:\n{routers}\n  services:\n{services}\n";
    }

    private static string RenderHttpRouter(ResourceDefinition resource)
    {
        var middlewares = BuildResourceMiddlewares(resource);
        var rule = string.IsNullOrWhiteSpace(resource.PathPrefix)
            ? $"Host(`{resource.Domain}`)"
            : $"Host(`{resource.Domain}`) && PathPrefix(`{resource.PathPrefix}`)";
        var lines = new List<string>
        {
            $"    {resource.Slug}:",
            $"      rule: {rule}",
            "      entryPoints:",
            "        - websecure",
            "      middlewares:",
        };
        lines.AddRange(middlewares.Select(m => $"        - {m}"));
        if (!string.IsNullOrWhiteSpace(resource.PathRewrite))
        {
            lines.Add($"        - {resource.Slug}-strip");
        }

        lines.Add($"      service: {resource.Slug}");
        if (resource.Kind == ResourceKind.Https)
        {
            lines.Add("      tls:");
            lines.Add("        certResolver: gts");
        }

        return string.Join('\n', lines);
    }

    private static string RenderStreamResources(IReadOnlyList<ResourceDefinition> resources)
    {
        if (resources.Count == 0)
        {
            return "tcp: {}\nudp: {}\n";
        }

        var tcpRouters = string.Join('\n', resources.Where(r => r.Kind == ResourceKind.Tcp).Select(r =>
            $$"""
                  {{r.Slug}}:
                    entryPoints:
                      - {{r.Slug}}-tcp
                    rule: HostSNI(`*`)
                    service: {{r.Slug}}
                """));
        var tcpServices = string.Join('\n', resources.Where(r => r.Kind == ResourceKind.Tcp).Select(r =>
            $$"""
                  {{r.Slug}}:
                    loadBalancer:
                      servers:
                        - address: "{{r.TargetHost}}:{{r.TargetPort}}"
                """));

        return $$"""
            tcp:
              routers:
            {{tcpRouters}}
              services:
            {{tcpServices}}
            """;
    }

    private static string RenderUserMiddlewaresPlaceholder() => """
        http:
          middlewares: {}
        """;

    private static string RenderSecurity(IReadOnlyList<ResourceDefinition> resources)
    {
        var wafBlocks = string.Join('\n', resources
            .Where(r => r.WafMode != WafMode.Off)
            .Select(r => WafMiddlewareRenderer.RenderCorazaMiddleware(r.Slug, r.WafMode).TrimEnd()));
        return string.IsNullOrEmpty(wafBlocks) ? "http:\n  middlewares: {}\n" : wafBlocks + "\n";
    }

    private static IReadOnlyList<string> BuildResourceMiddlewares(ResourceDefinition resource)
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
        return chain;
    }

    private static string RenderAcmeBlock(TraefikRenderOptions options)
    {
        var eabBlock = string.IsNullOrWhiteSpace(options.AcmeEabKeyId) || string.IsNullOrWhiteSpace(options.AcmeEabHmac)
            ? string.Empty
            : $$"""

                      externalAccountBinding:
                        keyID: {{options.AcmeEabKeyId}}
                        hmacEncoded: {{options.AcmeEabHmac}}
                """;

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
                      - "1.1.1.1:53"
                      - "8.8.8.8:53"{{eabBlock}}
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
                  - url: "http://127.0.0.1:8080/api/health"
        """;
}
