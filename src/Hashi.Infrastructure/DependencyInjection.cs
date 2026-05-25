using Fido2NetLib;
using Hashi.Core.Dns;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Connections;
using Hashi.Infrastructure.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Providers.Dns;
using Hashi.Core.Connections;
using Hashi.Infrastructure.Ssh;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;

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
        services.AddSingleton<VaultSessionState>();
        services.AddSingleton<ServiceSyncVaultState>();

        services.AddHttpClient("hetzner-dns", client =>
        {
            client.BaseAddress = new Uri("https://dns.hetzner.com/api/v1/");
        });

        services.AddScoped<SetupStateService>();
        services.AddScoped<AuditService>();
        services.AddScoped<AppSettingsService>();
        services.AddScoped<BootstrapAuthService>();
        services.AddScoped<PasskeyAuthService>();
        services.AddScoped<VaultService>();
        services.AddScoped<SecretRecordService>();
        services.AddScoped<SetupCompletionService>();
        services.AddScoped<WebAuthnChallengeStore>();
        services.AddScoped<DnsConnectionService>();
        services.AddScoped<SshConnectionService>();
        services.AddSingleton<IDnsProviderFactory, DnsProviderFactory>();
        services.AddSingleton<ISshRemoteExecutor, SshRemoteExecutor>();

        services.AddHostedService<ServiceSyncVaultBootstrapper>();

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
