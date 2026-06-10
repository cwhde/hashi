using System.Net;
using Hashi.Core.Resources;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class ResourceRuleEvaluationTests
{
    [Theory]
    [InlineData(ResourceRuleMatchTypeNames.Ip, "203.0.113.10", "203.0.113.10", true)]
    [InlineData(ResourceRuleMatchTypeNames.Ip, "203.0.113.10", "203.0.113.11", false)]
    [InlineData(ResourceRuleMatchTypeNames.Cidr, "203.0.113.0/24", "203.0.113.50", true)]
    [InlineData(ResourceRuleMatchTypeNames.Cidr, "203.0.113.0/24", "198.51.100.50", false)]
    [InlineData(ResourceRuleMatchTypeNames.Path, "/admin", "/admin/settings", true)]
    [InlineData(ResourceRuleMatchTypeNames.Path, "/admin", "/api/users", false)]
    [InlineData(ResourceRuleMatchTypeNames.Country, "US", "US", true)]
    [InlineData(ResourceRuleMatchTypeNames.Country, "US", "DE", false)]
    [InlineData(ResourceRuleMatchTypeNames.Region, "NY", "NY", true)]
    [InlineData(ResourceRuleMatchTypeNames.Region, "NY", "CA", false)]
    [InlineData(ResourceRuleMatchTypeNames.Asn, "13335", "AS13335", true)]
    [InlineData(ResourceRuleMatchTypeNames.Asn, "13335", "AS24940", false)]
    public async Task Resource_rule_matches_based_on_match_type(
        string matchType,
        string matchValue,
        string requestValue,
        bool shouldMatch)
    {
        await using var db = CreateDb();
        var resource = Resource("app.example.com", "off");
        db.Resources.Add(resource);

        var request = BuildRequest(
            matchType switch
            {
                ResourceRuleMatchTypeNames.Ip => requestValue,
                ResourceRuleMatchTypeNames.Cidr => "10.0.0.1",
                _ => "203.0.113.10",
            },
            "app.example.com",
            matchType == ResourceRuleMatchTypeNames.Path ? requestValue : "/",
            country: matchType == ResourceRuleMatchTypeNames.Country ? requestValue : null,
            region: matchType == ResourceRuleMatchTypeNames.Region ? requestValue : null,
            asn: matchType == ResourceRuleMatchTypeNames.Asn ? requestValue : null);

        db.ResourceRules.Add(new ResourceRuleEntity
        {
            ResourceId = resource.Id,
            MatchType = matchType,
            MatchValue = matchValue,
            Action = "allow",
        });
        await db.SaveChangesAsync();

        var result = await CreateDecisionService(db).DecideForwardAuthAsync(request);

        if (shouldMatch)
        {
            Assert.Equal(SecurityDecisionActionNames.AllowUpstream, result.Action);
            Assert.Contains("resource_rule", result.Explanation.Select(x => x.Source));
        }
        else
        {
            Assert.NotEqual("resource_rule", result.Explanation.FirstOrDefault()?.Source);
        }
    }

    [Theory]
    [InlineData("allow", SecurityDecisionActionNames.AllowUpstream)]
    [InlineData("deny", SecurityDecisionActionNames.DenyResourceRule)]
    [InlineData("require_sso", SecurityDecisionActionNames.RequireSso)]
    [InlineData("require_challenge", SecurityDecisionActionNames.RequireChallenge)]
    [InlineData("soft_block", SecurityDecisionActionNames.DenySoftBlock)]
    [InlineData("firewall_block", SecurityDecisionActionNames.DenyFirewallBlocked)]
    [InlineData("block_access", SecurityDecisionActionNames.DenyResourceRule)]
    [InlineData("bypass_auth", SecurityDecisionActionNames.AllowUpstream)]
    [InlineData("adaptive_challenge", SecurityDecisionActionNames.RequireChallenge)]
    public async Task Resource_rule_action_aliases_normalize_correctly(string action, string expectedAction)
    {
        await using var db = CreateDb();
        var resource = Resource("app.example.com", "off");
        db.Resources.Add(resource);
        db.ResourceRules.Add(new ResourceRuleEntity
        {
            ResourceId = resource.Id,
            MatchType = ResourceRuleMatchTypeNames.Path,
            MatchValue = "/",
            Action = action,
        });
        await db.SaveChangesAsync();

        var result = await CreateDecisionService(db).DecideForwardAuthAsync(
            Request("198.51.100.15", "app.example.com", "/"));

        Assert.Equal(expectedAction, result.Action);
    }

    [Fact]
    public async Task Resource_rule_priority_determines_evaluation_order()
    {
        await using var db = CreateDb();
        var resource = Resource("app.example.com", "off");
        db.Resources.Add(resource);
        db.ResourceRules.AddRange(
            new ResourceRuleEntity
            {
                ResourceId = resource.Id,
                MatchType = ResourceRuleMatchTypeNames.Path,
                MatchValue = "/",
                Action = "allow",
                Priority = 0,
            },
            new ResourceRuleEntity
            {
                ResourceId = resource.Id,
                MatchType = ResourceRuleMatchTypeNames.Path,
                MatchValue = "/",
                Action = "deny",
                Priority = 10,
            });
        await db.SaveChangesAsync();

        var result = await CreateDecisionService(db).DecideForwardAuthAsync(
            Request("198.51.100.20", "app.example.com", "/"));

        Assert.Equal(SecurityDecisionActionNames.DenyResourceRule, result.Action);
    }

    [Fact]
    public async Task Disabled_resource_rule_is_not_evaluated()
    {
        await using var db = CreateDb();
        var resource = Resource("app.example.com", "off");
        db.Resources.Add(resource);
        db.ResourceRules.Add(new ResourceRuleEntity
        {
            ResourceId = resource.Id,
            MatchType = ResourceRuleMatchTypeNames.Path,
            MatchValue = "/",
            Action = "deny",
            Enabled = false,
        });
        await db.SaveChangesAsync();

        var result = await CreateDecisionService(db).DecideForwardAuthAsync(
            Request("198.51.100.25", "app.example.com", "/"));

        Assert.Equal(SecurityDecisionActionNames.AllowUpstream, result.Action);
    }

    private static SecurityDecisionRequest Request(
        string ip,
        string host = "app.example.com",
        string path = "/",
        string? country = null,
        string? region = null,
        string? asn = null)
        => new(
            host,
            path,
            IPAddress.Parse(ip),
            CountryCode: country,
            RegionCode: region,
            Asn: asn,
            Method: "GET",
            AcceptHeader: "text/html");

    private static SecurityDecisionRequest BuildRequest(
        string ip,
        string host,
        string path,
        string? country = null,
        string? region = null,
        string? asn = null)
        => Request(ip, host, path, country, region, asn);

    private static ResourceEntity Resource(string domain, string forwardAuthPolicy)
        => new()
        {
            Name = domain,
            Slug = domain.Replace('.', '-'),
            Domain = domain,
            ForwardAuthPolicy = forwardAuthPolicy,
        };

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private static SecurityDecisionService CreateDecisionService(HashiDbContext db)
        => new(db, CreateOidcService(db));

    private static OidcEdgeAuthService CreateOidcService(HashiDbContext db)
        => new(
            db,
            new SecretRecordService(db, new VaultSessionState(), new ServiceSyncVaultState()),
            new Microsoft.Extensions.DependencyInjection.ServiceCollection().AddHttpClient().BuildServiceProvider().GetRequiredService<IHttpClientFactory>(),
            new AppSettingsService(db),
            new Microsoft.Extensions.Configuration.ConfigurationBuilder().Build(),
            new Microsoft.AspNetCore.DataProtection.EphemeralDataProtectionProvider());
}
