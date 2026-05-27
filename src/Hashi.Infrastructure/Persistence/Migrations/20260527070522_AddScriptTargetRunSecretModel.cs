using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddScriptTargetRunSecretModel : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<string>(
                name: "LastRunError",
                table: "scripts",
                type: "text",
                nullable: true);

            migrationBuilder.AddColumn<Guid>(
                name: "LastRunId",
                table: "scripts",
                type: "uuid",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "LastRunStatus",
                table: "scripts",
                type: "character varying(32)",
                maxLength: 32,
                nullable: false,
                defaultValue: "never_run");

            migrationBuilder.AddColumn<int>(
                name: "RunTimeoutSeconds",
                table: "scripts",
                type: "integer",
                nullable: false,
                defaultValue: 300);

            migrationBuilder.CreateTable(
                name: "host_script_environment_variables",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ScriptId = table.Column<Guid>(type: "uuid", nullable: false),
                    Name = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    IsSecret = table.Column<bool>(type: "boolean", nullable: false),
                    PlainValue = table.Column<string>(type: "text", nullable: true),
                    SecretId = table.Column<Guid>(type: "uuid", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_host_script_environment_variables", x => x.Id);
                    table.ForeignKey(
                        name: "FK_host_script_environment_variables_scripts_ScriptId",
                        column: x => x.ScriptId,
                        principalTable: "scripts",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                    table.ForeignKey(
                        name: "FK_host_script_environment_variables_secret_records_SecretId",
                        column: x => x.SecretId,
                        principalTable: "secret_records",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Restrict);
                });

            migrationBuilder.CreateTable(
                name: "host_script_runs",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ScriptId = table.Column<Guid>(type: "uuid", nullable: false),
                    ConnectionId = table.Column<Guid>(type: "uuid", nullable: false),
                    StartedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    CompletedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    Status = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    Succeeded = table.Column<bool>(type: "boolean", nullable: false),
                    Error = table.Column<string>(type: "text", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_host_script_runs", x => x.Id);
                    table.ForeignKey(
                        name: "FK_host_script_runs_connections_ConnectionId",
                        column: x => x.ConnectionId,
                        principalTable: "connections",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Restrict);
                    table.ForeignKey(
                        name: "FK_host_script_runs_scripts_ScriptId",
                        column: x => x.ScriptId,
                        principalTable: "scripts",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateTable(
                name: "host_script_targets",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    ScriptId = table.Column<Guid>(type: "uuid", nullable: false),
                    ConnectionId = table.Column<Guid>(type: "uuid", nullable: false),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_host_script_targets", x => x.Id);
                    table.ForeignKey(
                        name: "FK_host_script_targets_connections_ConnectionId",
                        column: x => x.ConnectionId,
                        principalTable: "connections",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Restrict);
                    table.ForeignKey(
                        name: "FK_host_script_targets_scripts_ScriptId",
                        column: x => x.ScriptId,
                        principalTable: "scripts",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateTable(
                name: "host_script_outputs",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    RunId = table.Column<Guid>(type: "uuid", nullable: false),
                    Stream = table.Column<string>(type: "character varying(16)", maxLength: 16, nullable: false),
                    Content = table.Column<string>(type: "text", nullable: false),
                    CapturedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_host_script_outputs", x => x.Id);
                    table.ForeignKey(
                        name: "FK_host_script_outputs_host_script_runs_RunId",
                        column: x => x.RunId,
                        principalTable: "host_script_runs",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateIndex(
                name: "IX_scripts_ConnectionId",
                table: "scripts",
                column: "ConnectionId");

            migrationBuilder.CreateIndex(
                name: "IX_host_script_environment_variables_ScriptId_Name",
                table: "host_script_environment_variables",
                columns: new[] { "ScriptId", "Name" },
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_host_script_environment_variables_SecretId",
                table: "host_script_environment_variables",
                column: "SecretId");

            migrationBuilder.CreateIndex(
                name: "IX_host_script_outputs_RunId",
                table: "host_script_outputs",
                column: "RunId");

            migrationBuilder.CreateIndex(
                name: "IX_host_script_runs_ConnectionId",
                table: "host_script_runs",
                column: "ConnectionId");

            migrationBuilder.CreateIndex(
                name: "IX_host_script_runs_ScriptId_StartedAtUtc",
                table: "host_script_runs",
                columns: new[] { "ScriptId", "StartedAtUtc" });

            migrationBuilder.CreateIndex(
                name: "IX_host_script_targets_ConnectionId",
                table: "host_script_targets",
                column: "ConnectionId");

            migrationBuilder.CreateIndex(
                name: "IX_host_script_targets_ScriptId_ConnectionId",
                table: "host_script_targets",
                columns: new[] { "ScriptId", "ConnectionId" },
                unique: true);

            migrationBuilder.AddForeignKey(
                name: "FK_scripts_connections_ConnectionId",
                table: "scripts",
                column: "ConnectionId",
                principalTable: "connections",
                principalColumn: "Id",
                onDelete: ReferentialAction.Restrict);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropForeignKey(
                name: "FK_scripts_connections_ConnectionId",
                table: "scripts");

            migrationBuilder.DropTable(
                name: "host_script_environment_variables");

            migrationBuilder.DropTable(
                name: "host_script_outputs");

            migrationBuilder.DropTable(
                name: "host_script_targets");

            migrationBuilder.DropTable(
                name: "host_script_runs");

            migrationBuilder.DropIndex(
                name: "IX_scripts_ConnectionId",
                table: "scripts");

            migrationBuilder.DropColumn(
                name: "LastRunError",
                table: "scripts");

            migrationBuilder.DropColumn(
                name: "LastRunId",
                table: "scripts");

            migrationBuilder.DropColumn(
                name: "LastRunStatus",
                table: "scripts");

            migrationBuilder.DropColumn(
                name: "RunTimeoutSeconds",
                table: "scripts");
        }
    }
}
