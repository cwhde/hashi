using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations;

[DbContext(typeof(HashiDbContext))]
[Migration("20260601180000_AddPulseAgentHeartbeatContract")]
public partial class AddPulseAgentHeartbeatContract : Migration
{
    protected override void Up(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.AddColumn<string>(
            name: "AllowedScopesJson",
            table: "pulse_agents",
            type: "jsonb",
            nullable: false,
            defaultValueSql: "'[\"heartbeat\"]'::jsonb");

        migrationBuilder.AddColumn<int>(
            name: "HeartbeatIntervalSeconds",
            table: "pulse_agents",
            type: "integer",
            nullable: false,
            defaultValue: 60);

        migrationBuilder.AddColumn<string>(
            name: "InstallType",
            table: "pulse_agents",
            type: "character varying(32)",
            maxLength: 32,
            nullable: false,
            defaultValue: "linux_service");

        migrationBuilder.AddColumn<string>(
            name: "LastDockerMetadataJson",
            table: "pulse_agents",
            type: "jsonb",
            nullable: true);

        migrationBuilder.AddColumn<string>(
            name: "LastPrivateIpv4CandidatesJson",
            table: "pulse_agents",
            type: "jsonb",
            nullable: false,
            defaultValueSql: "'[]'::jsonb");

        migrationBuilder.AddColumn<string>(
            name: "LastPrivateIpv6CandidatesJson",
            table: "pulse_agents",
            type: "jsonb",
            nullable: false,
            defaultValueSql: "'[]'::jsonb");

        migrationBuilder.AddColumn<string>(
            name: "LastSelectedInterface",
            table: "pulse_agents",
            type: "character varying(128)",
            maxLength: 128,
            nullable: true);

        migrationBuilder.AddColumn<string>(
            name: "LastSelectedIp",
            table: "pulse_agents",
            type: "text",
            nullable: true);

        migrationBuilder.CreateTable(
            name: "pulse_heartbeats",
            columns: table => new
            {
                Id = table.Column<Guid>(type: "uuid", nullable: false),
                PulseAgentId = table.Column<Guid>(type: "uuid", nullable: false),
                ReceivedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                AgentTimestampUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                RemotePublicIp = table.Column<string>(type: "text", nullable: true),
                Version = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                Hostname = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false),
                PrivateIpv4CandidatesJson = table.Column<string>(type: "jsonb", nullable: false),
                PrivateIpv6CandidatesJson = table.Column<string>(type: "jsonb", nullable: false),
                SelectedIp = table.Column<string>(type: "text", nullable: true),
                SelectedInterface = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: true),
                DockerMetadataJson = table.Column<string>(type: "jsonb", nullable: true),
            },
            constraints: table =>
            {
                table.PrimaryKey("PK_pulse_heartbeats", x => x.Id);
                table.ForeignKey(
                    name: "FK_pulse_heartbeats_pulse_agents_PulseAgentId",
                    column: x => x.PulseAgentId,
                    principalTable: "pulse_agents",
                    principalColumn: "Id",
                    onDelete: ReferentialAction.Cascade);
            });

        migrationBuilder.CreateIndex(
            name: "IX_pulse_agents_Status",
            table: "pulse_agents",
            column: "Status");

        migrationBuilder.CreateIndex(
            name: "IX_pulse_heartbeats_PulseAgentId_ReceivedAtUtc",
            table: "pulse_heartbeats",
            columns: new[] { "PulseAgentId", "ReceivedAtUtc" });
    }

    protected override void Down(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.DropTable(name: "pulse_heartbeats");

        migrationBuilder.DropIndex(
            name: "IX_pulse_agents_Status",
            table: "pulse_agents");

        migrationBuilder.DropColumn(
            name: "AllowedScopesJson",
            table: "pulse_agents");

        migrationBuilder.DropColumn(
            name: "HeartbeatIntervalSeconds",
            table: "pulse_agents");

        migrationBuilder.DropColumn(
            name: "InstallType",
            table: "pulse_agents");

        migrationBuilder.DropColumn(
            name: "LastDockerMetadataJson",
            table: "pulse_agents");

        migrationBuilder.DropColumn(
            name: "LastPrivateIpv4CandidatesJson",
            table: "pulse_agents");

        migrationBuilder.DropColumn(
            name: "LastPrivateIpv6CandidatesJson",
            table: "pulse_agents");

        migrationBuilder.DropColumn(
            name: "LastSelectedInterface",
            table: "pulse_agents");

        migrationBuilder.DropColumn(
            name: "LastSelectedIp",
            table: "pulse_agents");
    }
}
