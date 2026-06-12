using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddResourceSecurityProfiles : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<bool>(
                name: "AdGuardRewriteEnabled",
                table: "resources",
                type: "boolean",
                nullable: false,
                defaultValue: true);

            migrationBuilder.AddColumn<string>(
                name: "ExplicitRoutingOverride",
                table: "resources",
                type: "character varying(256)",
                maxLength: 256,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "SecurityProfileName",
                table: "resources",
                type: "character varying(128)",
                maxLength: 128,
                nullable: true);

            migrationBuilder.CreateTable(
                name: "security_profiles",
                columns: table => new
                {
                    Name = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    ForwardAuthPolicy = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false, defaultValue: "adaptive"),
                    WafMode = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false, defaultValue: "detect_only"),
                    RateLimitAverage = table.Column<int>(type: "integer", nullable: false, defaultValue: 100),
                    RateLimitBurst = table.Column<int>(type: "integer", nullable: false, defaultValue: 200)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_security_profiles", x => x.Name);
                });
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "security_profiles");

            migrationBuilder.DropColumn(
                name: "AdGuardRewriteEnabled",
                table: "resources");

            migrationBuilder.DropColumn(
                name: "ExplicitRoutingOverride",
                table: "resources");

            migrationBuilder.DropColumn(
                name: "SecurityProfileName",
                table: "resources");
        }
    }
}
