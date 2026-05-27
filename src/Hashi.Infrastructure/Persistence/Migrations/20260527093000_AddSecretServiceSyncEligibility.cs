using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    [DbContext(typeof(HashiDbContext))]
    [Migration("20260527093000_AddSecretServiceSyncEligibility")]
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
