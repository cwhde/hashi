using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations;

[DbContext(typeof(HashiDbContext))]
[Migration("20260602103524_AddGeoIpSettingsAndDatabases")]
public partial class AddGeoIpSettingsAndDatabases : Migration
{
    protected override void Up(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.AddColumn<string>(
            name: "GeoIpAccountId",
            table: "app_settings",
            type: "character varying(128)",
            maxLength: 128,
            nullable: true);

        migrationBuilder.AddColumn<bool>(
            name: "GeoIpEnabled",
            table: "app_settings",
            type: "boolean",
            nullable: false,
            defaultValue: false);

        migrationBuilder.AddColumn<DateTimeOffset>(
            name: "GeoIpLastUpdateAtUtc",
            table: "app_settings",
            type: "timestamp with time zone",
            nullable: true);

        migrationBuilder.AddColumn<string>(
            name: "GeoIpLastUpdateMessage",
            table: "app_settings",
            type: "text",
            nullable: true);

        migrationBuilder.AddColumn<string>(
            name: "GeoIpLastUpdateStatus",
            table: "app_settings",
            type: "character varying(32)",
            maxLength: 32,
            nullable: false,
            defaultValue: "never_run");

        migrationBuilder.AddColumn<Guid>(
            name: "GeoIpLicenseKeySecretId",
            table: "app_settings",
            type: "uuid",
            nullable: true);

        migrationBuilder.AddColumn<DateTimeOffset>(
            name: "GeoIpNextUpdateAtUtc",
            table: "app_settings",
            type: "timestamp with time zone",
            nullable: true);

        migrationBuilder.AddColumn<int>(
            name: "GeoIpUpdateIntervalHours",
            table: "app_settings",
            type: "integer",
            nullable: false,
            defaultValue: 72);

        migrationBuilder.CreateTable(
            name: "geoip_databases",
            columns: table => new
            {
                Id = table.Column<Guid>(type: "uuid", nullable: false),
                EditionId = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                FileName = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                Path = table.Column<string>(type: "character varying(512)", maxLength: 512, nullable: false),
                Status = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false, defaultValue: "never_run"),
                LastDownloadedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                LastModifiedUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                SizeBytes = table.Column<long>(type: "bigint", nullable: true),
                ContentHash = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: true),
                Error = table.Column<string>(type: "text", nullable: true),
                UpdatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
            },
            constraints: table =>
            {
                table.PrimaryKey("PK_geoip_databases", x => x.Id);
            });

        migrationBuilder.CreateIndex(
            name: "IX_geoip_databases_EditionId",
            table: "geoip_databases",
            column: "EditionId",
            unique: true);
    }

    protected override void Down(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.DropTable(name: "geoip_databases");

        migrationBuilder.DropColumn(
            name: "GeoIpAccountId",
            table: "app_settings");

        migrationBuilder.DropColumn(
            name: "GeoIpEnabled",
            table: "app_settings");

        migrationBuilder.DropColumn(
            name: "GeoIpLastUpdateAtUtc",
            table: "app_settings");

        migrationBuilder.DropColumn(
            name: "GeoIpLastUpdateMessage",
            table: "app_settings");

        migrationBuilder.DropColumn(
            name: "GeoIpLastUpdateStatus",
            table: "app_settings");

        migrationBuilder.DropColumn(
            name: "GeoIpLicenseKeySecretId",
            table: "app_settings");

        migrationBuilder.DropColumn(
            name: "GeoIpNextUpdateAtUtc",
            table: "app_settings");

        migrationBuilder.DropColumn(
            name: "GeoIpUpdateIntervalHours",
            table: "app_settings");
    }
}
