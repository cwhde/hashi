using System.Net;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Xunit;

namespace Hashi.UnitTests;

public sealed class EdgeAuthServiceTests
{
    [Fact]
    public async Task Edge_rule_deny_takes_precedence_over_resource_policy()
    {
        await using var db = CreateDb();
        db.EdgeAuthRules.Add(new EdgeAuthRuleEntity
        {
            Priority = 1,
            MatchJson = """{"host":"app.example.com"}""",
            Action = "deny",
        });
        db.Resources.Add(Resource("app.example.com", "sso_required"));
        await SeedProviderAsync(db);
        await db.SaveChangesAsync();

        var result = await Evaluate(db, "app.example.com", "/");

        Assert.Equal("deny", result.Decision);
    }

    [Fact]
    public async Task Observe_mode_allows_without_session()
    {
        await using var db = CreateDb();
        db.Resources.Add(Resource("app.example.com", "sso_required"));
        await SeedProviderAsync(db);
        await db.SaveChangesAsync();

        var result = await Evaluate(db, "app.example.com", "/", mode: "observe");

        Assert.Equal("allow", result.Decision);
    }

    [Fact]
    public async Task Blocklist_entry_denies_before_policy()
    {
        await using var db = CreateDb();
        db.BlocklistEntries.Add(new BlocklistEntryEntity { ClientIp = "203.0.113.10", Reason = "abuse" });
        db.Resources.Add(Resource("app.example.com", "sso_required"));
        await SeedProviderAsync(db);
        await db.SaveChangesAsync();

        var result = await Evaluate(db, "app.example.com", "/", clientIp: IPAddress.Parse("203.0.113.10"));

        Assert.Equal("deny", result.Decision);
    }

    [Fact]
    public async Task Sso_required_challenges_anonymous_traffic()
    {
        await using var db = CreateDb();
        db.Resources.Add(Resource("app.example.com", "sso_required"));
        await SeedProviderAsync(db);
        await db.SaveChangesAsync();

        var result = await Evaluate(db, "app.example.com", "/dashboard");

        Assert.Equal("challenge", result.Decision);
        Assert.Contains("returnUrl=", result.RedirectUrl, StringComparison.Ordinal);
        Assert.Contains("app.example.com%2Fdashboard", result.RedirectUrl, StringComparison.Ordinal);
    }

    [Fact]
    public async Task Sso_required_denies_when_no_oidc_provider_is_enabled()
    {
        await using var db = CreateDb();
        db.Resources.Add(Resource("app.example.com", "sso_required"));
        await db.SaveChangesAsync();

        var result = await Evaluate(db, "app.example.com", "/dashboard");

        Assert.Equal("deny", result.Decision);
    }

    [Fact]
    public async Task Valid_edge_session_allows_sso_required_resource()
    {
        await using var db = CreateDb();
        var providerId = await SeedProviderAsync(db);
        db.Resources.Add(Resource("app.example.com", "sso_required"));
        await db.SaveChangesAsync();

        const string sessionKey = "test-session";
        db.EdgeSessions.Add(new EdgeSessionEntity
        {
            SessionKey = sessionKey,
            OidcProviderId = providerId,
            Subject = "user",
            ExpiresAtUtc = DateTimeOffset.UtcNow.AddHours(1),
        });
        await db.SaveChangesAsync();

        var result = await Evaluate(db, "app.example.com", "/", edgeSessionKey: sessionKey);

        Assert.Equal("allow", result.Decision);
    }

    [Fact]
    public async Task Valid_edge_session_satisfies_matching_resource_auth_rule()
    {
        await using var db = CreateDb();
        var providerId = await SeedProviderAsync(db);
        var resource = Resource("app.example.com", "sso_required");
        db.Resources.Add(resource);
        db.ResourceRules.Add(new ResourceRuleEntity
        {
            ResourceId = resource.Id,
            MatchType = "path",
            MatchValue = "/",
            Action = "pass_to_auth",
        });
        const string sessionKey = "test-session";
        db.EdgeSessions.Add(new EdgeSessionEntity
        {
            SessionKey = sessionKey,
            OidcProviderId = providerId,
            Subject = "user",
            ExpiresAtUtc = DateTimeOffset.UtcNow.AddHours(1),
        });
        await db.SaveChangesAsync();

        var result = await Evaluate(db, "app.example.com", "/", edgeSessionKey: sessionKey);

        Assert.Equal("allow", result.Decision);
    }

    [Fact]
    public async Task Resource_auth_rule_denies_when_no_oidc_provider_is_enabled()
    {
        await using var db = CreateDb();
        var resource = Resource("app.example.com", "adaptive");
        db.Resources.Add(resource);
        db.ResourceRules.Add(new ResourceRuleEntity
        {
            ResourceId = resource.Id,
            MatchType = "path",
            MatchValue = "/admin",
            Action = "pass_to_auth",
        });
        await db.SaveChangesAsync();

        var result = await Evaluate(db, "app.example.com", "/admin");

        Assert.Equal("deny", result.Decision);
    }

    [Fact]
    public async Task Adaptive_allows_anonymous_until_abuse_bucket_challenges()
    {
        await using var db = CreateDb();
        db.Resources.Add(Resource("app.example.com", "adaptive"));
        db.AbuseBuckets.Add(new AbuseBucketEntity
        {
            ClientIp = "198.51.100.20",
            State = "challenge",
        });
        await SeedProviderAsync(db);
        await db.SaveChangesAsync();

        var result = await Evaluate(db, "app.example.com", "/", clientIp: IPAddress.Parse("198.51.100.20"));

        Assert.Equal("challenge", result.Decision);
    }

    [Fact]
    public async Task Adaptive_blocks_when_abuse_bucket_is_block()
    {
        await using var db = CreateDb();
        db.Resources.Add(Resource("app.example.com", "adaptive"));
        db.AbuseBuckets.Add(new AbuseBucketEntity
        {
            ClientIp = "198.51.100.21",
            State = "block",
        });
        await SeedProviderAsync(db);
        await db.SaveChangesAsync();

        var result = await Evaluate(db, "app.example.com", "/", clientIp: IPAddress.Parse("198.51.100.21"));

        Assert.Equal("deny", result.Decision);
    }

    [Fact]
    public async Task Strict_mode_overrides_adaptive_resource_policy()
    {
        await using var db = CreateDb();
        db.Resources.Add(Resource("app.example.com", "adaptive"));
        await SeedProviderAsync(db);
        await db.SaveChangesAsync();

        var result = await Evaluate(db, "app.example.com", "/", mode: "strict");

        Assert.Equal("challenge", result.Decision);
    }

    [Fact]
    public async Task Off_policy_allows_without_session()
    {
        await using var db = CreateDb();
        db.Resources.Add(Resource("app.example.com", "off"));
        await SeedProviderAsync(db);
        await db.SaveChangesAsync();

        var result = await Evaluate(db, "app.example.com", "/");

        Assert.Equal("allow", result.Decision);
    }

    [Fact]
    public async Task Rule_matches_cidr_country_and_path()
    {
        await using var db = CreateDb();
        db.EdgeAuthRules.Add(new EdgeAuthRuleEntity
        {
            Priority = 1,
            MatchJson = """
                        {"host":"app.example.com","pathPrefix":"/admin","cidr":"203.0.113.0/24","country":"US","asn":"AS13335"}
                        """,
            Action = "deny",
        });
        await db.SaveChangesAsync();

        var match = await Evaluate(
            db,
            "app.example.com",
            "/admin/users",
            clientIp: IPAddress.Parse("203.0.113.55"),
            countryCode: "US",
            asn: "AS13335");
        var miss = await Evaluate(
            db,
            "app.example.com",
            "/public",
            clientIp: IPAddress.Parse("203.0.113.55"),
            countryCode: "US",
            asn: "AS13335");

        Assert.Equal("deny", match.Decision);
        Assert.Equal("allow", miss.Decision);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private static ResourceEntity Resource(string domain, string forwardAuthPolicy)
        => new()
        {
            Name = domain,
            Slug = domain.Replace('.', '-'),
            Domain = domain,
            ForwardAuthPolicy = forwardAuthPolicy,
        };

    private static async Task<Guid> SeedProviderAsync(HashiDbContext db)
    {
        var provider = new OidcProviderEntity
        {
            Name = "Test IdP",
            Issuer = "https://idp.fake.local",
            ClientId = "hashi-edge",
            ClientSecretId = Guid.NewGuid(),
        };
        db.OidcProviders.Add(provider);
        await db.SaveChangesAsync();
        return provider.Id;
    }

    private static Task<EdgeAuthForwardResponse> Evaluate(
        HashiDbContext db,
        string host,
        string path,
        IPAddress? clientIp = null,
        string? countryCode = null,
        string? regionCode = null,
        string? asn = null,
        string? edgeSessionKey = null,
        string? mode = null)
    {
        var geoIp = new GeoIpLookupService(new Microsoft.Extensions.Configuration.ConfigurationBuilder().Build(), Microsoft.Extensions.Logging.Abstractions.NullLogger<GeoIpLookupService>.Instance);
        var oidc = new OidcEdgeAuthService(
            db,
            new SecretRecordService(db, new VaultSessionState(), new ServiceSyncVaultState()),
            new ServiceCollection().AddHttpClient().BuildServiceProvider().GetRequiredService<IHttpClientFactory>(),
            new AppSettingsService(db),
            new Microsoft.Extensions.Configuration.ConfigurationBuilder()
                .AddInMemoryCollection(new Dictionary<string, string?>
                {
                    ["Hashi:Oidc:AllowUnsignedTestTokens"] = "true",
                })
                .Build());
        var service = new EdgeAuthService(db, geoIp, oidc);
        return service.EvaluateForwardAsync(
            host,
            path,
            clientIp ?? IPAddress.Loopback,
            countryCode,
            regionCode,
            asn,
            edgeSessionKey,
            mode);
    }
}
