using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddResourceSecurityAndFirewallPublicIp : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<string>(
                name: "ForwardAuthPolicy",
                table: "resources",
                type: "text",
                nullable: false,
                defaultValue: "");

            migrationBuilder.AddColumn<string>(
                name: "WafMode",
                table: "resources",
                type: "text",
                nullable: false,
                defaultValue: "");

            migrationBuilder.AddColumn<string>(
                name: "PublicIp",
                table: "firewall_hosts",
                type: "text",
                nullable: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropColumn(
                name: "ForwardAuthPolicy",
                table: "resources");

            migrationBuilder.DropColumn(
                name: "WafMode",
                table: "resources");

            migrationBuilder.DropColumn(
                name: "PublicIp",
                table: "firewall_hosts");
        }
    }
}
