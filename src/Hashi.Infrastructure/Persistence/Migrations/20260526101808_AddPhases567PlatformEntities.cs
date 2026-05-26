using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddPhases567PlatformEntities : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<string>(
                name: "PendingAcmeEabJson",
                table: "setup_state",
                type: "text",
                nullable: true);

            migrationBuilder.AlterColumn<string>(
                name: "WafMode",
                table: "resources",
                type: "character varying(32)",
                maxLength: 32,
                nullable: false,
                oldClrType: typeof(string),
                oldType: "text");

            migrationBuilder.AlterColumn<string>(
                name: "ForwardAuthPolicy",
                table: "resources",
                type: "character varying(32)",
                maxLength: 32,
                nullable: false,
                oldClrType: typeof(string),
                oldType: "text");

            migrationBuilder.AddColumn<int>(
                name: "PublicPort",
                table: "resources",
                type: "integer",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "LxcBridge",
                table: "firewall_hosts",
                type: "text",
                nullable: true);

            migrationBuilder.AddColumn<bool>(
                name: "NetBirdEnabled",
                table: "firewall_hosts",
                type: "boolean",
                nullable: false,
                defaultValue: false);

            migrationBuilder.AddColumn<string>(
                name: "NetBirdInterface",
                table: "firewall_hosts",
                type: "text",
                nullable: false,
                defaultValue: "");

            migrationBuilder.AddColumn<string>(
                name: "NetBirdOverlayCidrsJson",
                table: "firewall_hosts",
                type: "text",
                nullable: false,
                defaultValue: "");

            migrationBuilder.AddColumn<string>(
                name: "NetBirdRoutedCidrsJson",
                table: "firewall_hosts",
                type: "text",
                nullable: false,
                defaultValue: "");

            migrationBuilder.AddColumn<bool>(
                name: "NetBirdRoutingPeer",
                table: "firewall_hosts",
                type: "boolean",
                nullable: false,
                defaultValue: false);

            migrationBuilder.AddColumn<int>(
                name: "RollbackTimerSeconds",
                table: "firewall_hosts",
                type: "integer",
                nullable: false,
                defaultValue: 0);

            migrationBuilder.AddColumn<string>(
                name: "WanInterface",
                table: "firewall_hosts",
                type: "text",
                nullable: true);

            migrationBuilder.AddColumn<Guid>(
                name: "AcmeEabSecretId",
                table: "app_settings",
                type: "uuid",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "AcmeEmail",
                table: "app_settings",
                type: "character varying(256)",
                maxLength: 256,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "AcmeResolversJson",
                table: "app_settings",
                type: "text",
                nullable: false,
                defaultValue: "");

            migrationBuilder.AddColumn<int>(
                name: "DnsChallengeDelaySeconds",
                table: "app_settings",
                type: "integer",
                nullable: false,
                defaultValue: 0);

            migrationBuilder.CreateTable(
                name: "resource_routes",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ResourceId = table.Column<Guid>(type: "uuid", nullable: false),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false),
                    Priority = table.Column<int>(type: "integer", nullable: false),
                    PathMatchType = table.Column<string>(type: "character varying(16)", maxLength: 16, nullable: false),
                    PathValue = table.Column<string>(type: "text", nullable: false),
                    TargetScheme = table.Column<string>(type: "text", nullable: false),
                    TargetHost = table.Column<string>(type: "text", nullable: false),
                    TargetPort = table.Column<int>(type: "integer", nullable: false),
                    RewriteMode = table.Column<string>(type: "text", nullable: true),
                    RewriteValue = table.Column<string>(type: "text", nullable: true),
                    ExtraMiddlewaresJson = table.Column<string>(type: "text", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_resource_routes", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "resource_rules",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ResourceId = table.Column<Guid>(type: "uuid", nullable: false),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false),
                    Priority = table.Column<int>(type: "integer", nullable: false),
                    Action = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    MatchType = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    MatchValue = table.Column<string>(type: "text", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_resource_rules", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "traefik_entrypoints",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    Port = table.Column<int>(type: "integer", nullable: false),
                    Protocol = table.Column<string>(type: "character varying(8)", maxLength: 8, nullable: false),
                    ResourceId = table.Column<Guid>(type: "uuid", nullable: true),
                    Label = table.Column<string>(type: "text", nullable: true),
                    Confirmed = table.Column<bool>(type: "boolean", nullable: false),
                    ConfirmedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    CreatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_traefik_entrypoints", x => x.Id);
                });

            migrationBuilder.CreateIndex(
                name: "IX_resource_routes_ResourceId",
                table: "resource_routes",
                column: "ResourceId");

            migrationBuilder.CreateIndex(
                name: "IX_resource_rules_ResourceId",
                table: "resource_rules",
                column: "ResourceId");

            migrationBuilder.CreateIndex(
                name: "IX_traefik_entrypoints_Port_Protocol",
                table: "traefik_entrypoints",
                columns: new[] { "Port", "Protocol" },
                unique: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "resource_routes");

            migrationBuilder.DropTable(
                name: "resource_rules");

            migrationBuilder.DropTable(
                name: "traefik_entrypoints");

            migrationBuilder.DropColumn(
                name: "PendingAcmeEabJson",
                table: "setup_state");

            migrationBuilder.DropColumn(
                name: "PublicPort",
                table: "resources");

            migrationBuilder.DropColumn(
                name: "LxcBridge",
                table: "firewall_hosts");

            migrationBuilder.DropColumn(
                name: "NetBirdEnabled",
                table: "firewall_hosts");

            migrationBuilder.DropColumn(
                name: "NetBirdInterface",
                table: "firewall_hosts");

            migrationBuilder.DropColumn(
                name: "NetBirdOverlayCidrsJson",
                table: "firewall_hosts");

            migrationBuilder.DropColumn(
                name: "NetBirdRoutedCidrsJson",
                table: "firewall_hosts");

            migrationBuilder.DropColumn(
                name: "NetBirdRoutingPeer",
                table: "firewall_hosts");

            migrationBuilder.DropColumn(
                name: "RollbackTimerSeconds",
                table: "firewall_hosts");

            migrationBuilder.DropColumn(
                name: "WanInterface",
                table: "firewall_hosts");

            migrationBuilder.DropColumn(
                name: "AcmeEabSecretId",
                table: "app_settings");

            migrationBuilder.DropColumn(
                name: "AcmeEmail",
                table: "app_settings");

            migrationBuilder.DropColumn(
                name: "AcmeResolversJson",
                table: "app_settings");

            migrationBuilder.DropColumn(
                name: "DnsChallengeDelaySeconds",
                table: "app_settings");

            migrationBuilder.AlterColumn<string>(
                name: "WafMode",
                table: "resources",
                type: "text",
                nullable: false,
                oldClrType: typeof(string),
                oldType: "character varying(32)",
                oldMaxLength: 32);

            migrationBuilder.AlterColumn<string>(
                name: "ForwardAuthPolicy",
                table: "resources",
                type: "text",
                nullable: false,
                oldClrType: typeof(string),
                oldType: "character varying(32)",
                oldMaxLength: 32);
        }
    }
}
