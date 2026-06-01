using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations;

[DbContext(typeof(HashiDbContext))]
[Migration("20260601150000_AddManualDnsDashboardSelection")]
public partial class AddManualDnsDashboardSelection : Migration
{
    protected override void Up(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.AddColumn<bool>(
            name: "DashboardEnabled",
            table: "dns_records",
            type: "boolean",
            nullable: false,
            defaultValue: false);

        migrationBuilder.AddColumn<string>(
            name: "DashboardDisplayName",
            table: "dns_records",
            type: "character varying(128)",
            maxLength: 128,
            nullable: true);
    }

    protected override void Down(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.DropColumn(
            name: "DashboardEnabled",
            table: "dns_records");

        migrationBuilder.DropColumn(
            name: "DashboardDisplayName",
            table: "dns_records");
    }
}
