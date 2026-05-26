using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddSetupHttpsAndResourcePathFields : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<DateTimeOffset>(
                name: "HttpsDomainVerifiedAtUtc",
                table: "setup_state",
                type: "timestamp with time zone",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "PathPrefix",
                table: "resources",
                type: "text",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "PathRewrite",
                table: "resources",
                type: "text",
                nullable: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropColumn(
                name: "HttpsDomainVerifiedAtUtc",
                table: "setup_state");

            migrationBuilder.DropColumn(
                name: "PathPrefix",
                table: "resources");

            migrationBuilder.DropColumn(
                name: "PathRewrite",
                table: "resources");
        }
    }
}
