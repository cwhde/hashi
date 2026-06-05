using System;
using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    [DbContext(typeof(HashiDbContext))]
    [Migration("20260605120000_AddBlocklistLifecycleAndJobFields")]
    public partial class AddBlocklistLifecycleAndJobFields : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<DateTimeOffset>(
                name: "FirstSeenAtUtc",
                table: "blocklist_entries",
                type: "timestamp with time zone",
                nullable: false,
                defaultValueSql: "now()");

            migrationBuilder.AddColumn<DateTimeOffset>(
                name: "LastSeenAtUtc",
                table: "blocklist_entries",
                type: "timestamp with time zone",
                nullable: false,
                defaultValueSql: "now()");

            migrationBuilder.AddColumn<int>(
                name: "RejectedCount",
                table: "blocklist_fetch_runs",
                type: "integer",
                nullable: false,
                defaultValue: 0);

            migrationBuilder.AddColumn<int>(
                name: "EntryCount",
                table: "blocklist_sources",
                type: "integer",
                nullable: false,
                defaultValue: 0);

            migrationBuilder.AddColumn<int?>(
                name: "LastHttpStatusCode",
                table: "blocklist_sources",
                type: "integer",
                nullable: true);

            migrationBuilder.AddColumn<DateTimeOffset>(
                name: "LastSuccessAtUtc",
                table: "blocklist_sources",
                type: "timestamp with time zone",
                nullable: true);

            migrationBuilder.AddColumn<int>(
                name: "RejectedCount",
                table: "blocklist_sources",
                type: "integer",
                nullable: false,
                defaultValue: 0);

            migrationBuilder.CreateIndex(
                name: "IX_blocklist_entries_LastSeenAtUtc",
                table: "blocklist_entries",
                column: "LastSeenAtUtc");

            migrationBuilder.CreateIndex(
                name: "IX_blocklist_sources_LastSuccessAtUtc",
                table: "blocklist_sources",
                column: "LastSuccessAtUtc");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropIndex(
                name: "IX_blocklist_entries_LastSeenAtUtc",
                table: "blocklist_entries");

            migrationBuilder.DropIndex(
                name: "IX_blocklist_sources_LastSuccessAtUtc",
                table: "blocklist_sources");

            migrationBuilder.DropColumn(
                name: "FirstSeenAtUtc",
                table: "blocklist_entries");

            migrationBuilder.DropColumn(
                name: "LastSeenAtUtc",
                table: "blocklist_entries");

            migrationBuilder.DropColumn(
                name: "RejectedCount",
                table: "blocklist_fetch_runs");

            migrationBuilder.DropColumn(
                name: "EntryCount",
                table: "blocklist_sources");

            migrationBuilder.DropColumn(
                name: "LastHttpStatusCode",
                table: "blocklist_sources");

            migrationBuilder.DropColumn(
                name: "LastSuccessAtUtc",
                table: "blocklist_sources");

            migrationBuilder.DropColumn(
                name: "RejectedCount",
                table: "blocklist_sources");
        }
    }
}
