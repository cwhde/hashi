using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations;

[DbContext(typeof(HashiDbContext))]
[Migration("20260604000000_AddManualDnsMonitoringSelection")]
public partial class AddManualDnsMonitoringSelection : Migration
{
    protected override void Up(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.AddColumn<bool>(
            name: "MonitoringEnabled",
            table: "dns_records",
            type: "boolean",
            nullable: false,
            defaultValue: false);

        migrationBuilder.AddColumn<string>(
            name: "MonitoringDisplayName",
            table: "dns_records",
            type: "character varying(128)",
            maxLength: 128,
            nullable: true);

        migrationBuilder.AddColumn<Guid>(
            name: "DnsRecordId",
            table: "monitor_endpoints",
            type: "uuid",
            nullable: true);

        migrationBuilder.CreateIndex(
            name: "IX_monitor_endpoints_DnsRecordId",
            table: "monitor_endpoints",
            column: "DnsRecordId");
    }

    protected override void Down(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.DropColumn(
            name: "MonitoringEnabled",
            table: "dns_records");

        migrationBuilder.DropColumn(
            name: "MonitoringDisplayName",
            table: "dns_records");

        migrationBuilder.DropIndex(
            name: "IX_monitor_endpoints_DnsRecordId",
            table: "monitor_endpoints");

        migrationBuilder.DropColumn(
            name: "DnsRecordId",
            table: "monitor_endpoints");
    }
}
