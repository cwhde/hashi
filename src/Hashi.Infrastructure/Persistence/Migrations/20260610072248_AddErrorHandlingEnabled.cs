using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddErrorHandlingEnabled : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<bool>(
                name: "ErrorHandlingEnabled",
                table: "resources",
                type: "boolean",
                nullable: false,
                defaultValue: false);

            migrationBuilder.AddColumn<bool>(
                name: "ErrorHandlingEnabled",
                table: "app_settings",
                type: "boolean",
                nullable: false,
                defaultValue: false);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropColumn(
                name: "ErrorHandlingEnabled",
                table: "resources");

            migrationBuilder.DropColumn(
                name: "ErrorHandlingEnabled",
                table: "app_settings");
        }
    }
}
