using Microsoft.EntityFrameworkCore.Migrations;

namespace Hashi.Infrastructure.Persistence.Migrations;

public partial class AddResourceDetectedFirewallHostId : Migration
{
    protected override void Up(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.AddColumn<Guid>(
            name: "DetectedFirewallHostId",
            table: "resources",
            type: "uuid",
            nullable: true);
    }

    protected override void Down(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.DropColumn(
            name: "DetectedFirewallHostId",
            table: "resources");
    }
}
