using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddSecretServiceSyncEligibility : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<bool>(
                name: "IsServiceSyncEligible",
                table: "secret_records",
                type: "boolean",
                nullable: false,
                defaultValue: false);

            migrationBuilder.CreateIndex(
                name: "IX_secret_records_IsServiceSyncEligible",
                table: "secret_records",
                column: "IsServiceSyncEligible");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropIndex(
                name: "IX_secret_records_IsServiceSyncEligible",
                table: "secret_records");

            migrationBuilder.DropColumn(
                name: "IsServiceSyncEligible",
                table: "secret_records");
        }
    }
}
