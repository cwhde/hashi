using FluentValidation;
using Hashi.Core.Hosting;
using Hashi.Core.Dns;
using Hashi.Core.Validation;
using Fido2NetLib;
using Hashi.Infrastructure.Notifications;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Connections;
using Hashi.Core.Connections;
using Hashi.Infrastructure.Ssh;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Dns;
using Hashi.Infrastructure.Providers.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Services;
using Hashi.Infrastructure.Sync;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;

namespace Hashi.Infrastructure;

public static class DependencyInjection
{
    public static IServiceCollection AddHashiInfrastructure(this IServiceCollection services, IConfiguration configuration)
    {
        var connectionString = configuration.GetConnectionString("Hashi")
            ?? "Host=localhost;Port=5432;Database=hashi;Username=hashi;Password=hashi";

        services.AddDbContext<HashiDbContext>(options =>
            options.UseNpgsql(connectionString, npgsql => npgsql.MigrationsAssembly(typeof(HashiDbContext).Assembly.FullName)));

        services.AddMemoryCache();
        services.AddDataProtection();
        services.AddHttpContextAccessor();
        services.AddSingleton<VaultSessionState>();
        services.AddSingleton<ServiceSyncVaultState>();
        services.AddSingleton<ReauthenticationState>();
        services.TryAddSingleton(_ => HashiPortOptions.FromConfiguration(configuration));

        services.AddHttpClient("hetzner-dns", client =>
        {
            client.BaseAddress = new Uri("https://dns.hetzner.com/api/v1/");
        });
        services.AddHttpClient("oidc-edge");
        services.AddHttpClient("monitor-checks");
        services.AddHttpClient("adguard");
        services.AddHttpClient("maxmind-geoip", client =>
        {
            client.BaseAddress = new Uri("https://download.maxmind.com/");
        });

        services.AddScoped<SetupStateService>();
        services.AddScoped<AuditService>();
        services.AddScoped<AppSettingsService>();
        services.AddScoped<BootstrapAuthService>();
        services.AddScoped<PasskeyAuthService>();
        services.AddScoped<VaultService>();
        services.AddScoped<SecretRecordService>();
        services.AddScoped<SetupCompletionService>();
        services.AddScoped<SystemResourceSetupService>();
        services.AddScoped<WebAuthnChallengeStore>();
        services.AddScoped<DnsConnectionService>();
        services.AddScoped<DnsRecordService>();
        services.AddScoped<SshConnectionService>();
        services.AddScoped<CertificateSetupService>();
        services.AddScoped<TraefikEntryPointService>();
        services.AddScoped<HashiInternalUrlResolver>();
        services.AddScoped<FirewallTrustedIpResolver>();
        services.AddScoped<ResourceService>();
        services.AddScoped<TraefikUserMiddlewareService>();
        services.AddScoped<TraefikPlatformService>();
        services.AddScoped<TraefikSyncService>();
        services.AddScoped<FirewallPlatformService>();
        services.AddScoped<FirewallApplyService>();
        services.AddScoped<MonitoringService>();
        services.AddScoped<PublicDashboardService>();
        services.AddScoped<EdgeAuthService>();
        services.AddSingleton<GeoIpLookupService>();
        services.AddScoped<OidcEdgeAuthService>();
        services.AddScoped<OidcProviderAdminService>();
        services.AddScoped<NotificationRoutingService>();
        services.AddScoped<SecurityIngestionService>();
        services.AddScoped<BackgroundJobService>();
        services.AddScoped<GeoIpSettingsService>();
        services.AddScoped<GeoIpUpdateService>();
        services.AddScoped<IGeoIpDatabaseDownloader, MaxMindGeoIpDatabaseDownloader>();
        services.AddScoped<AdGuardSyncService>();
        services.AddScoped<ConnectionTargetResolver>();
        services.AddScoped<ScriptExecutionService>();
        services.AddScoped<PulseAgentService>();
        services.AddScoped<NotificationDispatcher>();
        services.AddSingleton<IDnsProviderFactory, DnsProviderFactory>();
        services.AddSingleton<ISshRemoteExecutor, SshRemoteExecutor>();
        services.AddValidatorsFromAssemblyContaining<CreateResourceRequestValidator>();

        var skipStartupHooks = configuration.GetValue<bool>("Hashi:SkipStartupHooks")
            || string.Equals(Environment.GetEnvironmentVariable("HASHI_SKIP_STARTUP_HOOKS"), "1", StringComparison.Ordinal);
        services.AddScoped<SyncRunService>();
        services.AddScoped<SyncOrchestratorService>();
        services.AddHostedService<ServiceSyncVaultBootstrapper>();
        if (!skipStartupHooks)
        {
            services.AddHostedService<MonitorCheckWorker>();
            services.AddHostedService<MonitorRollupWorker>();
            services.AddHostedService<SyncOrchestratorHostedService>();
            services.AddHostedService<ScriptCronHostedService>();
            services.AddHostedService<AccessLogIngestWorker>();
            services.AddHostedService<GeoIpUpdateWorker>();
        }

        var fidoDomain = configuration["Hashi:WebAuthn:ServerDomain"] ?? "localhost";
        var fidoOrigin = configuration["Hashi:WebAuthn:Origin"] ?? "http://localhost:8080";
        services.AddSingleton<IFido2>(_ => new Fido2(new Fido2Configuration
        {
            ServerDomain = fidoDomain,
            ServerName = "Hashi",
            Origins = new HashSet<string> { fidoOrigin },
            TimestampDriftTolerance = 300000,
        }));

        return services;
    }
}
