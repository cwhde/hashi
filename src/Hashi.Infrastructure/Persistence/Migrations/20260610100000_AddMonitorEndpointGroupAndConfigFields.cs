using Microsoft.EntityFrameworkCore.Migrations;

namespace Hashi.Infrastructure.Persistence.Migrations;

public partial class AddMonitorEndpointGroupAndConfigFields : Migration
{
    protected override void Up(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.AddColumn<string>(
            name: "Group",
            table: "monitor_endpoints",
            type: "character varying(128)",
            maxLength: 128,
            nullable: true);

        migrationBuilder.AddColumn<int>(
            name: "CheckIntervalSeconds",
            table: "monitor_endpoints",
            type: "integer",
            nullable: true);

        migrationBuilder.AddColumn<int>(
            name: "TimeoutSeconds",
            table: "monitor_endpoints",
            type: "integer",
            nullable: true);

        migrationBuilder.CreateIndex(
            name: "IX_monitor_endpoints_Group",
            table: "monitor_endpoints",
            column: "Group");
    }

    protected override void Down(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.DropIndex(
            name: "IX_monitor_endpoints_Group",
            table: "monitor_endpoints");

        migrationBuilder.DropColumn(
            name: "Group",
            table: "monitor_endpoints");

        migrationBuilder.DropColumn(
            name: "CheckIntervalSeconds",
            table: "monitor_endpoints");

        migrationBuilder.DropColumn(
            name: "TimeoutSeconds",
            table: "monitor_endpoints");
    }
}
