using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddTraefikUserMiddlewaresAndResourceExtraMiddlewares : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<string>(
                name: "ExtraMiddlewaresJson",
                table: "resources",
                type: "text",
                nullable: false,
                defaultValue: "");

            migrationBuilder.CreateTable(
                name: "traefik_user_middlewares",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    Yaml = table.Column<string>(type: "text", nullable: false),
                    LastValidYaml = table.Column<string>(type: "text", nullable: true),
                    LastParseError = table.Column<string>(type: "text", nullable: true),
                    UpdatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_traefik_user_middlewares", x => x.Id);
                });
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "traefik_user_middlewares");

            migrationBuilder.DropColumn(
                name: "ExtraMiddlewaresJson",
                table: "resources");
        }
    }
}
