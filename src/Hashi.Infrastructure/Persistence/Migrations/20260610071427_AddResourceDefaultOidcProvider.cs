using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddResourceDefaultOidcProvider : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<Guid>(
                name: "OidcProviderId",
                table: "resources",
                type: "uuid",
                nullable: true);

            migrationBuilder.AddColumn<bool>(
                name: "IsDefault",
                table: "oidc_providers",
                type: "boolean",
                nullable: false,
                defaultValue: false);

            migrationBuilder.CreateIndex(
                name: "IX_resources_OidcProviderId",
                table: "resources",
                column: "OidcProviderId");

            migrationBuilder.AddForeignKey(
                name: "FK_resources_oidc_providers_OidcProviderId",
                table: "resources",
                column: "OidcProviderId",
                principalTable: "oidc_providers",
                principalColumn: "Id",
                onDelete: ReferentialAction.SetNull);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropForeignKey(
                name: "FK_resources_oidc_providers_OidcProviderId",
                table: "resources");

            migrationBuilder.DropIndex(
                name: "IX_resources_OidcProviderId",
                table: "resources");

            migrationBuilder.DropColumn(
                name: "OidcProviderId",
                table: "resources");

            migrationBuilder.DropColumn(
                name: "IsDefault",
                table: "oidc_providers");
        }
    }
}
