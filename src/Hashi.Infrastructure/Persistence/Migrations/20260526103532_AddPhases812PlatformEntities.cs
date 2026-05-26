using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddPhases812PlatformEntities : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<DateTimeOffset>(
                name: "DnsPendingAtUtc",
                table: "pulse_agents",
                type: "timestamp with time zone",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "LastAgentVersion",
                table: "pulse_agents",
                type: "text",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "LastHostname",
                table: "pulse_agents",
                type: "text",
                nullable: true);

            migrationBuilder.AddColumn<int>(
                name: "EdgeSsoSessionHours",
                table: "app_settings",
                type: "integer",
                nullable: false,
                defaultValue: 0);

            migrationBuilder.AddColumn<int>(
                name: "MonitorCheckIntervalSeconds",
                table: "app_settings",
                type: "integer",
                nullable: false,
                defaultValue: 0);

            migrationBuilder.AddColumn<int>(
                name: "MonitorCheckTimeoutSeconds",
                table: "app_settings",
                type: "integer",
                nullable: false,
                defaultValue: 0);

            migrationBuilder.AddColumn<int>(
                name: "MonitorDegradedLatencyMs",
                table: "app_settings",
                type: "integer",
                nullable: false,
                defaultValue: 0);

            migrationBuilder.AddColumn<int>(
                name: "MonitorSampleRetentionDays",
                table: "app_settings",
                type: "integer",
                nullable: false,
                defaultValue: 0);

            migrationBuilder.CreateTable(
                name: "edge_sessions",
                columns: table => new
                {
                    SessionKey = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    OidcProviderId = table.Column<Guid>(type: "uuid", nullable: false),
                    Subject = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false),
                    ExpiresAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    CreatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_edge_sessions", x => x.SessionKey);
                });

            migrationBuilder.CreateTable(
                name: "monitor_events",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    MonitorEndpointId = table.Column<Guid>(type: "uuid", nullable: false),
                    PreviousStatus = table.Column<string>(type: "character varying(16)", maxLength: 16, nullable: false),
                    NewStatus = table.Column<string>(type: "character varying(16)", maxLength: 16, nullable: false),
                    LatencyMs = table.Column<int>(type: "integer", nullable: true),
                    OccurredAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_monitor_events", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "security_events",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    Category = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    Action = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                    ClientIp = table.Column<string>(type: "text", nullable: true),
                    Host = table.Column<string>(type: "text", nullable: true),
                    Path = table.Column<string>(type: "text", nullable: true),
                    DetailsJson = table.Column<string>(type: "text", nullable: true),
                    OccurredAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_security_events", x => x.Id);
                });

            migrationBuilder.CreateIndex(
                name: "IX_edge_sessions_ExpiresAtUtc",
                table: "edge_sessions",
                column: "ExpiresAtUtc");

            migrationBuilder.CreateIndex(
                name: "IX_monitor_events_MonitorEndpointId",
                table: "monitor_events",
                column: "MonitorEndpointId");

            migrationBuilder.CreateIndex(
                name: "IX_monitor_events_OccurredAtUtc",
                table: "monitor_events",
                column: "OccurredAtUtc");

            migrationBuilder.CreateIndex(
                name: "IX_security_events_OccurredAtUtc",
                table: "security_events",
                column: "OccurredAtUtc");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "edge_sessions");

            migrationBuilder.DropTable(
                name: "monitor_events");

            migrationBuilder.DropTable(
                name: "security_events");

            migrationBuilder.DropColumn(
                name: "DnsPendingAtUtc",
                table: "pulse_agents");

            migrationBuilder.DropColumn(
                name: "LastAgentVersion",
                table: "pulse_agents");

            migrationBuilder.DropColumn(
                name: "LastHostname",
                table: "pulse_agents");

            migrationBuilder.DropColumn(
                name: "EdgeSsoSessionHours",
                table: "app_settings");

            migrationBuilder.DropColumn(
                name: "MonitorCheckIntervalSeconds",
                table: "app_settings");

            migrationBuilder.DropColumn(
                name: "MonitorCheckTimeoutSeconds",
                table: "app_settings");

            migrationBuilder.DropColumn(
                name: "MonitorDegradedLatencyMs",
                table: "app_settings");

            migrationBuilder.DropColumn(
                name: "MonitorSampleRetentionDays",
                table: "app_settings");
        }
    }
}
