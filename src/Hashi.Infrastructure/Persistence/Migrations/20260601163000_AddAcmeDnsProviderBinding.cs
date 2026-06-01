using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations;

[DbContext(typeof(HashiDbContext))]
[Migration("20260601163000_AddAcmeDnsProviderBinding")]
public partial class AddAcmeDnsProviderBinding : Migration
{
    protected override void Up(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.AddColumn<Guid>(
            name: "AcmeDnsProviderConnectionId",
            table: "app_settings",
            type: "uuid",
            nullable: true);

        migrationBuilder.CreateIndex(
            name: "IX_app_settings_AcmeDnsProviderConnectionId",
            table: "app_settings",
            column: "AcmeDnsProviderConnectionId");
    }

    protected override void Down(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.DropIndex(
            name: "IX_app_settings_AcmeDnsProviderConnectionId",
            table: "app_settings");

        migrationBuilder.DropColumn(
            name: "AcmeDnsProviderConnectionId",
            table: "app_settings");
    }
}
