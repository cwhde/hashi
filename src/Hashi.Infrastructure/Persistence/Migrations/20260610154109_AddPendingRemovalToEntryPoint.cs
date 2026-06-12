using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddPendingRemovalToEntryPoint : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<bool>(
                name: "PendingRemoval",
                table: "traefik_entrypoints",
                type: "boolean",
                nullable: false,
                defaultValue: false);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropColumn(
                name: "PendingRemoval",
                table: "traefik_entrypoints");
        }
    }
}
