using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations;

[DbContext(typeof(HashiDbContext))]
[Migration("20260602090000_AddResourceDomainAndRewriteModes")]
public partial class AddResourceDomainAndRewriteModes : Migration
{
    protected override void Up(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.AddColumn<string>(
            name: "DomainMode",
            table: "resources",
            type: "character varying(32)",
            maxLength: 32,
            nullable: false,
            defaultValue: "custom");

        migrationBuilder.AddColumn<string>(
            name: "PathRewriteMode",
            table: "resources",
            type: "character varying(32)",
            maxLength: 32,
            nullable: true);

        migrationBuilder.Sql("""
            UPDATE resources
            SET "DomainMode" = 'subdomain'
            WHERE "Domain" IS NULL OR btrim("Domain") = '';
            """);

        migrationBuilder.Sql("""
            UPDATE resources
            SET "DomainMode" = 'root',
                "Domain" = NULL
            WHERE "Domain" = '@';
            """);
    }

    protected override void Down(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.DropColumn(
            name: "DomainMode",
            table: "resources");

        migrationBuilder.DropColumn(
            name: "PathRewriteMode",
            table: "resources");
    }
}
