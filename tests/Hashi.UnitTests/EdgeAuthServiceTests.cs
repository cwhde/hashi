using System.Net;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Microsoft.EntityFrameworkCore;
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
    public async Task Valid_edge_session_allows_sso_required_resource()
    {
        await using var db = CreateDb();
        db.Resources.Add(Resource("app.example.com", "sso_required"));
        await SeedProviderAsync(db);
        await db.SaveChangesAsync();

        const string sessionKey = "test-session";
        EdgeSessionStore.Set(sessionKey, new EdgeSessionState("user", DateTimeOffset.UtcNow.AddHours(1)));

        var result = await Evaluate(db, "app.example.com", "/", edgeSessionKey: sessionKey);

        Assert.Equal("allow", result.Decision);
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

    private static async Task SeedProviderAsync(HashiDbContext db)
    {
        db.OidcProviders.Add(new OidcProviderEntity
        {
            Name = "Test IdP",
            Issuer = "https://idp.fake.local",
            ClientId = "hashi-edge",
            ClientSecretId = Guid.NewGuid(),
        });
        await db.SaveChangesAsync();
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
        var service = new EdgeAuthService(db, geoIp);
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
