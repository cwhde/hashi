using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddMonitorRollupIntervals : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropIndex(
                name: "IX_monitor_rollups_MonitorEndpointId_BucketStartUtc",
                table: "monitor_rollups");

            migrationBuilder.AddColumn<int>(
                name: "IntervalMinutes",
                table: "monitor_rollups",
                type: "integer",
                nullable: false,
                defaultValue: 60);

            migrationBuilder.AddColumn<Guid>(
                name: "ResourceId",
                table: "monitor_endpoints",
                type: "uuid",
                nullable: true);

            migrationBuilder.CreateIndex(
                name: "IX_monitor_rollups_MonitorEndpointId_BucketStartUtc_IntervalMi~",
                table: "monitor_rollups",
                columns: new[] { "MonitorEndpointId", "BucketStartUtc", "IntervalMinutes" },
                unique: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropIndex(
                name: "IX_monitor_rollups_MonitorEndpointId_BucketStartUtc_IntervalMi~",
                table: "monitor_rollups");

            migrationBuilder.DropColumn(
                name: "IntervalMinutes",
                table: "monitor_rollups");

            migrationBuilder.DropColumn(
                name: "ResourceId",
                table: "monitor_endpoints");

            migrationBuilder.CreateIndex(
                name: "IX_monitor_rollups_MonitorEndpointId_BucketStartUtc",
                table: "monitor_rollups",
                columns: new[] { "MonitorEndpointId", "BucketStartUtc" },
                unique: true);
        }
    }
}
