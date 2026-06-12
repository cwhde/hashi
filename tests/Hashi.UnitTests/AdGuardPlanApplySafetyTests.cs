using System.Net;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json;
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
    public async Task ApplyPlanAsync_fails_when_remote_readback_does_not_contain_created_rewrite()
    {
        await using var db = CreateDb();
        var connectionId = await AddConnectionAsync(db);
        var handler = new FakeAdGuardHandler("""{"rewrites":[]}""") { PersistMutations = false };
        var service = CreateService(db, handler);
        var mutation = await service.UpsertRewriteAsync(
            connectionId,
            new UpsertAdGuardRewriteRequest("app.example.com", "10.0.0.10"));

        var result = await service.ApplyPlanAsync(
            connectionId,
            new AdGuardRewriteApplyRequest(mutation.Plan.PlanId, ConfirmDestructive: false));

        Assert.False(result.Succeeded);
        Assert.Contains("remote verification failed", result.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public async Task ApplyPlanAsync_cleans_duplicate_remote_rows_for_managed_domains()
    {
        await using var db = CreateDb();
        var connectionId = await AddConnectionAsync(db);
        db.AdGuardRewrites.Add(new AdGuardRewriteEntity
        {
            ConnectionId = connectionId,
            Domain = "app.example.com",
            Answer = "10.0.0.10",
            ManagedByHashi = true,
        });
        await db.SaveChangesAsync();
        var handler = new FakeAdGuardHandler(
            """{"rewrites":[{"domain":"app.example.com","answer":"10.0.0.10","id":"remote-1"},{"domain":"app.example.com","answer":"10.0.0.11","id":"remote-2"}]}""");
        var service = CreateService(db, handler);
        var plan = await service.PlanSyncAsync(connectionId);

        var result = await service.ApplyPlanAsync(
            connectionId,
            new AdGuardRewriteApplyRequest(plan.PlanId, ConfirmDestructive: false));

        Assert.True(result.Succeeded);
        Assert.Equal(1, handler.DeleteCalls);
        Assert.Equal(1, handler.CountForDomain("app.example.com"));
        Assert.True(await db.AuditEvents.AnyAsync(x => x.Action == "duplicate_rewrites_cleaned"));
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

    [Fact]
    public async Task Safe_apply_creates_rewrites_without_applying_pending_deletes()
    {
        await using var db = CreateDb();
        db.AppSettings.Add(new AppSettingsEntity { RootDomain = "example.com" });
        var connectionId = await AddConnectionAsync(db);
        db.AdGuardRewrites.AddRange(
            new AdGuardRewriteEntity
            {
                ConnectionId = connectionId,
                Domain = "stale.example.com",
                Answer = "10.0.0.99",
                ManagedByHashi = true,
                Source = AdGuardRewriteSourceNames.Topology,
            },
            new AdGuardRewriteEntity
            {
                ConnectionId = connectionId,
                Domain = "new.example.com",
                Answer = "10.0.0.50",
                ManagedByHashi = true,
                Source = AdGuardRewriteSourceNames.Manual,
            });
        await db.SaveChangesAsync();

        var handler = new FakeAdGuardHandler("""{"rewrites":[{"domain":"stale.example.com","answer":"10.0.0.99","id":"remote-1"}]}""");
        var service = CreateService(db, handler);
        var plan = await service.PlanSyncAsync(connectionId, updateTopologyDesiredState: true);

        var result = await service.ApplySafePlanAsync(
            connectionId,
            plan.PlanId,
            updateTopologyDesiredState: true);

        Assert.True(result.Succeeded);
        Assert.Equal(SyncRunStatusNames.AwaitingConfirmation, result.Status);
        Assert.Equal(1, handler.AddCalls);
        Assert.Equal(0, handler.DeleteCalls);
        Assert.True(await db.AdGuardRewrites.AnyAsync(x => x.Domain == "stale.example.com"));
    }

    [Fact]
    public async Task TestConnectionAsync_uses_pulse_agent_target_base_uri()
    {
        await using var db = CreateDb();
        var connectionId = await AddConnectionAsync(db);
        var agentId = Guid.NewGuid();
        db.PulseAgents.Add(new PulseAgentEntity
        {
            Id = agentId,
            Name = "edge",
            TokenHash = "hash",
            Status = "online",
            LastSeenAtUtc = DateTimeOffset.UtcNow,
            LastSelectedIp = "10.0.0.53",
            LastPrivateIp = "10.0.0.53",
            LastPrivateIpv4CandidatesJson = """["10.0.0.53"]""",
        });
        db.ConnectionTargets.Add(new ConnectionTargetEntity
        {
            OwnerType = ConnectionTargetOwnerTypeNames.AdGuardConnection,
            OwnerId = connectionId,
            TargetMode = ConnectionTargetModeNames.PulseAgent,
            PulseAgentId = agentId,
            PulseIpMode = PulseTargetIpModeNames.Selected,
            Scheme = "http",
            Port = 3000,
        });
        await db.SaveChangesAsync();
        var handler = new FakeAdGuardHandler("""{"rewrites":[]}""");
        var service = CreateService(db, handler);

        var result = await service.TestConnectionAsync(connectionId);

        Assert.True(result.Connected);
        Assert.Equal("http://10.0.0.53:3000", result.ResolvedBaseUrl);
        Assert.Equal(ConnectionTargetStatusNames.Resolved, result.Target?.Status);
        Assert.NotNull(handler.LastRequestUri);
        Assert.Equal("10.0.0.53", handler.LastRequestUri!.Host);
    }

    [Fact]
    public async Task Internal_agent_dns_plan_generates_desired_rewrite_and_preserves_manual_rewrites()
    {
        await using var db = CreateDb();
        var connectionId = await AddConnectionAsync(db);
        var agentId = Guid.NewGuid();
        db.InternalAgentDnsSettings.Add(new InternalAgentDnsSettingsEntity
        {
            Enabled = true,
            Domain = "hashi.home.arpa",
            AdGuardConnectionId = connectionId,
        });
        db.PulseAgents.Add(new PulseAgentEntity
        {
            Id = agentId,
            Name = "Kanae Node",
            TokenHash = "hash",
            Status = "online",
            LastSeenAtUtc = DateTimeOffset.UtcNow,
            LastSelectedIp = "10.0.0.42",
        });
        db.AdGuardRewrites.Add(new AdGuardRewriteEntity
        {
            ConnectionId = connectionId,
            Domain = "manual.example.com",
            Answer = "10.0.0.10",
            ManagedByHashi = false,
            Source = AdGuardRewriteSourceNames.Manual,
        });
        await db.SaveChangesAsync();

        var handler = new FakeAdGuardHandler("""{"rewrites":[]}""");
        var service = CreateService(db, handler);

        var plan = await service.PlanSyncAsync(connectionId, updateInternalAgentDnsDesiredState: true);

        Assert.Contains(plan.Changes, x =>
            x.Kind == "create" &&
            x.Domain == "kanae-node.hashi.home.arpa" &&
            x.DesiredAnswer == "10.0.0.42");
        Assert.DoesNotContain(plan.Changes, x => x.Domain == "manual.example.com");
        Assert.True(await db.AdGuardRewrites.AnyAsync(x =>
            x.Domain == "kanae-node.hashi.home.arpa" &&
            x.Source == AdGuardRewriteSourceNames.InternalAgentDns));
        Assert.Equal(0, handler.AddCalls);
    }

    [Fact]
    public async Task Internal_agent_dns_stale_agent_keeps_last_rewrite_by_default()
    {
        await using var db = CreateDb();
        var connectionId = await AddConnectionAsync(db);
        var agentId = Guid.NewGuid();
        db.InternalAgentDnsSettings.Add(new InternalAgentDnsSettingsEntity
        {
            Enabled = true,
            Domain = "hashi.home.arpa",
            AdGuardConnectionId = connectionId,
            KeepLastRewriteWhenAgentStale = true,
        });
        db.PulseAgents.Add(new PulseAgentEntity
        {
            Id = agentId,
            Name = "stale-edge",
            TokenHash = "hash",
            Status = "online",
            LastSeenAtUtc = DateTimeOffset.UtcNow.AddHours(-2),
        });
        db.InternalAgentDnsAgentSettings.Add(new InternalAgentDnsAgentSettingsEntity
        {
            PulseAgentId = agentId,
            Enabled = true,
            KeepLastRewriteWhenStale = true,
        });
        db.AdGuardRewrites.Add(new AdGuardRewriteEntity
        {
            ConnectionId = connectionId,
            Domain = "stale-edge.hashi.home.arpa",
            Answer = "10.0.0.99",
            ManagedByHashi = true,
            Source = AdGuardRewriteSourceNames.InternalAgentDns,
        });
        await db.SaveChangesAsync();

        var handler = new FakeAdGuardHandler("""{"rewrites":[{"domain":"stale-edge.hashi.home.arpa","answer":"10.0.0.99","id":"remote-1"}]}""");
        var service = CreateService(db, handler);

        var plan = await service.PlanSyncAsync(connectionId, updateInternalAgentDnsDesiredState: true);

        Assert.DoesNotContain(plan.Changes, x =>
            x.Kind == "delete" &&
            x.Domain == "stale-edge.hashi.home.arpa");
        Assert.True(await db.AdGuardRewrites.AnyAsync(x => x.Domain == "stale-edge.hashi.home.arpa"));
    }

    [Fact]
    public async Task Internal_agent_dns_apply_records_result_hash_and_audit()
    {
        await using var db = CreateDb();
        var connectionId = await AddConnectionAsync(db);
        db.InternalAgentDnsSettings.Add(new InternalAgentDnsSettingsEntity
        {
            Enabled = true,
            Domain = "hashi.home.arpa",
            AdGuardConnectionId = connectionId,
        });
        db.PulseAgents.Add(new PulseAgentEntity
        {
            Id = Guid.NewGuid(),
            Name = "edge",
            TokenHash = "hash",
            Status = "online",
            LastSeenAtUtc = DateTimeOffset.UtcNow,
            LastSelectedIp = "10.0.0.42",
        });
        await db.SaveChangesAsync();
        var handler = new FakeAdGuardHandler("""{"rewrites":[]}""");
        var service = CreateService(db, handler);

        var plan = await service.PlanSyncAsync(connectionId, updateInternalAgentDnsDesiredState: true);
        var result = await service.ApplyPlanAsync(
            connectionId,
            new AdGuardRewriteApplyRequest(plan.PlanId),
            updateInternalAgentDnsDesiredState: true);

        Assert.True(result.Succeeded);
        Assert.Equal(1, handler.AddCalls);
        var settings = await db.InternalAgentDnsSettings.SingleAsync();
        Assert.Equal(SyncRunStatusNames.Succeeded, settings.LastSyncStatus);
        Assert.False(string.IsNullOrWhiteSpace(settings.LastAppliedHash));
        Assert.Contains(await db.AuditEvents.ToListAsync(), x => x.Category == "adguard" && x.Action == "apply_succeeded");
    }

    [Fact]
    public async Task Internal_agent_dns_settings_detect_name_collisions_before_save()
    {
        await using var db = CreateDb();
        var connectionId = await AddConnectionAsync(db);
        db.PulseAgents.AddRange(
            new PulseAgentEntity { Id = Guid.NewGuid(), Name = "Edge!", TokenHash = "hash", Status = "online" },
            new PulseAgentEntity { Id = Guid.NewGuid(), Name = "edge", TokenHash = "hash", Status = "online" });
        await db.SaveChangesAsync();
        var handler = new FakeAdGuardHandler("""{"rewrites":[]}""");
        var adguard = CreateService(db, handler);
        var settings = new InternalAgentDnsSettingsService(db, new AuditService(db), adguard);

        var ex = await Assert.ThrowsAsync<InvalidOperationException>(() =>
            settings.UpdateAsync(new InternalAgentDnsSettingsRequest(
                true,
                "hashi.home.arpa",
                true,
                connectionId,
                null)));

        Assert.Contains("collision", ex.Message, StringComparison.OrdinalIgnoreCase);
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
            syncRuns,
            new ConnectionTargetResolver(db, new AuditService(db)));
    }

    private sealed class FakeHttpClientFactory(HttpMessageHandler handler) : IHttpClientFactory
    {
        public HttpClient CreateClient(string name) => new(handler, disposeHandler: false);
    }

    private sealed class FakeAdGuardHandler(string rewriteListJson) : HttpMessageHandler
    {
        private readonly List<RemoteRewrite> _rewrites = ParseRewrites(rewriteListJson);

        public int AddCalls { get; private set; }

        public int DeleteCalls { get; private set; }

        public HttpStatusCode DeleteStatusCode { get; init; } = HttpStatusCode.OK;

        public bool PersistMutations { get; init; } = true;

        public Uri? LastRequestUri { get; private set; }

        public int CountForDomain(string domain)
            => _rewrites.Count(x => string.Equals(x.Domain, domain, StringComparison.OrdinalIgnoreCase));

        protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
        {
            LastRequestUri = request.RequestUri;
            if (request.RequestUri?.AbsolutePath.EndsWith("/control/rewrite/list", StringComparison.Ordinal) == true)
            {
                return JsonResponse(JsonSerializer.Serialize(new
                {
                    rewrites = _rewrites.Select(x => new { domain = x.Domain, answer = x.Answer, id = x.Id }),
                }));
            }

            if (request.RequestUri?.AbsolutePath.EndsWith("/control/rewrite/add", StringComparison.Ordinal) == true)
            {
                AddCalls++;
                var payload = await request.Content!.ReadFromJsonAsync<RewritePayload>(cancellationToken: cancellationToken)
                    ?? throw new InvalidOperationException("Missing rewrite payload.");
                if (PersistMutations)
                {
                    _rewrites.RemoveAll(x => string.Equals(x.Domain, payload.Domain, StringComparison.OrdinalIgnoreCase));
                    _rewrites.Add(new RemoteRewrite(payload.Domain, payload.Answer, "created-1"));
                }
                return JsonResponse("""{"id":"created-1"}""");
            }

            if (request.RequestUri?.AbsolutePath.EndsWith("/control/rewrite/delete", StringComparison.Ordinal) == true)
            {
                DeleteCalls++;
                if ((int)DeleteStatusCode is >= 200 and < 300)
                {
                    var payload = await request.Content!.ReadFromJsonAsync<RewritePayload>(cancellationToken: cancellationToken)
                        ?? throw new InvalidOperationException("Missing rewrite payload.");
                    if (PersistMutations)
                    {
                        var index = _rewrites.FindIndex(x =>
                            string.Equals(x.Domain, payload.Domain, StringComparison.OrdinalIgnoreCase) &&
                            string.Equals(x.Answer, payload.Answer, StringComparison.Ordinal));
                        if (index >= 0)
                        {
                            _rewrites.RemoveAt(index);
                        }
                    }
                }
                return new HttpResponseMessage(DeleteStatusCode)
                {
                    Content = new StringContent("{}", Encoding.UTF8, "application/json"),
                };
            }

            return new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent("{}", Encoding.UTF8, "application/json"),
            };
        }

        private static List<RemoteRewrite> ParseRewrites(string json)
        {
            using var doc = JsonDocument.Parse(json);
            return doc.RootElement.GetProperty("rewrites").EnumerateArray().Select(x =>
                new RemoteRewrite(
                    x.GetProperty("domain").GetString() ?? string.Empty,
                    x.GetProperty("answer").GetString() ?? string.Empty,
                    x.TryGetProperty("id", out var id) ? id.GetString() ?? string.Empty : string.Empty))
                .ToList();
        }

        private static HttpResponseMessage JsonResponse(string json) => new(HttpStatusCode.OK)
        {
            Content = new StringContent(json, Encoding.UTF8, "application/json"),
        };

        private sealed record RewritePayload(string Domain, string Answer);

        private sealed record RemoteRewrite(string Domain, string Answer, string Id);
    }
}
