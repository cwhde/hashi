using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations;

[DbContext(typeof(HashiDbContext))]
[Migration("20260602130000_AddBlocklistStateAndForwardAuthContext")]
public partial class AddBlocklistStateAndForwardAuthContext : Migration
{
    protected override void Up(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.AddColumn<string>(
            name: "CreatedBy",
            table: "blocklist_entries",
            type: "character varying(128)",
            maxLength: 128,
            nullable: false,
            defaultValue: "hashi");

        migrationBuilder.AddColumn<DateTimeOffset>(
            name: "ExpiresAtUtc",
            table: "blocklist_entries",
            type: "timestamp with time zone",
            nullable: true);

        migrationBuilder.AddColumn<DateTimeOffset>(
            name: "LastHitAtUtc",
            table: "blocklist_entries",
            type: "timestamp with time zone",
            nullable: true);

        migrationBuilder.AddColumn<string>(
            name: "Scope",
            table: "blocklist_entries",
            type: "character varying(32)",
            maxLength: 32,
            nullable: false,
            defaultValue: "global");

        migrationBuilder.AddColumn<string>(
            name: "Source",
            table: "blocklist_entries",
            type: "character varying(64)",
            maxLength: 64,
            nullable: false,
            defaultValue: "automatic");

        migrationBuilder.AddColumn<string>(
            name: "Type",
            table: "blocklist_entries",
            type: "character varying(32)",
            maxLength: 32,
            nullable: false,
            defaultValue: "ip");

        migrationBuilder.AddColumn<string>(
            name: "Value",
            table: "blocklist_entries",
            type: "character varying(128)",
            maxLength: 128,
            nullable: false,
            defaultValue: "");

        migrationBuilder.Sql("""
            UPDATE blocklist_entries
            SET "Value" = "ClientIp"
            WHERE "Value" = '' AND "ClientIp" <> '';
            """);

        migrationBuilder.AlterColumn<string>(
            name: "Reason",
            table: "blocklist_entries",
            type: "character varying(256)",
            maxLength: 256,
            nullable: false,
            oldClrType: typeof(string),
            oldType: "text");

        migrationBuilder.CreateTable(
            name: "blocklist_applied_hosts",
            columns: table => new
            {
                Id = table.Column<Guid>(type: "uuid", nullable: false),
                BlocklistEntryId = table.Column<Guid>(type: "uuid", nullable: false),
                FirewallHostId = table.Column<Guid>(type: "uuid", nullable: false),
                Status = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                AppliedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                LastError = table.Column<string>(type: "text", nullable: true),
            },
            constraints: table =>
            {
                table.PrimaryKey("PK_blocklist_applied_hosts", x => x.Id);
                table.ForeignKey(
                    name: "FK_blocklist_applied_hosts_blocklist_entries_BlocklistEntryId",
                    column: x => x.BlocklistEntryId,
                    principalTable: "blocklist_entries",
                    principalColumn: "Id",
                    onDelete: ReferentialAction.Cascade);
                table.ForeignKey(
                    name: "FK_blocklist_applied_hosts_firewall_hosts_FirewallHostId",
                    column: x => x.FirewallHostId,
                    principalTable: "firewall_hosts",
                    principalColumn: "Id",
                    onDelete: ReferentialAction.Cascade);
            });

        migrationBuilder.CreateIndex(
            name: "IX_blocklist_entries_ExpiresAtUtc",
            table: "blocklist_entries",
            column: "ExpiresAtUtc");

        migrationBuilder.CreateIndex(
            name: "IX_blocklist_entries_Scope_Type_Value",
            table: "blocklist_entries",
            columns: new[] { "Scope", "Type", "Value" });

        migrationBuilder.CreateIndex(
            name: "IX_blocklist_applied_hosts_BlocklistEntryId_FirewallHostId",
            table: "blocklist_applied_hosts",
            columns: new[] { "BlocklistEntryId", "FirewallHostId" },
            unique: true);

        migrationBuilder.CreateIndex(
            name: "IX_blocklist_applied_hosts_FirewallHostId",
            table: "blocklist_applied_hosts",
            column: "FirewallHostId");
    }

    protected override void Down(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.DropTable(name: "blocklist_applied_hosts");

        migrationBuilder.DropIndex(
            name: "IX_blocklist_entries_ExpiresAtUtc",
            table: "blocklist_entries");

        migrationBuilder.DropIndex(
            name: "IX_blocklist_entries_Scope_Type_Value",
            table: "blocklist_entries");

        migrationBuilder.AlterColumn<string>(
            name: "Reason",
            table: "blocklist_entries",
            type: "text",
            nullable: false,
            oldClrType: typeof(string),
            oldType: "character varying(256)",
            oldMaxLength: 256);

        migrationBuilder.DropColumn(
            name: "CreatedBy",
            table: "blocklist_entries");

        migrationBuilder.DropColumn(
            name: "ExpiresAtUtc",
            table: "blocklist_entries");

        migrationBuilder.DropColumn(
            name: "LastHitAtUtc",
            table: "blocklist_entries");

        migrationBuilder.DropColumn(
            name: "Scope",
            table: "blocklist_entries");

        migrationBuilder.DropColumn(
            name: "Source",
            table: "blocklist_entries");

        migrationBuilder.DropColumn(
            name: "Type",
            table: "blocklist_entries");

        migrationBuilder.DropColumn(
            name: "Value",
            table: "blocklist_entries");
    }
}
