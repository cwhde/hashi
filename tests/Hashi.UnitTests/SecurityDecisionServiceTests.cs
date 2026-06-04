using System.Net;
using Hashi.Core.Resources;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.DataProtection;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Xunit;

namespace Hashi.UnitTests;

public sealed class SecurityDecisionServiceTests
{
    [Theory]
    [InlineData(SecuritySubjectTypeNames.Ip, "not-an-ip", false, "")]
    [InlineData(SecuritySubjectTypeNames.Ip, "203.0.113.10", true, "203.0.113.10")]
    [InlineData(SecuritySubjectTypeNames.Cidr, "203.0.113.99/24", true, "203.0.113.0/24")]
    [InlineData(SecuritySubjectTypeNames.Asn, "13335", true, "AS13335")]
    [InlineData(SecuritySubjectTypeNames.Asn, "as24940", true, "AS24940")]
    [InlineData(SecuritySubjectTypeNames.Country, "ch", true, "CH")]
    [InlineData(SecuritySubjectTypeNames.Region, "zh", true, "ZH")]
    public void Security_subject_normalizer_handles_supported_subjects(
        string type,
        string value,
        bool expectedValid,
        string expectedNormalized)
    {
        var valid = SecuritySubjectNormalizer.TryNormalize(type, value, out var subject);

        Assert.Equal(expectedValid, valid);
        if (expectedValid)
        {
            Assert.Equal(expectedNormalized, subject.NormalizedValue);
        }
    }

    [Fact]
    public void Manual_allow_defaults_do_not_bypass_sso_or_challenge()
    {
        var entry = new ManualSecurityEntryEntity();

        Assert.Equal(ManualSecurityEntryTypeNames.Allow, entry.EntryType);
        Assert.True(entry.BypassBlocking);
        Assert.True(entry.BypassAdaptiveEscalation);
        Assert.False(entry.BypassRateLimit);
        Assert.False(entry.BypassChallenge);
        Assert.False(entry.BypassSso);
    }

    [Fact]
    public async Task Manual_block_entry_wins_over_manual_allow()
    {
        await using var db = CreateDb();
        db.ManualSecurityEntries.AddRange(
            ManualEntry("198.51.100.10", ManualSecurityEntryTypeNames.Allow),
            ManualEntry("198.51.100.10", ManualSecurityEntryTypeNames.Block));
        await db.SaveChangesAsync();

        var result = await CreateDecisionService(db).DecideForwardAuthAsync(Request("198.51.100.10"));

        Assert.Equal("deny", result.Decision);
        Assert.Equal(SecurityDecisionActionNames.DenyManualBlock, result.Action);
        Assert.Equal("manual_block", result.Reason);
    }

    [Fact]
    public async Task Manual_allow_bypasses_automatic_blocklist_but_not_sso()
    {
        await using var db = CreateDb();
        db.Resources.Add(Resource("app.example.com", "sso_required"));
        db.ManualSecurityEntries.Add(ManualEntry("198.51.100.11", ManualSecurityEntryTypeNames.Allow));
        db.BlocklistEntries.Add(new BlocklistEntryEntity
        {
            Type = BlocklistTypeNames.Ip,
            Value = "198.51.100.11",
            Reason = "automatic",
            Source = BlocklistSourceNames.Automatic,
            EnforcementMode = BlocklistEnforcementModeNames.Firewall,
        });
        await SeedProviderAsync(db);
        await db.SaveChangesAsync();

        var result = await CreateDecisionService(db).DecideForwardAuthAsync(Request("198.51.100.11", "app.example.com"));

        Assert.Equal(SecurityDecisionActionNames.RequireSso, result.Action);
        Assert.Equal("challenge", result.Decision);
        Assert.Contains("/api/edge-auth/login", result.RedirectUrl, StringComparison.Ordinal);
    }

    [Fact]
    public async Task Firewall_block_state_precedes_resource_rules()
    {
        await using var db = CreateDb();
        var subject = SecuritySubjectNormalizer.NormalizeIp(IPAddress.Parse("198.51.100.12"));
        var entity = new SecuritySubjectEntity
        {
            SubjectType = subject.SubjectType,
            SubjectValue = subject.SubjectValue,
            NormalizedValue = subject.NormalizedValue,
            CurrentState = SecuritySubjectStateNames.FirewallBlocked,
        };
        db.SecuritySubjects.Add(entity);
        db.SecuritySubjectStates.Add(new SecuritySubjectStateEntity { SecuritySubjectId = entity.Id });
        var resource = Resource("app.example.com", "off");
        db.Resources.Add(resource);
        db.ResourceRules.Add(new ResourceRuleEntity
        {
            ResourceId = resource.Id,
            MatchType = ResourceRuleMatchTypeNames.Path,
            MatchValue = "/",
            Action = ResourceRuleActionNames.Allow,
        });
        await db.SaveChangesAsync();

        var result = await CreateDecisionService(db).DecideForwardAuthAsync(Request("198.51.100.12", "app.example.com"));

        Assert.Equal(SecurityDecisionActionNames.DenyFirewallBlocked, result.Action);
        Assert.Equal("deny", result.Decision);
    }

    [Fact]
    public async Task Active_challenge_blocks_upstream_and_counts_ignored_requests()
    {
        await using var db = CreateDb();
        var subject = SecuritySubjectNormalizer.NormalizeIp(IPAddress.Parse("198.51.100.13"));
        var entity = new SecuritySubjectEntity
        {
            SubjectType = subject.SubjectType,
            SubjectValue = subject.SubjectValue,
            NormalizedValue = subject.NormalizedValue,
            CurrentState = SecuritySubjectStateNames.Challenged,
        };
        db.SecuritySubjects.Add(entity);
        db.SecuritySubjectStates.Add(new SecuritySubjectStateEntity
        {
            SecuritySubjectId = entity.Id,
            ChallengeRequired = true,
            ChallengeReason = "rate_limit",
        });
        db.Resources.Add(Resource("app.example.com", "adaptive"));
        await db.SaveChangesAsync();

        var result = await CreateDecisionService(db).DecideForwardAuthAsync(Request("198.51.100.13", "app.example.com"));

        Assert.Equal(SecurityDecisionActionNames.RequireChallenge, result.Action);
        Assert.Equal("challenge", result.Decision);
        Assert.Contains("/api/edge-challenge/start", result.RedirectUrl, StringComparison.Ordinal);
        var state = await db.SecuritySubjectStates.SingleAsync();
        Assert.Equal(1, state.RequestsWhileChallenged);
    }

    [Fact]
    public async Task Api_like_challenge_returns_api_response_mode()
    {
        await using var db = CreateDb();
        var resource = Resource("api.example.com", "off");
        db.Resources.Add(resource);
        db.ResourceRules.Add(new ResourceRuleEntity
        {
            ResourceId = resource.Id,
            MatchType = ResourceRuleMatchTypeNames.Path,
            MatchValue = "/v1",
            Action = "adaptive_challenge",
        });
        await db.SaveChangesAsync();

        var result = await CreateDecisionService(db).DecideForwardAuthAsync(
            Request("198.51.100.14", "api.example.com", "/v1/orders", method: "POST", accept: "application/json"));

        Assert.Equal(SecurityDecisionActionNames.RequireChallenge, result.Action);
        Assert.Equal(SecurityDecisionResponseModeNames.ApiChallenge, result.ResponseMode);
        Assert.Equal(403, result.StatusCode);
        Assert.Null(result.RedirectUrl);
    }

    [Theory]
    [InlineData("allow", SecurityDecisionActionNames.AllowUpstream)]
    [InlineData("block_access", SecurityDecisionActionNames.DenyResourceRule)]
    [InlineData("require_sso", SecurityDecisionActionNames.RequireSso)]
    [InlineData("soft_block", SecurityDecisionActionNames.DenySoftBlock)]
    [InlineData("firewall_block", SecurityDecisionActionNames.DenyFirewallBlocked)]
    public async Task Resource_rule_actions_are_normalized_and_enforced(string action, string expectedAction)
    {
        await using var db = CreateDb();
        var resource = Resource("app.example.com", "off");
        db.Resources.Add(resource);
        db.ResourceRules.Add(new ResourceRuleEntity
        {
            ResourceId = resource.Id,
            MatchType = ResourceRuleMatchTypeNames.Path,
            MatchValue = "/admin",
            Action = action,
        });
        await SeedProviderAsync(db);
        await db.SaveChangesAsync();

        var result = await CreateDecisionService(db).DecideForwardAuthAsync(Request("198.51.100.15", "app.example.com", "/admin"));

        Assert.Equal(expectedAction, result.Action);
    }

    [Fact]
    public void Ban_duration_policy_evaluator_supports_required_policy_shapes()
    {
        var evaluator = new BanDurationPolicyEvaluator();

        Assert.Equal(TimeSpan.FromSeconds(300), evaluator.Evaluate(Policy(BanDurationPolicyTypeNames.Constant, 300), 5).Duration);
        Assert.Equal(TimeSpan.FromSeconds(900), evaluator.Evaluate(Policy(BanDurationPolicyTypeNames.Linear, 300), 3).Duration);
        Assert.Equal(TimeSpan.FromSeconds(1200), evaluator.Evaluate(Policy(BanDurationPolicyTypeNames.Exponential, 300), 3).Duration);
        Assert.Equal(TimeSpan.FromSeconds(1000), evaluator.Evaluate(Policy(BanDurationPolicyTypeNames.CappedExponential, 300, max: 1000), 4).Duration);
        Assert.True(evaluator.Evaluate(Policy(BanDurationPolicyTypeNames.PermanentAfterCount, 300, permanentAfter: 3), 3).IsPermanent);
    }

    [Fact]
    public async Task Untrusted_forwarded_context_is_denied_before_policy()
    {
        await using var db = CreateDb();
        db.Resources.Add(Resource("app.example.com", "off"));
        await db.SaveChangesAsync();

        var result = await CreateDecisionService(db).DecideForwardAuthAsync(
            Request("198.51.100.16", "app.example.com") with { TrustedForwardedContext = false });

        Assert.Equal(SecurityDecisionActionNames.DenyInvalidMetadata, result.Action);
        Assert.Equal("deny", result.Decision);
    }

    private static SecurityDecisionRequest Request(
        string ip,
        string host = "app.example.com",
        string path = "/",
        string method = "GET",
        string? accept = "text/html")
        => new(
            host,
            path,
            IPAddress.Parse(ip),
            CountryCode: null,
            RegionCode: null,
            Asn: null,
            Method: method,
            AcceptHeader: accept);

    private static ManualSecurityEntryEntity ManualEntry(string ip, string entryType)
    {
        var normalized = SecuritySubjectNormalizer.Normalize(SecuritySubjectTypeNames.Ip, ip);
        return new ManualSecurityEntryEntity
        {
            SubjectType = normalized.SubjectType,
            SubjectValue = normalized.SubjectValue,
            NormalizedValue = normalized.NormalizedValue,
            EntryType = entryType,
            BypassBlocking = entryType == ManualSecurityEntryTypeNames.Allow,
            BypassAdaptiveEscalation = entryType == ManualSecurityEntryTypeNames.Allow,
            BypassRateLimit = false,
            BypassChallenge = false,
            BypassSso = false,
        };
    }

    private static ResourceEntity Resource(string domain, string forwardAuthPolicy)
        => new()
        {
            Name = domain,
            Slug = domain.Replace('.', '-'),
            Domain = domain,
            ForwardAuthPolicy = forwardAuthPolicy,
        };

    private static BanDurationPolicy Policy(
        string type,
        int seconds,
        int? max = null,
        int? permanentAfter = null)
        => new(type, seconds, 1, 2, max, permanentAfter, 86400, 604800);

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
            new ServiceCollection().AddHttpClient().BuildServiceProvider().GetRequiredService<IHttpClientFactory>(),
            new AppSettingsService(db),
            new Microsoft.Extensions.Configuration.ConfigurationBuilder().Build(),
            new EphemeralDataProtectionProvider());

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
}
