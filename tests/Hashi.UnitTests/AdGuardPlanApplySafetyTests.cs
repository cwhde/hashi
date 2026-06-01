using System.Net;
using System.Text;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Crypto;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Hashi.Infrastructure.Sync;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class AdGuardPlanApplySafetyTests
{
    [Fact]
    public async Task UpsertRewriteAsync_saves_desired_state_and_returns_plan_without_remote_write()
    {
        await using var db = CreateDb();
        var connectionId = await AddConnectionAsync(db);
        var handler = new FakeAdGuardHandler("""{"rewrites":[]}""");
        var service = CreateService(db, handler);

        var result = await service.UpsertRewriteAsync(
            connectionId,
            new UpsertAdGuardRewriteRequest("App.Example.Com.", "10.0.0.10"));

        Assert.NotNull(result.Rewrite);
        Assert.Equal("app.example.com", result.Rewrite!.Domain);
        Assert.Contains(result.Plan.Changes, x => x.Kind == "create" && x.Domain == "app.example.com");
        Assert.Equal(0, handler.AddCalls);
        Assert.Single(await db.AdGuardRewrites.ToListAsync());
    }

    [Fact]
    public async Task ApplyDeletePlanAsync_preserves_local_desired_state_when_remote_delete_fails()
    {
        await using var db = CreateDb();
        var connectionId = await AddConnectionAsync(db);
        var rewrite = new AdGuardRewriteEntity
        {
            ConnectionId = connectionId,
            Domain = "app.example.com",
            Answer = "10.0.0.10",
            ManagedByHashi = true,
        };
        db.AdGuardRewrites.Add(rewrite);
        await db.SaveChangesAsync();

        var handler = new FakeAdGuardHandler("""{"rewrites":[{"domain":"app.example.com","answer":"10.0.0.10","id":"remote-1"}]}""")
        {
            DeleteStatusCode = HttpStatusCode.InternalServerError,
        };
        var service = CreateService(db, handler);

        var plan = await service.DeleteRewriteAsync(connectionId, rewrite.Id);
        Assert.NotNull(plan);

        var result = await service.ApplyPlanAsync(
            connectionId,
            new AdGuardRewriteApplyRequest(plan!.PlanId, ConfirmDestructive: true),
            rewrite.Id);

        Assert.False(result.Succeeded);
        Assert.Equal(SyncRunStatusNames.Failed, result.Status);
        Assert.True(await db.AdGuardRewrites.AnyAsync(x => x.Id == rewrite.Id));
        Assert.Contains(await db.AuditEvents.ToListAsync(), x => x.Category == "adguard" && x.Action == "apply_failed");
    }

    [Fact]
    public async Task Topology_sync_preserves_manual_hashi_rewrites_outside_resource_desired_set()
    {
        await using var db = CreateDb();
        db.AppSettings.Add(new AppSettingsEntity { RootDomain = "example.com" });
        var connectionId = await AddConnectionAsync(db);
        var firewallHostId = Guid.NewGuid();
        db.FirewallHosts.Add(new FirewallHostEntity
        {
            Id = firewallHostId,
            ConnectionId = Guid.NewGuid(),
            Name = "edge",
            Domain = "edge.example.com",
            LinkedTraefikHost = "edge.example.com",
            InternalTraefikIp = "10.0.0.2",
        });
        db.Resources.Add(new ResourceEntity
        {
            Name = "App",
            Slug = "app",
            Domain = "app.example.com",
            FirewallHostId = firewallHostId,
            Enabled = true,
        });
        db.AdGuardRewrites.Add(new AdGuardRewriteEntity
        {
            ConnectionId = connectionId,
            Domain = "manual.example.com",
            Answer = "10.0.0.50",
            ManagedByHashi = true,
            Source = AdGuardRewriteSourceNames.Manual,
        });
        await db.SaveChangesAsync();

        var handler = new FakeAdGuardHandler("""{"rewrites":[]}""");
        var service = CreateService(db, handler);

        var plan = await service.PlanSyncAsync(connectionId, updateTopologyDesiredState: true);

        Assert.DoesNotContain(plan.Changes, x => x.Kind == "delete" && x.Domain == "manual.example.com");
        Assert.True(await db.AdGuardRewrites.AnyAsync(x =>
            x.ConnectionId == connectionId &&
            x.Domain == "manual.example.com" &&
            x.Source == AdGuardRewriteSourceNames.Manual));
        Assert.True(await db.AdGuardRewrites.AnyAsync(x =>
            x.ConnectionId == connectionId &&
            x.Domain == "app.example.com" &&
            x.Source == AdGuardRewriteSourceNames.Topology));
    }

    [Fact]
    public async Task Topology_stale_rewrites_plan_as_confirmed_deletes_before_remote_cleanup()
    {
        await using var db = CreateDb();
        db.AppSettings.Add(new AppSettingsEntity { RootDomain = "example.com" });
        var connectionId = await AddConnectionAsync(db);
        db.AdGuardRewrites.Add(new AdGuardRewriteEntity
        {
            ConnectionId = connectionId,
            Domain = "stale.example.com",
            Answer = "10.0.0.99",
            ManagedByHashi = true,
            Source = AdGuardRewriteSourceNames.Topology,
        });
        await db.SaveChangesAsync();

        var handler = new FakeAdGuardHandler("""{"rewrites":[{"domain":"stale.example.com","answer":"10.0.0.99","id":"remote-1"}]}""");
        var service = CreateService(db, handler);

        var plan = await service.PlanSyncAsync(connectionId, updateTopologyDesiredState: true);

        Assert.True(plan.RequiresConfirmation);
        Assert.Contains(plan.Changes, x =>
            x.Kind == "delete" &&
            x.Domain == "stale.example.com" &&
            x.Summary.Contains("topology", StringComparison.OrdinalIgnoreCase));

        var unconfirmed = await service.ApplyPlanAsync(
            connectionId,
            new AdGuardRewriteApplyRequest(plan.PlanId, ConfirmDestructive: false),
            updateTopologyDesiredState: true);

        Assert.False(unconfirmed.Succeeded);
        Assert.Equal(SyncRunStatusNames.AwaitingConfirmation, unconfirmed.Status);
        Assert.Equal(0, handler.DeleteCalls);
        Assert.True(await db.AdGuardRewrites.AnyAsync(x => x.Domain == "stale.example.com"));

        var confirmedPlan = await service.PlanSyncAsync(connectionId, updateTopologyDesiredState: true);
        var confirmed = await service.ApplyPlanAsync(
            connectionId,
            new AdGuardRewriteApplyRequest(confirmedPlan.PlanId, ConfirmDestructive: true),
            updateTopologyDesiredState: true);

        Assert.True(confirmed.Succeeded);
        Assert.Equal(1, handler.DeleteCalls);
        Assert.False(await db.AdGuardRewrites.AnyAsync(x => x.Domain == "stale.example.com"));
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private static async Task<Guid> AddConnectionAsync(HashiDbContext db)
    {
        var secretId = Guid.NewGuid();
        var rootKey = new byte[32];
        var dek = new byte[32];
        db.SecretRecords.Add(new SecretRecordEntity
        {
            Id = secretId,
            Purpose = SecretPurposeMapping.ToName(SecretPurpose.AdGuardCredential),
            Label = "AdGuard",
            AdminWrappedDekBlob = AesGcmCipher.Encrypt(dek, rootKey).ToBlob(),
            CiphertextBlob = AesGcmCipher.Encrypt(Encoding.UTF8.GetBytes("""{"password":"test"}"""), dek).ToBlob(),
        });

        var connectionId = Guid.NewGuid();
        db.AdGuardConnections.Add(new AdGuardConnectionEntity
        {
            Id = connectionId,
            Name = "home",
            BaseUrl = "http://adguard.test",
            PasswordSecretId = secretId,
            Enabled = true,
        });
        await db.SaveChangesAsync();
        return connectionId;
    }

    private static AdGuardSyncService CreateService(HashiDbContext db, FakeAdGuardHandler handler)
    {
        var vault = new VaultSessionState();
        vault.Unlock(new byte[32]);
        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var syncRuns = new SyncRunService(db);
        return new AdGuardSyncService(
            db,
            new FakeHttpClientFactory(handler),
            secrets,
            new AuditService(db),
            syncRuns);
    }

    private sealed class FakeHttpClientFactory(HttpMessageHandler handler) : IHttpClientFactory
    {
        public HttpClient CreateClient(string name) => new(handler, disposeHandler: false);
    }

    private sealed class FakeAdGuardHandler(string rewriteListJson) : HttpMessageHandler
    {
        public int AddCalls { get; private set; }

        public int DeleteCalls { get; private set; }

        public HttpStatusCode DeleteStatusCode { get; init; } = HttpStatusCode.OK;

        protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
        {
            if (request.RequestUri?.AbsolutePath.EndsWith("/control/rewrite/list", StringComparison.Ordinal) == true)
            {
                return Task.FromResult(JsonResponse(rewriteListJson));
            }

            if (request.RequestUri?.AbsolutePath.EndsWith("/control/rewrite/add", StringComparison.Ordinal) == true)
            {
                AddCalls++;
                return Task.FromResult(JsonResponse("""{"id":"created-1"}"""));
            }

            if (request.RequestUri?.AbsolutePath.EndsWith("/control/rewrite/delete", StringComparison.Ordinal) == true)
            {
                DeleteCalls++;
                return Task.FromResult(new HttpResponseMessage(DeleteStatusCode)
                {
                    Content = new StringContent("{}", Encoding.UTF8, "application/json"),
                });
            }

            return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent("{}", Encoding.UTF8, "application/json"),
            });
        }

        private static HttpResponseMessage JsonResponse(string json) => new(HttpStatusCode.OK)
        {
            Content = new StringContent(json, Encoding.UTF8, "application/json"),
        };
    }
}
