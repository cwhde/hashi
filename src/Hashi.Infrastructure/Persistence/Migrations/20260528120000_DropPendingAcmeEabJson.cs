using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    [DbContext(typeof(HashiDbContext))]
    [Migration("20260528120000_DropPendingAcmeEabJson")]
    public partial class DropPendingAcmeEabJson : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropColumn(
                name: "PendingAcmeEabJson",
                table: "setup_state");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<string>(
                name: "PendingAcmeEabJson",
                table: "setup_state",
                type: "text",
                nullable: true);
        }
    }
}
