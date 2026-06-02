using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations;

[DbContext(typeof(HashiDbContext))]
[Migration("20260602143000_AddResourceProxyProtocolAndMonitoringHints")]
public partial class AddResourceProxyProtocolAndMonitoringHints : Migration
{
    protected override void Up(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.AddColumn<string>(
            name: "MonitoringProtocolHint",
            table: "resources",
            type: "character varying(16)",
            maxLength: 16,
            nullable: true);

        migrationBuilder.AddColumn<bool>(
            name: "TcpProxyProtocolEnabled",
            table: "resources",
            type: "boolean",
            nullable: true);
    }

    protected override void Down(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.DropColumn(
            name: "MonitoringProtocolHint",
            table: "resources");

        migrationBuilder.DropColumn(
            name: "TcpProxyProtocolEnabled",
            table: "resources");
    }
}
