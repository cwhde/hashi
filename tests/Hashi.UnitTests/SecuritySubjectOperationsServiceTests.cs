using Hashi.Contracts.Api;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Hashi.UnitTests.Fakes;
using Microsoft.AspNetCore.DataProtection;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Xunit;

namespace Hashi.UnitTests;

public sealed class SecuritySubjectOperationsServiceTests
{
    [Fact]
    public async Task Search_detail_and_timeline_project_subject_context()
    {
        await using var db = CreateDb();
        var subject = new SecuritySubjectEntity
        {
            SubjectType = SecuritySubjectTypeNames.Ip,
            SubjectValue = "203.0.113.10",
            NormalizedValue = "203.0.113.10",
            LastCountry = "US",
            LastAsn = "AS64500",
        };
        db.SecuritySubjects.Add(subject);
        db.SecuritySubjectStates.Add(new SecuritySubjectStateEntity
        {
            SecuritySubjectId = subject.Id,
            ChallengeRequired = true,
            ChallengeReason = "rate_limit",
        });
        db.ManualSecurityEntries.Add(new ManualSecurityEntryEntity
        {
            SubjectType = SecuritySubjectTypeNames.Ip,
            SubjectValue = "203.0.113.10",
            NormalizedValue = "203.0.113.10",
            EntryType = ManualSecurityEntryTypeNames.Allow,
            Reason = "maintenance probe",
        });
        db.BlocklistEntries.Add(new BlocklistEntryEntity
        {
            SubjectType = SecuritySubjectTypeNames.Cidr,
            Type = BlocklistTypeNames.Cidr,
            Value = "203.0.113.0/24",
            NormalizedValue = "203.0.113.0/24",
            Reason = "feed hit",
            Source = "test",
        });
        db.SecurityEvents.Add(new SecurityEventEntity
        {
            SubjectType = SecuritySubjectTypeNames.Ip,
            SubjectValue = "203.0.113.10",
            NormalizedSubjectValue = "203.0.113.10",
            EventType = "forward_auth",
            Decision = "challenge",
            OccurredAtUtc = DateTimeOffset.UtcNow,
        });
        db.SecurityRequestBuckets.Add(new SecurityRequestBucketEntity
        {
            BucketStartUtc = DateTimeOffset.UtcNow,
            ClientIp = "203.0.113.10",
            NormalizedSubjectValue = "203.0.113.10",
            RequestCount = 12,
            ChallengedCount = 3,
            Resource = "app.example.test",
            TraefikInstance = "edge",
        });
        await db.SaveChangesAsync();

        var service = CreateService(db);

        var search = await service.SearchAsync("203.0.113.10");
        var result = Assert.Single(search.Results);
        Assert.Equal(subject.Id, result.Id);

        var detail = await service.GetDetailAsync(subject.Id);
        Assert.NotNull(detail);
        Assert.True(detail.State!.ChallengeRequired);
        Assert.Single(detail.ManualEntries);
        Assert.Single(detail.BlocklistEntries);

        var events = await service.ListEventsAsync(subject.Id, "forward_auth", null);
        Assert.Single(events);

        var buckets = await service.ListBucketsAsync(subject.Id);
        Assert.Equal(12, Assert.Single(buckets).RequestCount);
    }

    [Fact]
    public async Task Manual_allow_defaults_do_not_bypass_sso_or_challenge_and_audit()
    {
        await using var db = CreateDb();
        var service = CreateService(db);

        var entry = await service.CreateManualEntryAsync(new UpsertManualSecurityEntryRequest(
            SecuritySubjectTypeNames.Ip,
            "198.51.100.20",
            ManualSecurityEntryTypeNames.Allow,
            ManualSecurityScopeTypeNames.Global,
            null,
            "trusted scanner",
            null,
            true,
            null,
            null,
            null,
            null,
            null,
            true));

        Assert.True(entry.BypassBlocking);
        Assert.True(entry.BypassAdaptiveEscalation);
        Assert.False(entry.BypassChallenge);
        Assert.False(entry.BypassSso);
        Assert.Contains(await db.AuditEvents.ToListAsync(), x => x.Action == "manual_allow_created");
        Assert.Contains(await db.SecurityEvents.ToListAsync(), x => x.EventType == "manual_allow_created");
    }

    [Fact]
    public async Task Block_actions_update_state_and_effective_decision()
    {
        await using var db = CreateDb();
        var service = CreateService(db);

        var created = await service.CreateBlockAsync(new CreateSecurityBlockRequest(
            SecuritySubjectTypeNames.Ip,
            "198.51.100.44",
            "soft",
            "incident",
            DateTimeOffset.UtcNow.AddHours(1),
            false));

        Assert.Equal(ManualSecurityEntryTypeNames.Block, created.ManualEntry.EntryType);
        Assert.True(created.State!.ManualBlockActive);
        Assert.False(created.ManualEntry.BypassBlocking);

        var decision = await service.GetEffectiveDecisionAsync(created.State.SecuritySubjectId);
        Assert.NotNull(decision);
        Assert.Equal(SecurityDecisionActionNames.DenyManualBlock, decision.Action);

        var extended = await service.ExtendBlockAsync(created.ManualEntry.Id, 3600);
        Assert.NotNull(extended);
        Assert.False(extended.ManualEntry.IsPermanent);

        var expired = await service.ExpireBlockAsync(created.ManualEntry.Id);
        Assert.NotNull(expired);
        Assert.False(expired.ManualEntry.Enabled);
        Assert.False(expired.State!.ManualBlockActive);
    }

    private static SecuritySubjectOperationsService CreateService(HashiDbContext db)
        => new(
            db,
            new SecurityDecisionService(db, CreateOidcService(db)),
            TestPlatformHelpers.CreateFirewallApply(db),
            new AuditService(db));

    private static OidcEdgeAuthService CreateOidcService(HashiDbContext db)
        => new(
            db,
            new SecretRecordService(db, new VaultSessionState(), new ServiceSyncVaultState()),
            new ServiceCollection().AddHttpClient().BuildServiceProvider().GetRequiredService<IHttpClientFactory>(),
            new AppSettingsService(db),
            new Microsoft.Extensions.Configuration.ConfigurationBuilder().Build(),
            new EphemeralDataProtectionProvider());

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
