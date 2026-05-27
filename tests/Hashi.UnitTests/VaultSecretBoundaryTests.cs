using System.Security.Claims;
using Hashi.Core.Auth;
using Hashi.Core.Setup;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Http;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Xunit;

namespace Hashi.UnitTests;

public sealed class VaultSecretBoundaryTests
{
    [Fact]
    public void Admin_vault_unlock_is_bound_to_current_session()
    {
        var accessor = new HttpContextAccessor();
        var vault = new VaultSessionState(accessor);

        accessor.HttpContext = ContextForSession("session-a");
        vault.Unlock(new byte[32]);

        Assert.True(vault.IsUnlocked);

        accessor.HttpContext = ContextForSession("session-b");
        Assert.False(vault.IsUnlocked);
    }

    [Fact]
    public async Task Service_sync_decrypts_only_explicitly_eligible_secrets()
    {
        await using var db = CreateDb();
        var adminVault = new VaultSessionState();
        var serviceSync = new ServiceSyncVaultState();
        adminVault.Unlock(new byte[32]);
        serviceSync.Initialize([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32]);
        var secrets = new SecretRecordService(db, adminVault, serviceSync);

        var adminOnly = await secrets.StoreAsync(
            SecretPurpose.ScriptEnvironment,
            "admin only",
            [42]);
        var eligible = await secrets.StoreAsync(
            SecretPurpose.DnsProviderToken,
            "routine dns",
            [43],
            serviceSyncEligible: true);

        Assert.Null(await secrets.DecryptForServiceSyncAsync(adminOnly.Id));
        Assert.Equal([43], await secrets.DecryptForServiceSyncAsync(eligible.Id));
    }

    [Fact]
    public async Task Service_sync_eligibility_flag_blocks_existing_service_wrapped_dek()
    {
        await using var db = CreateDb();
        var adminVault = new VaultSessionState();
        var serviceSync = new ServiceSyncVaultState();
        adminVault.Unlock(new byte[32]);
        serviceSync.Initialize([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32]);
        var secrets = new SecretRecordService(db, adminVault, serviceSync);

        var stored = await secrets.StoreAsync(
            SecretPurpose.DnsProviderToken,
            "legacy wrapped secret",
            [44],
            serviceSyncEligible: true);
        var entity = await db.SecretRecords.SingleAsync(x => x.Id == stored.Id);
        entity.IsServiceSyncEligible = false;
        await db.SaveChangesAsync();

        Assert.Null(await secrets.DecryptForServiceSyncAsync(stored.Id));
    }

    [Fact]
    public async Task Service_sync_wrap_validation_does_not_unlock_admin_vault_status()
    {
        await using var db = CreateDb();
        db.SetupStates.Add(new SetupStateEntity { IsComplete = true });
        db.VaultWrappedKeys.Add(new VaultWrappedKeyEntity
        {
            WrapMethod = VaultWrapMethodNames.RecoveryKey,
            WrappedKeyBlob = [1],
            RecoveryKeyHash = "configured",
        });
        await db.SaveChangesAsync();

        var accessor = new HttpContextAccessor();
        var adminVault = new VaultSessionState(accessor);
        var serviceSync = new ServiceSyncVaultState();
        serviceSync.Initialize(new byte[32]);
        var vault = new VaultService(
            db,
            adminVault,
            serviceSync,
            new SetupStateService(db, NullLogger<SetupStateService>.Instance),
            new AuditService(db),
            NullLogger<VaultService>.Instance);

        var status = await vault.GetStatusAsync();

        Assert.Equal(VaultLockState.Locked, status.LockState);
        Assert.True(status.ServiceSyncVaultReady);
    }

    private static DefaultHttpContext ContextForSession(string sessionId)
    {
        var identity = new ClaimsIdentity(
            [new Claim(ClaimTypes.NameIdentifier, sessionId), new Claim(ClaimTypes.Name, "admin")],
            "Test");
        return new DefaultHttpContext { User = new ClaimsPrincipal(identity) };
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
