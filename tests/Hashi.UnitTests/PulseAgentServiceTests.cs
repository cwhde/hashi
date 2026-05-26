using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Providers.Dns;
using Hashi.Infrastructure.Services;
using Hashi.UnitTests.Fakes;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;
using Xunit;

namespace Hashi.UnitTests;

public sealed class PulseAgentServiceTests
{
    [Fact]
    public async Task RevokeAgent_rejects_subsequent_heartbeat()
    {
        await using var db = CreateDb();
        var service = CreateService(db);
        var created = await service.CreateAgentAsync(new Hashi.Contracts.Api.CreatePulseAgentRequest("edge-1"));
        await service.RevokeAgentAsync(created.Id);

        var accepted = await service.AcceptHeartbeatAsync(
            created.Id,
            new Hashi.Contracts.Api.PulseHeartbeatAuthRequest(created.Token, "0.1.0", "host", ["10.0.0.5"]),
            "203.0.113.10");

        Assert.False(accepted);
    }

    private static PulseAgentService CreateService(HashiDbContext db)
    {
        var audit = new AuditService(db);
        var secrets = new SecretRecordService(db, new VaultSessionState(), new ServiceSyncVaultState());
        var httpClientFactory = new ServiceCollection().AddHttpClient().BuildServiceProvider().GetRequiredService<IHttpClientFactory>();
        var dns = new DnsConnectionService(
            db,
            new DnsProviderFactory(httpClientFactory, NullLogger<HetznerDnsProvider>.Instance),
            secrets,
            audit);
        return new PulseAgentService(db, dns, NullLogger<PulseAgentService>.Instance);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
