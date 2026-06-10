using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class FixPendingModelChanges : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<string>(
                name: "PurposeTag",
                table: "vault_wrapped_keys",
                type: "text",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "DynamicConfigPathsJson",
                table: "traefik_host_states",
                type: "text",
                nullable: false,
                defaultValue: "");

            migrationBuilder.AddColumn<DateTimeOffset>(
                name: "FirstOffenseAtUtc",
                table: "security_subject_states",
                type: "timestamp with time zone",
                nullable: true);

            migrationBuilder.AddColumn<DateTimeOffset>(
                name: "LastOffenseAtUtc",
                table: "security_subject_states",
                type: "timestamp with time zone",
                nullable: true);

            migrationBuilder.AddColumn<int>(
                name: "RateLimitRequestCount",
                table: "security_subject_states",
                type: "integer",
                nullable: false,
                defaultValue: 0);

            migrationBuilder.AddColumn<DateTimeOffset>(
                name: "RateLimitedUntilUtc",
                table: "security_subject_states",
                type: "timestamp with time zone",
                nullable: true);

            migrationBuilder.AddColumn<int>(
                name: "TotalBlockCount",
                table: "security_subject_states",
                type: "integer",
                nullable: false,
                defaultValue: 0);

            migrationBuilder.AddColumn<int>(
                name: "TotalOffenseCount",
                table: "security_subject_states",
                type: "integer",
                nullable: false,
                defaultValue: 0);

            migrationBuilder.AddColumn<string>(
                name: "PurposeKeyTag",
                table: "secret_records",
                type: "text",
                nullable: true);

            migrationBuilder.AddColumn<byte[]>(
                name: "PurposeWrappedDekBlob",
                table: "secret_records",
                type: "bytea",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "SecretClass",
                table: "secret_records",
                type: "text",
                nullable: false,
                defaultValue: "");

            migrationBuilder.AddColumn<Guid>(
                name: "DetectedFirewallHostId",
                table: "resources",
                type: "uuid",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "CredentialIdBase64",
                table: "passkey_credentials",
                type: "text",
                nullable: false,
                defaultValue: "");

            migrationBuilder.AddColumn<int>(
                name: "CheckIntervalSeconds",
                table: "monitor_endpoints",
                type: "integer",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "Group",
                table: "monitor_endpoints",
                type: "character varying(128)",
                maxLength: 128,
                nullable: true);

            migrationBuilder.AddColumn<int>(
                name: "TimeoutSeconds",
                table: "monitor_endpoints",
                type: "integer",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "AcmeProvider",
                table: "app_settings",
                type: "text",
                nullable: false,
                defaultValue: "");

            migrationBuilder.AddColumn<int>(
                name: "AdminSessionMinutes",
                table: "app_settings",
                type: "integer",
                nullable: false,
                defaultValue: 0);

            migrationBuilder.AddColumn<string>(
                name: "InternalScheme",
                table: "app_settings",
                type: "text",
                nullable: false,
                defaultValue: "");

            migrationBuilder.CreateIndex(
                name: "IX_monitor_endpoints_Group",
                table: "monitor_endpoints",
                column: "Group");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropIndex(
                name: "IX_monitor_endpoints_Group",
                table: "monitor_endpoints");

            migrationBuilder.DropColumn(
                name: "PurposeTag",
                table: "vault_wrapped_keys");

            migrationBuilder.DropColumn(
                name: "DynamicConfigPathsJson",
                table: "traefik_host_states");

            migrationBuilder.DropColumn(
                name: "FirstOffenseAtUtc",
                table: "security_subject_states");

            migrationBuilder.DropColumn(
                name: "LastOffenseAtUtc",
                table: "security_subject_states");

            migrationBuilder.DropColumn(
                name: "RateLimitRequestCount",
                table: "security_subject_states");

            migrationBuilder.DropColumn(
                name: "RateLimitedUntilUtc",
                table: "security_subject_states");

            migrationBuilder.DropColumn(
                name: "TotalBlockCount",
                table: "security_subject_states");

            migrationBuilder.DropColumn(
                name: "TotalOffenseCount",
                table: "security_subject_states");

            migrationBuilder.DropColumn(
                name: "PurposeKeyTag",
                table: "secret_records");

            migrationBuilder.DropColumn(
                name: "PurposeWrappedDekBlob",
                table: "secret_records");

            migrationBuilder.DropColumn(
                name: "SecretClass",
                table: "secret_records");

            migrationBuilder.DropColumn(
                name: "DetectedFirewallHostId",
                table: "resources");

            migrationBuilder.DropColumn(
                name: "CredentialIdBase64",
                table: "passkey_credentials");

            migrationBuilder.DropColumn(
                name: "CheckIntervalSeconds",
                table: "monitor_endpoints");

            migrationBuilder.DropColumn(
                name: "Group",
                table: "monitor_endpoints");

            migrationBuilder.DropColumn(
                name: "TimeoutSeconds",
                table: "monitor_endpoints");

            migrationBuilder.DropColumn(
                name: "AcmeProvider",
                table: "app_settings");

            migrationBuilder.DropColumn(
                name: "AdminSessionMinutes",
                table: "app_settings");

            migrationBuilder.DropColumn(
                name: "InternalScheme",
                table: "app_settings");
        }
    }
}
