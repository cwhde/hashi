using System.Security.Cryptography;
using System.Text;
using Hashi.Core.Auth;
using Hashi.Core.Connections;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Connections;
using Hashi.Infrastructure.Notifications;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Hashi.UnitTests.Fakes;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;
using Xunit;

namespace Hashi.UnitTests;

public sealed class AccessLogIngestWorkerTests
{
    [Fact]
    public async Task AccessLogIngestWorker_ingests_traefik_lines_and_updates_abuse_state()
    {
        var databaseName = Guid.NewGuid().ToString();
        var fakeSsh = new FakeSshRemoteExecutor();
        var vault = new VaultSessionState();
        vault.Unlock(RandomNumberGenerator.GetBytes(32));

        var services = new ServiceCollection();
        services.AddDbContext<HashiDbContext>(options => options.UseInMemoryDatabase(databaseName));
        services.AddSingleton(vault);
        services.AddSingleton(new ServiceSyncVaultState());
        services.AddSingleton<ISshRemoteExecutor>(fakeSsh);
        services.AddScoped<AuditService>();
        services.AddScoped<ConnectionTargetResolver>();
        services.AddScoped<SecretRecordService>();
        services.AddHttpClient();
        services.AddScoped<NotificationDispatcher>();
        services.AddScoped<NotificationRoutingService>();
        services.AddScoped(sp => TestPlatformHelpers.CreateFirewallApply(
            sp.GetRequiredService<HashiDbContext>(),
            fakeSsh,
            vault));
        services.AddScoped<SecurityIngestionService>();

        using var provider = services.BuildServiceProvider();
        using (var scope = provider.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
            var secrets = scope.ServiceProvider.GetRequiredService<SecretRecordService>();
            var secret = await secrets.StoreAsync(
                SecretPurpose.SshCredential,
                "SSH: traefik-1",
                ConnectionSshCredentialResolver.SerializeCredentialPayload("password", "pw", null, null));

            db.Connections.Add(new ConnectionEntity
            {
                Name = "Traefik Host 1",
                Type = ConnectionTypeNames.TraefikHost,
                Enabled = true,
                SecretId = secret.Id,
                SettingsJson = """
                    {
                      "Host": "10.0.0.10",
                      "Port": 22,
                      "Username": "root",
                      "AccessLogPath": "/var/log/hashi/traefik/access.log"
                    }
                    """,
            });
            await db.SaveChangesAsync();
        }

        fakeSsh.ReadFiles["/var/log/hashi/traefik/access.log"] = Encoding.UTF8.GetBytes(
            "{\"ClientAddr\":\"198.51.100.10:43210\",\"RequestHost\":\"app.example.com\",\"RequestPath\":\"/login\",\"RequestMethod\":\"POST\",\"DownstreamStatus\":404,\"RequestHeaders\":{\"X-Request-Id\":\"req-1\",\"User-Agent\":\"Mozilla/5.0\"}}\n" +
            "{\"ClientAddr\":\"198.51.100.10:43211\",\"RequestHost\":\"app.example.com\",\"RequestPath\":\"/admin\",\"DownstreamStatus\":429}\n");

        var worker = new AccessLogIngestWorker(
            provider.GetRequiredService<IServiceScopeFactory>(),
            NullLogger<AccessLogIngestWorker>.Instance);

        var firstRun = await worker.ProcessOnceAsync();
        var secondRun = await worker.ProcessOnceAsync();

        Assert.Equal(1, firstRun.HostsProcessed);
        Assert.Equal(2, firstRun.LinesProcessed);
        Assert.Equal(0, firstRun.HostErrors);
        Assert.Equal(0, secondRun.LinesProcessed);

        using var verifyScope = provider.CreateScope();
        var verifyDb = verifyScope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var bucket = await verifyDb.AbuseBuckets.SingleAsync();
        var events = await verifyDb.AccessLogEvents.ToListAsync();
        var securityEvents = await verifyDb.SecurityEvents.ToListAsync();
        var cursor = await verifyDb.AccessLogCursors.SingleAsync();

        Assert.Equal("198.51.100.10", bucket.ClientIp);
        Assert.Equal(4, bucket.Score);
        Assert.Equal(2, events.Count);
        Assert.Contains(securityEvents, x =>
            x.RequestId == "req-1"
            && x.RequestMethod == "POST"
            && x.UserAgentHash == Sha256Hex("Mozilla/5.0"));
        Assert.True(cursor.ByteOffset > 0);
    }

    private static string Sha256Hex(string value)
        => Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(value))).ToLowerInvariant();
}
