using Fido2NetLib;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
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

        services.AddScoped<SetupStateService>();
        services.AddScoped<AuditService>();
        services.AddScoped<AppSettingsService>();
        services.AddScoped<BootstrapAuthService>();
        services.AddScoped<PasskeyAuthService>();
        services.AddScoped<VaultService>();
        services.AddScoped<SecretRecordService>();
        services.AddScoped<SetupCompletionService>();
        services.AddScoped<WebAuthnChallengeStore>();

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
