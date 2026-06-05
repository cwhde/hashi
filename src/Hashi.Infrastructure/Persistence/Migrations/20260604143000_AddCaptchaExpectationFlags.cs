using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations;

public partial class AddCaptchaExpectationFlags : Migration
{
    protected override void Up(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.AddColumn<bool>(
            name: "InstrumentationExpected",
            table: "captcha_settings",
            type: "boolean",
            nullable: false,
            defaultValue: true);

        migrationBuilder.AddColumn<bool>(
            name: "HeadlessDetectionExpected",
            table: "captcha_settings",
            type: "boolean",
            nullable: false,
            defaultValue: false);
    }

    protected override void Down(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.DropColumn(
            name: "InstrumentationExpected",
            table: "captcha_settings");

        migrationBuilder.DropColumn(
            name: "HeadlessDetectionExpected",
            table: "captcha_settings");
    }
}
