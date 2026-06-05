using Hashi.Infrastructure.Auth;
using Hashi.Core.Hosting;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Hashi.Infrastructure.Sync;
using Hashi.UnitTests.Fakes;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.Logging.Abstractions;

namespace Hashi.UnitTests.Fakes;

public static class TestPlatformHelpers
{
    public static TraefikPlatformService CreateTraefikPlatform(
        HashiDbContext db,
        VaultSessionState? vault = null,
        ServiceSyncVaultState? serviceSync = null,
        HashiPortOptions? ports = null)
    {
        vault ??= new VaultSessionState();
        serviceSync ??= new ServiceSyncVaultState();
        var settings = new AppSettingsService(db);
        var userMiddlewares = new TraefikUserMiddlewareService(db);
        var audit = new AuditService(db);
        var secrets = new SecretRecordService(db, vault, serviceSync);
        var certificate = new CertificateSetupService(db, settings, secrets, vault, audit);
        var entryPoints = new TraefikEntryPointService(db);
        return new TraefikPlatformService(
            db,
            settings,
            userMiddlewares,
            certificate,
            entryPoints,
            new HashiInternalUrlResolver(ports ?? new HashiPortOptions()));
    }

    public static TraefikSyncService CreateTraefikSync(
        HashiDbContext db,
        FakeSshRemoteExecutor? ssh = null,
        VaultSessionState? vault = null,
        ServiceSyncVaultState? serviceSync = null)
    {
        ssh ??= new FakeSshRemoteExecutor();
        vault ??= new VaultSessionState();
        serviceSync ??= new ServiceSyncVaultState();
        var settings = new AppSettingsService(db);
        var secrets = new SecretRecordService(db, vault, serviceSync);
        var certificate = new CertificateSetupService(db, settings, secrets, vault, new AuditService(db));
        var traefik = CreateTraefikPlatform(db, vault, serviceSync);
        var audit = new AuditService(db);
        return new TraefikSyncService(db, ssh, traefik, certificate, secrets, audit, new ConnectionTargetResolver(db, audit));
    }

    public static FirewallApplyService CreateFirewallApply(HashiDbContext db, FakeSshRemoteExecutor? ssh = null, VaultSessionState? vault = null)
    {
        ssh ??= new FakeSshRemoteExecutor();
        vault ??= new VaultSessionState();
        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var audit = new AuditService(db);
        return new FirewallApplyService(
            db,
            ssh,
            secrets,
            audit,
            new FirewallTrustedIpResolver(NullLogger<FirewallTrustedIpResolver>.Instance),
            new SyncRunService(db),
            new ConnectionTargetResolver(db, audit));
    }

    public static ResourceService CreateResourceService(HashiDbContext db, GeoIpLookupService? geoIp = null)
        => new(
            db,
            new AuditService(db),
            new TraefikEntryPointService(db),
            geoIp ?? new GeoIpLookupService(new ConfigurationBuilder().Build(), NullLogger<GeoIpLookupService>.Instance));
}
