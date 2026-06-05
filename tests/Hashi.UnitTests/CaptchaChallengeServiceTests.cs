using System.Net;
using System.Text;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Hashi.UnitTests.Fakes;
using Microsoft.AspNetCore.DataProtection;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Xunit;

namespace Hashi.UnitTests;

public sealed class CaptchaChallengeServiceTests
{
    [Fact]
    public async Task Verify_success_clears_only_active_challenge_state_and_decays_triggering_buckets()
    {
        await using var db = CreateDb();
        var service = CreateService(db, new FakeCapClient(CapVerifyResult.Verified()));
        await SeedEnabledSettingsAsync(db, service);
        var subject = await SeedChallengedSubjectAsync(db, "198.51.100.20");
        db.SecurityRequestBuckets.AddRange(
            new SecurityRequestBucketEntity
            {
                BucketStartUtc = DateTimeOffset.UtcNow,
                ClientIp = "198.51.100.20",
                NormalizedSubjectValue = "198.51.100.20",
                Resource = "app.example.com",
                TraefikInstance = "forward-auth",
                Method = "GET",
                PathPrefix = "/",
                StatusClass = 4,
                ChallengedCount = 10,
                ChallengeIgnoredCount = 4,
                FailedChallengeCount = 2,
            },
            new SecurityRequestBucketEntity
            {
                BucketStartUtc = DateTimeOffset.UtcNow,
                ClientIp = "198.51.100.20",
                NormalizedSubjectValue = "198.51.100.20",
                Resource = "app.example.com",
                TraefikInstance = "forward-auth",
                Method = "GET",
                PathPrefix = "/ok",
                StatusClass = 2,
                AllowedCount = 9,
            });
        await db.SaveChangesAsync();

        var result = await service.VerifyChallengeAsync(
            IPAddress.Parse("198.51.100.20"),
            new CaptchaChallengeVerifyRequest("ok-token", "https://app.example.com/private"));

        Assert.True(result.Verified);
        Assert.True(result.ChallengeCleared);
        Assert.Equal("https://app.example.com/private", result.RedirectUrl);
        var state = await db.SecuritySubjectStates.SingleAsync(x => x.SecuritySubjectId == subject.Id);
        Assert.False(state.ChallengeRequired);
        Assert.Null(state.ChallengeReason);
        Assert.Equal(0, state.RequestsWhileChallenged);
        Assert.Equal(0, state.FailedChallengeCount);
        Assert.Equal(2, state.SuccessfulChallengeCount);
        Assert.NotNull(state.LastChallengeSolvedAtUtc);
        var challengedBucket = await db.SecurityRequestBuckets.SingleAsync(x => x.PathPrefix == "/");
        Assert.Equal(5, challengedBucket.ChallengedCount);
        Assert.Equal(2, challengedBucket.ChallengeIgnoredCount);
        Assert.Equal(1, challengedBucket.FailedChallengeCount);
        var allowedBucket = await db.SecurityRequestBuckets.SingleAsync(x => x.PathPrefix == "/ok");
        Assert.Equal(9, allowedBucket.AllowedCount);
        Assert.Empty(await db.EdgeSessions.ToListAsync());
    }

    [Fact]
    public async Task Verify_failure_keeps_challenge_and_counts_failure()
    {
        await using var db = CreateDb();
        var service = CreateService(db, new FakeCapClient(CapVerifyResult.Failed("bad token")));
        await SeedEnabledSettingsAsync(db, service);
        await SeedChallengedSubjectAsync(db, "198.51.100.21");

        var result = await service.VerifyChallengeAsync(
            IPAddress.Parse("198.51.100.21"),
            new CaptchaChallengeVerifyRequest("bad-token", "https://app.example.com/private"));

        Assert.False(result.Verified);
        Assert.Equal("failed", result.Status);
        var state = await db.SecuritySubjectStates.SingleAsync();
        Assert.True(state.ChallengeRequired);
        Assert.Equal(1, state.ChallengeAttempts);
        Assert.Equal(3, state.FailedChallengeCount);
    }

    [Fact]
    public async Task Verify_unavailable_keeps_challenge_and_reports_unavailable_without_secret_detail()
    {
        await using var db = CreateDb();
        var service = CreateService(db, new FakeCapClient(CapVerifyResult.Unavailable("connection refused with secret abc")));
        await SeedEnabledSettingsAsync(db, service);
        await SeedChallengedSubjectAsync(db, "198.51.100.22");

        var result = await service.VerifyChallengeAsync(
            IPAddress.Parse("198.51.100.22"),
            new CaptchaChallengeVerifyRequest("token", "https://app.example.com/private"));

        Assert.False(result.Verified);
        Assert.Equal("unavailable", result.Status);
        Assert.Equal("CAPTCHA verification is temporarily unavailable.", result.Error);
        var state = await db.SecuritySubjectStates.SingleAsync();
        Assert.True(state.ChallengeRequired);
    }

    [Fact]
    public async Task Protected_hits_while_challenged_escalate_to_soft_and_firewall_blocks()
    {
        await using var db = CreateDb();
        db.SecurityPolicySettings.Add(new SecurityPolicySettingsEntity
        {
            ChallengeIgnoredThreshold = 2,
            FirewallBlockThresholdWhileChallenged = 3,
        });
        var service = CreateService(db, new FakeCapClient(CapVerifyResult.Verified()));
        await SeedEnabledSettingsAsync(db, service);
        await SeedChallengedSubjectAsync(db, "198.51.100.23");
        var firstState = await db.SecuritySubjectStates.SingleAsync();
        firstState.RequestsWhileChallenged = 1;
        db.Resources.Add(new ResourceEntity { Name = "App", Slug = "app", Domain = "app.example.com", ForwardAuthPolicy = "adaptive" });
        await db.SaveChangesAsync();
        var decisions = CreateDecisionService(db, service);

        _ = await decisions.DecideForwardAuthAsync(Request("198.51.100.23", "app.example.com"));
        db.ChangeTracker.Clear();
        var softState = await db.SecuritySubjectStates.AsNoTracking().SingleAsync();
        Assert.NotNull(softState.SoftBlockedUntilUtc);

        await SeedChallengedSubjectAsync(db, "198.51.100.25");
        var firewallSeed = await db.SecuritySubjectStates.SingleAsync(x => x.SecuritySubject.NormalizedValue == "198.51.100.25");
        firewallSeed.RequestsWhileChallenged = 2;
        await db.SaveChangesAsync();
        _ = await decisions.DecideForwardAuthAsync(Request("198.51.100.25", "app.example.com"));
        db.ChangeTracker.Clear();
        var firewallState = await db.SecuritySubjectStates.AsNoTracking().SingleAsync(x => x.SecuritySubject.NormalizedValue == "198.51.100.25");
        Assert.NotNull(firewallState.FirewallBlockedUntilUtc);
    }

    [Fact]
    public async Task Required_public_challenge_resource_is_not_deletable_and_bypasses_challenge_only()
    {
        await using var db = CreateDb();
        var service = CreateService(db, new FakeCapClient(CapVerifyResult.Verified()));
        var settings = await service.UpdateSettingsAsync(new CaptchaSettingsRequest(
            true,
            "https://cap.example.com",
            "site-key",
            "secret-key",
            null,
            5,
            true,
            false,
            null,
            null,
            null,
            "challenge.example.com",
            "decay",
            50,
            300,
            5,
            30));
        Assert.NotNull(settings.PublicChallengeResourceId);

        var resourceService = TestPlatformHelpers.CreateResourceService(db);
        await Assert.ThrowsAsync<InvalidOperationException>(() => resourceService.DeleteAsync(settings.PublicChallengeResourceId!.Value));

        await SeedChallengedSubjectAsync(db, "198.51.100.24");
        var decisions = CreateDecisionService(db, service);
        var allow = await decisions.DecideForwardAuthAsync(Request("198.51.100.24", "challenge.example.com"));
        Assert.Equal(SecurityDecisionActionNames.AllowUpstream, allow.Action);

        var subject = await db.SecuritySubjects.SingleAsync(x => x.NormalizedValue == "198.51.100.24");
        subject.CurrentState = SecuritySubjectStateNames.FirewallBlocked;
        var deny = await decisions.DecideForwardAuthAsync(Request("198.51.100.24", "challenge.example.com"));
        Assert.Equal(SecurityDecisionActionNames.DenyFirewallBlocked, deny.Action);
    }

    private static async Task<SecuritySubjectEntity> SeedChallengedSubjectAsync(HashiDbContext db, string ip)
    {
        var normalized = SecuritySubjectNormalizer.NormalizeIp(IPAddress.Parse(ip));
        var subject = new SecuritySubjectEntity
        {
            SubjectType = normalized.SubjectType,
            SubjectValue = normalized.SubjectValue,
            NormalizedValue = normalized.NormalizedValue,
            CurrentState = SecuritySubjectStateNames.Challenged,
        };
        db.SecuritySubjects.Add(subject);
        db.SecuritySubjectStates.Add(new SecuritySubjectStateEntity
        {
            SecuritySubjectId = subject.Id,
            ChallengeRequired = true,
            ChallengeRequiredSinceUtc = DateTimeOffset.UtcNow.AddMinutes(-1),
            ChallengeReason = "rate_limit",
            RequestsWhileChallenged = 3,
            FailedChallengeCount = 2,
            SuccessfulChallengeCount = 1,
        });
        await db.SaveChangesAsync();
        return subject;
    }

    private static async Task SeedEnabledSettingsAsync(HashiDbContext db, CaptchaChallengeService service)
    {
        db.Resources.Add(new ResourceEntity { Name = "App", Slug = "app", Domain = "app.example.com", ForwardAuthPolicy = "adaptive" });
        await db.SaveChangesAsync();
        await service.UpdateSettingsAsync(new CaptchaSettingsRequest(
            true,
            "https://cap.example.com",
            "site-key",
            "secret-key",
            null,
            5,
            true,
            false,
            null,
            null,
            null,
            "challenge.example.com",
            "decay",
            50,
            300,
            5,
            30));
    }

    private static SecurityDecisionRequest Request(string ip, string host)
        => new(host, "/", IPAddress.Parse(ip), null, null, null, Method: "GET", AcceptHeader: "text/html");

    private static CaptchaChallengeService CreateService(HashiDbContext db, ICapClient capClient)
    {
        var vault = new VaultSessionState();
        var serviceSync = new ServiceSyncVaultState();
        vault.UnlockForSession("local-test-session", Encoding.UTF8.GetBytes("01234567890123456789012345678901"));
        serviceSync.Initialize(Encoding.UTF8.GetBytes("12345678901234567890123456789012"));
        return new CaptchaChallengeService(
            db,
            new SecretRecordService(db, vault, serviceSync),
            capClient,
            new AuditService(db),
            new BanDurationPolicyEvaluator());
    }

    private static SecurityDecisionService CreateDecisionService(HashiDbContext db, CaptchaChallengeService captcha)
        => new(db, CreateOidcService(db), captcha);

    private static OidcEdgeAuthService CreateOidcService(HashiDbContext db)
        => new(
            db,
            new SecretRecordService(db, new VaultSessionState(), new ServiceSyncVaultState()),
            new ServiceCollection().AddHttpClient().BuildServiceProvider().GetRequiredService<IHttpClientFactory>(),
            new AppSettingsService(db),
            new ConfigurationBuilder().Build(),
            new EphemeralDataProtectionProvider());

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private sealed class FakeCapClient(CapVerifyResult result) : ICapClient
    {
        public Task<CapVerifyResult> VerifyAsync(
            Uri capInstanceBaseUrl,
            string siteKey,
            string secretKey,
            string token,
            TimeSpan timeout,
            CancellationToken cancellationToken = default)
            => Task.FromResult(result);
    }
}
