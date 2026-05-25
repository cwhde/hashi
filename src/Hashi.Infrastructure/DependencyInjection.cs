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

        services.AddScoped<SetupStateService>();
        services.AddScoped<AuditService>();
        services.AddScoped<AppSettingsService>();

        return services;
    }
}
