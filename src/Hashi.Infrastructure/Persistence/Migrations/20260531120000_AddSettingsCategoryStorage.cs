using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations;

[DbContext(typeof(HashiDbContext))]
[Migration("20260531120000_AddSettingsCategoryStorage")]
public partial class AddSettingsCategoryStorage : Migration
{
    protected override void Up(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.AddColumn<string>(
            name: "OverviewWidgetsJson",
            table: "app_settings",
            type: "text",
            nullable: false,
            defaultValue: "{}");

        migrationBuilder.AddColumn<string>(
            name: "SettingsCategoriesJson",
            table: "app_settings",
            type: "text",
            nullable: false,
            defaultValue: "{}");
    }

    protected override void Down(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.DropColumn(
            name: "OverviewWidgetsJson",
            table: "app_settings");

        migrationBuilder.DropColumn(
            name: "SettingsCategoriesJson",
            table: "app_settings");
    }
}
