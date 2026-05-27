using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddOperationalDataModelGaps : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<string>(
                name: "DeletionPolicy",
                table: "resources",
                type: "character varying(32)",
                maxLength: 32,
                nullable: false,
                defaultValue: "optional");

            migrationBuilder.AddColumn<DateTimeOffset>(
                name: "LastAppliedAtUtc",
                table: "resources",
                type: "timestamp with time zone",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "LastAppliedHash",
                table: "resources",
                type: "text",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "Ownership",
                table: "resources",
                type: "character varying(32)",
                maxLength: 32,
                nullable: false,
                defaultValue: "user_created");

            migrationBuilder.AddColumn<string>(
                name: "OwningWorkflow",
                table: "resources",
                type: "character varying(64)",
                maxLength: 64,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "SyncState",
                table: "resources",
                type: "character varying(32)",
                maxLength: 32,
                nullable: false,
                defaultValue: "desired");

            migrationBuilder.CreateTable(
                name: "connection_health",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ConnectionId = table.Column<Guid>(type: "uuid", nullable: false),
                    State = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    CheckKind = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                    LatencyMs = table.Column<int>(type: "integer", nullable: true),
                    Message = table.Column<string>(type: "text", nullable: true),
                    DetailsJson = table.Column<string>(type: "text", nullable: true),
                    CheckedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_connection_health", x => x.Id);
                    table.ForeignKey(
                        name: "FK_connection_health_connections_ConnectionId",
                        column: x => x.ConnectionId,
                        principalTable: "connections",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateTable(
                name: "dns_record_ownership",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ZoneId = table.Column<Guid>(type: "uuid", nullable: false),
                    DnsRecordId = table.Column<Guid>(type: "uuid", nullable: true),
                    ResourceId = table.Column<Guid>(type: "uuid", nullable: true),
                    ProviderRecordId = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    Name = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false),
                    Type = table.Column<string>(type: "character varying(16)", maxLength: 16, nullable: false),
                    Value = table.Column<string>(type: "text", nullable: false),
                    Ownership = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    OwnerWorkflow = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                    SyncState = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    DesiredContentHash = table.Column<string>(type: "text", nullable: true),
                    AppliedContentHash = table.Column<string>(type: "text", nullable: true),
                    LastObservedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    LastAppliedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    CreatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_dns_record_ownership", x => x.Id);
                    table.ForeignKey(
                        name: "FK_dns_record_ownership_dns_records_DnsRecordId",
                        column: x => x.DnsRecordId,
                        principalTable: "dns_records",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.SetNull);
                    table.ForeignKey(
                        name: "FK_dns_record_ownership_dns_zones_ZoneId",
                        column: x => x.ZoneId,
                        principalTable: "dns_zones",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                    table.ForeignKey(
                        name: "FK_dns_record_ownership_resources_ResourceId",
                        column: x => x.ResourceId,
                        principalTable: "resources",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.SetNull);
                });

            migrationBuilder.CreateTable(
                name: "firewall_allowed_subjects",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    FirewallHostId = table.Column<Guid>(type: "uuid", nullable: false),
                    SubjectKind = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    SubjectValue = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false),
                    Reason = table.Column<string>(type: "text", nullable: false),
                    Ownership = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_firewall_allowed_subjects", x => x.Id);
                    table.ForeignKey(
                        name: "FK_firewall_allowed_subjects_firewall_hosts_FirewallHostId",
                        column: x => x.FirewallHostId,
                        principalTable: "firewall_hosts",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateTable(
                name: "firewall_block_subjects",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    FirewallHostId = table.Column<Guid>(type: "uuid", nullable: false),
                    BlocklistEntryId = table.Column<Guid>(type: "uuid", nullable: true),
                    SubjectKind = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    SubjectValue = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false),
                    Reason = table.Column<string>(type: "text", nullable: false),
                    Ownership = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_firewall_block_subjects", x => x.Id);
                    table.ForeignKey(
                        name: "FK_firewall_block_subjects_blocklist_entries_BlocklistEntryId",
                        column: x => x.BlocklistEntryId,
                        principalTable: "blocklist_entries",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.SetNull);
                    table.ForeignKey(
                        name: "FK_firewall_block_subjects_firewall_hosts_FirewallHostId",
                        column: x => x.FirewallHostId,
                        principalTable: "firewall_hosts",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateTable(
                name: "firewall_generated_scripts",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    FirewallHostId = table.Column<Guid>(type: "uuid", nullable: false),
                    SyncRunId = table.Column<Guid>(type: "uuid", nullable: true),
                    ScriptPath = table.Column<string>(type: "character varying(512)", maxLength: 512, nullable: false),
                    DesiredContentHash = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    AppliedContentHash = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: true),
                    DesiredScript = table.Column<string>(type: "text", nullable: false),
                    AppliedScript = table.Column<string>(type: "text", nullable: true),
                    Status = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    DiffSummary = table.Column<string>(type: "text", nullable: true),
                    ErrorDetails = table.Column<string>(type: "text", nullable: true),
                    CreatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    AppliedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_firewall_generated_scripts", x => x.Id);
                    table.ForeignKey(
                        name: "FK_firewall_generated_scripts_firewall_hosts_FirewallHostId",
                        column: x => x.FirewallHostId,
                        principalTable: "firewall_hosts",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                    table.ForeignKey(
                        name: "FK_firewall_generated_scripts_sync_runs_SyncRunId",
                        column: x => x.SyncRunId,
                        principalTable: "sync_runs",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.SetNull);
                });

            migrationBuilder.CreateTable(
                name: "firewall_ports",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    FirewallHostId = table.Column<Guid>(type: "uuid", nullable: false),
                    ResourceId = table.Column<Guid>(type: "uuid", nullable: true),
                    PublicPort = table.Column<int>(type: "integer", nullable: false),
                    TargetPort = table.Column<int>(type: "integer", nullable: false),
                    Protocol = table.Column<string>(type: "character varying(8)", maxLength: 8, nullable: false),
                    TargetHost = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false),
                    Ownership = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    Confirmed = table.Column<bool>(type: "boolean", nullable: false),
                    ConfirmedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_firewall_ports", x => x.Id);
                    table.ForeignKey(
                        name: "FK_firewall_ports_firewall_hosts_FirewallHostId",
                        column: x => x.FirewallHostId,
                        principalTable: "firewall_hosts",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                    table.ForeignKey(
                        name: "FK_firewall_ports_resources_ResourceId",
                        column: x => x.ResourceId,
                        principalTable: "resources",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.SetNull);
                });

            migrationBuilder.CreateTable(
                name: "firewall_subnets",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    FirewallHostId = table.Column<Guid>(type: "uuid", nullable: false),
                    Cidr = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    Purpose = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    Ownership = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_firewall_subnets", x => x.Id);
                    table.ForeignKey(
                        name: "FK_firewall_subnets_firewall_hosts_FirewallHostId",
                        column: x => x.FirewallHostId,
                        principalTable: "firewall_hosts",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateTable(
                name: "notification_routes",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ProviderId = table.Column<Guid>(type: "uuid", nullable: false),
                    Name = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    EventKind = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                    Severity = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    MatchJson = table.Column<string>(type: "text", nullable: false),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false),
                    CreatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_notification_routes", x => x.Id);
                    table.ForeignKey(
                        name: "FK_notification_routes_notification_providers_ProviderId",
                        column: x => x.ProviderId,
                        principalTable: "notification_providers",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateTable(
                name: "resource_ports",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ResourceId = table.Column<Guid>(type: "uuid", nullable: false),
                    PublicPort = table.Column<int>(type: "integer", nullable: false),
                    TargetPort = table.Column<int>(type: "integer", nullable: false),
                    Protocol = table.Column<string>(type: "character varying(8)", maxLength: 8, nullable: false),
                    Ownership = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    Confirmed = table.Column<bool>(type: "boolean", nullable: false),
                    ConfirmedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_resource_ports", x => x.Id);
                    table.ForeignKey(
                        name: "FK_resource_ports_resources_ResourceId",
                        column: x => x.ResourceId,
                        principalTable: "resources",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateTable(
                name: "resource_targets",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ResourceId = table.Column<Guid>(type: "uuid", nullable: false),
                    Priority = table.Column<int>(type: "integer", nullable: false),
                    Scheme = table.Column<string>(type: "character varying(16)", maxLength: 16, nullable: false),
                    Host = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false),
                    Port = table.Column<int>(type: "integer", nullable: false),
                    FirewallHostId = table.Column<Guid>(type: "uuid", nullable: true),
                    PulseAgentId = table.Column<Guid>(type: "uuid", nullable: true),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_resource_targets", x => x.Id);
                    table.ForeignKey(
                        name: "FK_resource_targets_firewall_hosts_FirewallHostId",
                        column: x => x.FirewallHostId,
                        principalTable: "firewall_hosts",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.SetNull);
                    table.ForeignKey(
                        name: "FK_resource_targets_resources_ResourceId",
                        column: x => x.ResourceId,
                        principalTable: "resources",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateTable(
                name: "system_resources",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ResourceId = table.Column<Guid>(type: "uuid", nullable: false),
                    SystemKey = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    OwningWorkflow = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                    RequiredForAppAccess = table.Column<bool>(type: "boolean", nullable: false),
                    CreatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_system_resources", x => x.Id);
                    table.ForeignKey(
                        name: "FK_system_resources_resources_ResourceId",
                        column: x => x.ResourceId,
                        principalTable: "resources",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateTable(
                name: "notification_deliveries",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    RouteId = table.Column<Guid>(type: "uuid", nullable: true),
                    ProviderId = table.Column<Guid>(type: "uuid", nullable: false),
                    EventKind = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                    Subject = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false),
                    Status = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    AttemptCount = table.Column<int>(type: "integer", nullable: false),
                    ErrorDetails = table.Column<string>(type: "text", nullable: true),
                    ProviderMessageId = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: true),
                    CreatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    SentAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_notification_deliveries", x => x.Id);
                    table.ForeignKey(
                        name: "FK_notification_deliveries_notification_providers_ProviderId",
                        column: x => x.ProviderId,
                        principalTable: "notification_providers",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                    table.ForeignKey(
                        name: "FK_notification_deliveries_notification_routes_RouteId",
                        column: x => x.RouteId,
                        principalTable: "notification_routes",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.SetNull);
                });

            migrationBuilder.CreateIndex(
                name: "IX_connection_health_ConnectionId_CheckedAtUtc",
                table: "connection_health",
                columns: new[] { "ConnectionId", "CheckedAtUtc" });

            migrationBuilder.CreateIndex(
                name: "IX_dns_record_ownership_DnsRecordId",
                table: "dns_record_ownership",
                column: "DnsRecordId");

            migrationBuilder.CreateIndex(
                name: "IX_dns_record_ownership_ProviderRecordId",
                table: "dns_record_ownership",
                column: "ProviderRecordId");

            migrationBuilder.CreateIndex(
                name: "IX_dns_record_ownership_ResourceId",
                table: "dns_record_ownership",
                column: "ResourceId");

            migrationBuilder.CreateIndex(
                name: "IX_dns_record_ownership_ZoneId_Name_Type_Value",
                table: "dns_record_ownership",
                columns: new[] { "ZoneId", "Name", "Type", "Value" },
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_firewall_allowed_subjects_FirewallHostId_SubjectKind_Subjec~",
                table: "firewall_allowed_subjects",
                columns: new[] { "FirewallHostId", "SubjectKind", "SubjectValue" },
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_firewall_block_subjects_BlocklistEntryId",
                table: "firewall_block_subjects",
                column: "BlocklistEntryId");

            migrationBuilder.CreateIndex(
                name: "IX_firewall_block_subjects_FirewallHostId_SubjectKind_SubjectV~",
                table: "firewall_block_subjects",
                columns: new[] { "FirewallHostId", "SubjectKind", "SubjectValue" },
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_firewall_generated_scripts_FirewallHostId_CreatedAtUtc",
                table: "firewall_generated_scripts",
                columns: new[] { "FirewallHostId", "CreatedAtUtc" });

            migrationBuilder.CreateIndex(
                name: "IX_firewall_generated_scripts_SyncRunId",
                table: "firewall_generated_scripts",
                column: "SyncRunId");

            migrationBuilder.CreateIndex(
                name: "IX_firewall_ports_FirewallHostId_PublicPort_Protocol",
                table: "firewall_ports",
                columns: new[] { "FirewallHostId", "PublicPort", "Protocol" },
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_firewall_ports_ResourceId",
                table: "firewall_ports",
                column: "ResourceId");

            migrationBuilder.CreateIndex(
                name: "IX_firewall_subnets_FirewallHostId_Cidr_Purpose",
                table: "firewall_subnets",
                columns: new[] { "FirewallHostId", "Cidr", "Purpose" },
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_notification_deliveries_ProviderId_CreatedAtUtc",
                table: "notification_deliveries",
                columns: new[] { "ProviderId", "CreatedAtUtc" });

            migrationBuilder.CreateIndex(
                name: "IX_notification_deliveries_RouteId_CreatedAtUtc",
                table: "notification_deliveries",
                columns: new[] { "RouteId", "CreatedAtUtc" });

            migrationBuilder.CreateIndex(
                name: "IX_notification_routes_ProviderId_EventKind",
                table: "notification_routes",
                columns: new[] { "ProviderId", "EventKind" });

            migrationBuilder.CreateIndex(
                name: "IX_resource_ports_PublicPort_Protocol",
                table: "resource_ports",
                columns: new[] { "PublicPort", "Protocol" },
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_resource_ports_ResourceId",
                table: "resource_ports",
                column: "ResourceId");

            migrationBuilder.CreateIndex(
                name: "IX_resource_targets_FirewallHostId",
                table: "resource_targets",
                column: "FirewallHostId");

            migrationBuilder.CreateIndex(
                name: "IX_resource_targets_ResourceId_Priority",
                table: "resource_targets",
                columns: new[] { "ResourceId", "Priority" });

            migrationBuilder.CreateIndex(
                name: "IX_system_resources_ResourceId",
                table: "system_resources",
                column: "ResourceId",
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_system_resources_SystemKey",
                table: "system_resources",
                column: "SystemKey",
                unique: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "connection_health");

            migrationBuilder.DropTable(
                name: "dns_record_ownership");

            migrationBuilder.DropTable(
                name: "firewall_allowed_subjects");

            migrationBuilder.DropTable(
                name: "firewall_block_subjects");

            migrationBuilder.DropTable(
                name: "firewall_generated_scripts");

            migrationBuilder.DropTable(
                name: "firewall_ports");

            migrationBuilder.DropTable(
                name: "firewall_subnets");

            migrationBuilder.DropTable(
                name: "notification_deliveries");

            migrationBuilder.DropTable(
                name: "resource_ports");

            migrationBuilder.DropTable(
                name: "resource_targets");

            migrationBuilder.DropTable(
                name: "system_resources");

            migrationBuilder.DropTable(
                name: "notification_routes");

            migrationBuilder.DropColumn(
                name: "DeletionPolicy",
                table: "resources");

            migrationBuilder.DropColumn(
                name: "LastAppliedAtUtc",
                table: "resources");

            migrationBuilder.DropColumn(
                name: "LastAppliedHash",
                table: "resources");

            migrationBuilder.DropColumn(
                name: "Ownership",
                table: "resources");

            migrationBuilder.DropColumn(
                name: "OwningWorkflow",
                table: "resources");

            migrationBuilder.DropColumn(
                name: "SyncState",
                table: "resources");
        }
    }
}
