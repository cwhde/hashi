using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddAdminSessionTokenManagement : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AlterColumn<int>(
                name: "AdminSessionMinutes",
                table: "app_settings",
                type: "integer",
                nullable: false,
                defaultValue: 240,
                oldClrType: typeof(int),
                oldType: "integer");

            migrationBuilder.AddColumn<int>(
                name: "AdminSessionAbsoluteMinutes",
                table: "app_settings",
                type: "integer",
                nullable: false,
                defaultValue: 480);

            migrationBuilder.Sql(
                "UPDATE app_settings SET \"AdminSessionMinutes\" = LEAST(GREATEST(\"AdminSessionMinutes\", 5), 240);");

            migrationBuilder.CreateTable(
                name: "admin_sessions",
                columns: table => new
                {
                    Id = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                    AuthMethod = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    PasskeyCredentialId = table.Column<Guid>(type: "uuid", nullable: true),
                    BoundIp = table.Column<string>(type: "character varying(45)", maxLength: 45, nullable: false),
                    ScopesJson = table.Column<string>(type: "jsonb", nullable: false, defaultValueSql: "'[]'::jsonb"),
                    IdleTimeoutMinutes = table.Column<int>(type: "integer", nullable: false, defaultValue: 240),
                    CreatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    LastSeenAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    IdleExpiresAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    AbsoluteExpiresAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    ReauthenticatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    RevokedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    RevocationReason = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: true),
                    UserAgentHash = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_admin_sessions", x => x.Id);
                    table.ForeignKey(
                        name: "FK_admin_sessions_passkey_credentials_PasskeyCredentialId",
                        column: x => x.PasskeyCredentialId,
                        principalTable: "passkey_credentials",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.SetNull);
                });

            migrationBuilder.CreateIndex(
                name: "IX_admin_sessions_AbsoluteExpiresAtUtc",
                table: "admin_sessions",
                column: "AbsoluteExpiresAtUtc");

            migrationBuilder.CreateIndex(
                name: "IX_admin_sessions_IdleExpiresAtUtc",
                table: "admin_sessions",
                column: "IdleExpiresAtUtc");

            migrationBuilder.CreateIndex(
                name: "IX_admin_sessions_PasskeyCredentialId",
                table: "admin_sessions",
                column: "PasskeyCredentialId");

            migrationBuilder.CreateIndex(
                name: "IX_admin_sessions_RevokedAtUtc",
                table: "admin_sessions",
                column: "RevokedAtUtc");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "admin_sessions");

            migrationBuilder.DropColumn(
                name: "AdminSessionAbsoluteMinutes",
                table: "app_settings");

            migrationBuilder.AlterColumn<int>(
                name: "AdminSessionMinutes",
                table: "app_settings",
                type: "integer",
                nullable: false,
                oldClrType: typeof(int),
                oldType: "integer",
                oldDefaultValue: 240);
        }
    }
}
