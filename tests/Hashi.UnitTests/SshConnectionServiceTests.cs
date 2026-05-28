using System.Security.Cryptography;
using Hashi.Core.Auth;
using Hashi.Core.Connections;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Connections;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Hashi.UnitTests.Fakes;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class SshConnectionServiceTests
{
    [Theory]
    [InlineData(ConnectionTypeNames.TraefikHost)]
    [InlineData(ConnectionTypeNames.FirewallHost)]
    public async Task CreateAsync_stores_runtime_ssh_credentials_for_service_sync(string connectionType)
    {
        await using var db = CreateDb();
        var serviceSync = ReadyServiceSync();
        var secrets = CreateSecrets(db, serviceSync);
        var service = new SshConnectionService(db, new FakeSshRemoteExecutor(), secrets, new AuditService(db));

        var connection = await service.CreateAsync(
            "runtime",
            connectionType,
            new SshConnectionSettings("10.0.0.10", 22, "root", OsFamily.Unknown, null, null),
            "password",
            "ssh-password",
            privateKeyPem: null,
            privateKeyPassphrase: null);

        var secret = await db.SecretRecords.SingleAsync(x => x.Id == connection.SecretId);
        Assert.True(secret.IsServiceSyncEligible);
        Assert.NotNull(secret.ServiceWrappedDekBlob);

        var decrypted = await secrets.DecryptForServiceSyncAsync(secret.Id);
        Assert.NotNull(decrypted);
        Assert.Contains("ssh-password", System.Text.Encoding.UTF8.GetString(decrypted), StringComparison.Ordinal);
    }

    private static SecretRecordService CreateSecrets(HashiDbContext db, ServiceSyncVaultState serviceSync)
    {
        var vault = new VaultSessionState();
        vault.Unlock(RandomNumberGenerator.GetBytes(32));
        return new SecretRecordService(db, vault, serviceSync);
    }

    private static ServiceSyncVaultState ReadyServiceSync()
    {
        var serviceSync = new ServiceSyncVaultState();
        serviceSync.Initialize(RandomNumberGenerator.GetBytes(32));
        return serviceSync;
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
