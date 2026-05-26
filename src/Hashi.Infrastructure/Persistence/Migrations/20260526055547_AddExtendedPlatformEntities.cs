using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddExtendedPlatformEntities : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AlterColumn<string>(
                name: "TokenHash",
                table: "pulse_agents",
                type: "character varying(128)",
                maxLength: 128,
                nullable: false,
                oldClrType: typeof(string),
                oldType: "text");

            migrationBuilder.CreateTable(
                name: "abuse_buckets",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ClientIp = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                    Score = table.Column<int>(type: "integer", nullable: false),
                    State = table.Column<string>(type: "text", nullable: false),
                    UpdatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_abuse_buckets", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "access_log_events",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ReceivedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    ClientIp = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                    Host = table.Column<string>(type: "text", nullable: false),
                    Path = table.Column<string>(type: "text", nullable: false),
                    StatusCode = table.Column<int>(type: "integer", nullable: false),
                    CountryCode = table.Column<string>(type: "text", nullable: true),
                    Asn = table.Column<string>(type: "text", nullable: true),
                    Decision = table.Column<string>(type: "text", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_access_log_events", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "adguard_connections",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    Name = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    BaseUrl = table.Column<string>(type: "text", nullable: false),
                    PasswordSecretId = table.Column<Guid>(type: "uuid", nullable: false),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_adguard_connections", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "adguard_rewrites",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ConnectionId = table.Column<Guid>(type: "uuid", nullable: false),
                    Domain = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false),
                    Answer = table.Column<string>(type: "text", nullable: false),
                    ManagedByHashi = table.Column<bool>(type: "boolean", nullable: false),
                    ProviderRewriteId = table.Column<string>(type: "text", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_adguard_rewrites", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "blocklist_entries",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ClientIp = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                    Reason = table.Column<string>(type: "text", nullable: false),
                    SyncedToFirewall = table.Column<bool>(type: "boolean", nullable: false),
                    CreatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_blocklist_entries", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "edge_auth_rules",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    Name = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    Priority = table.Column<int>(type: "integer", nullable: false),
                    MatchJson = table.Column<string>(type: "text", nullable: false),
                    Action = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_edge_auth_rules", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "firewall_hosts",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ConnectionId = table.Column<Guid>(type: "uuid", nullable: false),
                    Name = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    Domain = table.Column<string>(type: "text", nullable: false),
                    ManagedSubnetsJson = table.Column<string>(type: "text", nullable: false),
                    LinkedTraefikHost = table.Column<string>(type: "text", nullable: false),
                    InternalTraefikIp = table.Column<string>(type: "text", nullable: false),
                    ScriptPath = table.Column<string>(type: "text", nullable: false),
                    NetBirdDetected = table.Column<bool>(type: "boolean", nullable: false),
                    LastAppliedScriptHash = table.Column<string>(type: "text", nullable: true),
                    RollbackScript = table.Column<string>(type: "text", nullable: true),
                    LastAppliedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_firewall_hosts", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "monitor_rollups",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    MonitorEndpointId = table.Column<Guid>(type: "uuid", nullable: false),
                    BucketStartUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    SampleCount = table.Column<int>(type: "integer", nullable: false),
                    UpCount = table.Column<int>(type: "integer", nullable: false),
                    DownCount = table.Column<int>(type: "integer", nullable: false),
                    AverageLatencyMs = table.Column<double>(type: "double precision", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_monitor_rollups", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "monitor_samples",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    MonitorEndpointId = table.Column<Guid>(type: "uuid", nullable: false),
                    PartitionDate = table.Column<DateOnly>(type: "date", nullable: false),
                    CheckedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    Status = table.Column<string>(type: "text", nullable: false),
                    LatencyMs = table.Column<int>(type: "integer", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_monitor_samples", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "notification_providers",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    Name = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    Type = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    SettingsJson = table.Column<string>(type: "text", nullable: false),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_notification_providers", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "oidc_providers",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    Name = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    Issuer = table.Column<string>(type: "text", nullable: false),
                    ClientId = table.Column<string>(type: "text", nullable: false),
                    ClientSecretId = table.Column<Guid>(type: "uuid", nullable: false),
                    Scopes = table.Column<string>(type: "text", nullable: false),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_oidc_providers", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "scripts",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ConnectionId = table.Column<Guid>(type: "uuid", nullable: false),
                    Name = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    Description = table.Column<string>(type: "text", nullable: false),
                    Body = table.Column<string>(type: "text", nullable: false),
                    CronExpression = table.Column<string>(type: "text", nullable: false),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false),
                    LastRunAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    LastRunOutput = table.Column<string>(type: "text", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_scripts", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "traefik_host_states",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ConnectionId = table.Column<Guid>(type: "uuid", nullable: false),
                    StaticConfigPath = table.Column<string>(type: "text", nullable: false),
                    DynamicConfigPath = table.Column<string>(type: "text", nullable: false),
                    LastAppliedContentHash = table.Column<string>(type: "text", nullable: true),
                    LastBackupStaticYaml = table.Column<string>(type: "text", nullable: true),
                    LastBackupDynamicYaml = table.Column<string>(type: "text", nullable: true),
                    LastAppliedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_traefik_host_states", x => x.Id);
                });

            migrationBuilder.CreateIndex(
                name: "IX_abuse_buckets_ClientIp",
                table: "abuse_buckets",
                column: "ClientIp",
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_access_log_events_ReceivedAtUtc",
                table: "access_log_events",
                column: "ReceivedAtUtc");

            migrationBuilder.CreateIndex(
                name: "IX_adguard_rewrites_ConnectionId_Domain",
                table: "adguard_rewrites",
                columns: new[] { "ConnectionId", "Domain" },
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_firewall_hosts_ConnectionId_Name",
                table: "firewall_hosts",
                columns: new[] { "ConnectionId", "Name" },
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_monitor_rollups_MonitorEndpointId_BucketStartUtc",
                table: "monitor_rollups",
                columns: new[] { "MonitorEndpointId", "BucketStartUtc" },
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_monitor_samples_MonitorEndpointId_PartitionDate_CheckedAtUtc",
                table: "monitor_samples",
                columns: new[] { "MonitorEndpointId", "PartitionDate", "CheckedAtUtc" });

            migrationBuilder.CreateIndex(
                name: "IX_traefik_host_states_ConnectionId",
                table: "traefik_host_states",
                column: "ConnectionId",
                unique: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "abuse_buckets");

            migrationBuilder.DropTable(
                name: "access_log_events");

            migrationBuilder.DropTable(
                name: "adguard_connections");

            migrationBuilder.DropTable(
                name: "adguard_rewrites");

            migrationBuilder.DropTable(
                name: "blocklist_entries");

            migrationBuilder.DropTable(
                name: "edge_auth_rules");

            migrationBuilder.DropTable(
                name: "firewall_hosts");

            migrationBuilder.DropTable(
                name: "monitor_rollups");

            migrationBuilder.DropTable(
                name: "monitor_samples");

            migrationBuilder.DropTable(
                name: "notification_providers");

            migrationBuilder.DropTable(
                name: "oidc_providers");

            migrationBuilder.DropTable(
                name: "scripts");

            migrationBuilder.DropTable(
                name: "traefik_host_states");

            migrationBuilder.AlterColumn<string>(
                name: "TokenHash",
                table: "pulse_agents",
                type: "text",
                nullable: false,
                oldClrType: typeof(string),
                oldType: "character varying(128)",
                oldMaxLength: 128);
        }
    }
}
