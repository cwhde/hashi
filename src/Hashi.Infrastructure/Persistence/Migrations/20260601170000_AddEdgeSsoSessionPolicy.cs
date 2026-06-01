using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations;

[DbContext(typeof(HashiDbContext))]
[Migration("20260601170000_AddEdgeSsoSessionPolicy")]
public partial class AddEdgeSsoSessionPolicy : Migration
{
    protected override void Up(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.AddColumn<int>(
            name: "EdgeSsoIdleTimeoutMinutes",
            table: "app_settings",
            type: "integer",
            nullable: false,
            defaultValue: 60);

        migrationBuilder.AddColumn<int>(
            name: "EdgeSsoRememberDeviceDays",
            table: "app_settings",
            type: "integer",
            nullable: false,
            defaultValue: 30);

        migrationBuilder.AddColumn<DateTimeOffset>(
            name: "LastSeenAtUtc",
            table: "edge_sessions",
            type: "timestamp with time zone",
            nullable: false,
            defaultValueSql: "now()");

        migrationBuilder.AddColumn<bool>(
            name: "RememberMe",
            table: "edge_sessions",
            type: "boolean",
            nullable: false,
            defaultValue: false);

        migrationBuilder.CreateIndex(
            name: "IX_edge_sessions_LastSeenAtUtc",
            table: "edge_sessions",
            column: "LastSeenAtUtc");
    }

    protected override void Down(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.DropIndex(
            name: "IX_edge_sessions_LastSeenAtUtc",
            table: "edge_sessions");

        migrationBuilder.DropColumn(
            name: "EdgeSsoIdleTimeoutMinutes",
            table: "app_settings");

        migrationBuilder.DropColumn(
            name: "EdgeSsoRememberDeviceDays",
            table: "app_settings");

        migrationBuilder.DropColumn(
            name: "LastSeenAtUtc",
            table: "edge_sessions");

        migrationBuilder.DropColumn(
            name: "RememberMe",
            table: "edge_sessions");
    }
}
