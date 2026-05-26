using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Design;
using Microsoft.Extensions.Configuration;

namespace Hashi.Infrastructure.Persistence;

public sealed class HashiDbContextFactory : IDesignTimeDbContextFactory<HashiDbContext>
{
    public HashiDbContext CreateDbContext(string[] args)
    {
        var configuration = new ConfigurationBuilder()
            .SetBasePath(Path.Combine(Directory.GetCurrentDirectory(), "../Hashi.Api"))
            .AddJsonFile("appsettings.json", optional: false)
            .AddJsonFile("appsettings.Development.json", optional: true)
            .AddEnvironmentVariables()
            .Build();

        var connectionString = configuration.GetConnectionString("Hashi")
            ?? "Host=localhost;Port=5432;Database=hashi;Username=hashi;Password=hashi";

        var optionsBuilder = new DbContextOptionsBuilder<HashiDbContext>();
        optionsBuilder.UseNpgsql(
            connectionString,
            npgsql => npgsql.MigrationsAssembly(typeof(HashiDbContext).Assembly.FullName));

        return new HashiDbContext(optionsBuilder.Options);
    }
}
