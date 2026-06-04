using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Migrations;
using Xunit;

namespace Hashi.UnitTests;

public sealed class PersistenceMigrationDiscoveryTests
{
    [Fact]
    public void Hand_authored_migrations_are_discoverable_by_ef()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseNpgsql("Host=localhost;Database=hashi;Username=hashi;Password=hashi")
            .Options;

        using var db = new HashiDbContext(options);
        var migrations = db.GetService<IMigrationsAssembly>().Migrations.Keys;

        Assert.Contains("20260528120000_DropPendingAcmeEabJson", migrations);
        Assert.Contains("20260531120000_AddSettingsCategoryStorage", migrations);
        Assert.Contains("20260604121917_AddHashiAddendumDataFoundation", migrations);
    }
}
