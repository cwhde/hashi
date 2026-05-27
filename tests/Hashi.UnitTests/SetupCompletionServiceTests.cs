using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Xunit;

namespace Hashi.UnitTests;

public sealed class SetupCompletionServiceTests
{
    [Fact]
    public async Task TryComplete_fails_until_required_enabled_connections_and_firewall_host_exist_presence_only()
    {
        await using var db = CreateDb();
        var vaultSession = new VaultSessionState();
        await SeedExistingCompletionPrerequisitesAsync(db, vaultSession);
        var service = CreateService(db, vaultSession);

        var missingDns = await service.TryCompleteAsync();

        Assert.False(missingDns.Succeeded);
        Assert.Contains("DNS provider", missingDns.Error);

        db.Connections.Add(new ConnectionEntity
        {
            Name = "DNS",
            Type = ConnectionTypeNames.DnsProvider,
            Enabled = true,
        });
        await db.SaveChangesAsync();

        var missingTraefik = await service.TryCompleteAsync();

        Assert.False(missingTraefik.Succeeded);
        Assert.Contains("Traefik host", missingTraefik.Error);

        var traefikConnection = new ConnectionEntity
        {
            Name = "Traefik",
            Type = ConnectionTypeNames.TraefikHost,
            Enabled = true,
        };
        db.Connections.Add(traefikConnection);
        await db.SaveChangesAsync();

        var missingFirewallHost = await service.TryCompleteAsync();

        Assert.False(missingFirewallHost.Succeeded);
        Assert.Contains("firewall host", missingFirewallHost.Error);

        db.FirewallHosts.Add(new FirewallHostEntity
        {
            ConnectionId = traefikConnection.Id,
            Name = "Firewall",
            Domain = "fw.example.com",
            LinkedTraefikHost = "traefik.example.com",
            InternalTraefikIp = "10.0.0.2",
        });
        await db.SaveChangesAsync();

        var completed = await service.TryCompleteAsync();

        Assert.True(completed.Succeeded);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private static SetupCompletionService CreateService(HashiDbContext db, VaultSessionState vaultSession)
        => new(
            db,
            new SetupStateService(db, NullLogger<SetupStateService>.Instance),
            vaultSession,
            new AuditService(db));

    private static async Task SeedExistingCompletionPrerequisitesAsync(HashiDbContext db, VaultSessionState vaultSession)
    {
        db.SetupStates.Add(new SetupStateEntity
        {
            HttpsDomainVerifiedAtUtc = DateTimeOffset.UtcNow,
        });
        db.PasskeyCredentials.Add(new PasskeyCredentialEntity
        {
            CredentialId = [1, 2, 3],
            PublicKey = [4, 5, 6],
        });
        db.VaultWrappedKeys.Add(new VaultWrappedKeyEntity
        {
            WrapMethod = VaultWrapMethodNames.RecoveryKey,
            WrappedKeyBlob = [7, 8, 9],
        });
        await db.SaveChangesAsync();
        vaultSession.Unlock(new byte[32]);
    }
}
