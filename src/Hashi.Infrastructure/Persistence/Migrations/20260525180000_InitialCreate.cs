using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations;

/// <inheritdoc />
public partial class InitialCreate : Migration
{
    /// <inheritdoc />
    protected override void Up(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.CreateTable(
            name: "app_settings",
            columns: table => new
            {
                Id = table.Column<int>(type: "integer", nullable: false),
                RootDomain = table.Column<string>(type: "text", nullable: true),
                AdminDomain = table.Column<string>(type: "text", nullable: true),
                InternalUrl = table.Column<string>(type: "text", nullable: true),
                DefaultSyncIntervalMinutes = table.Column<int>(type: "integer", nullable: false),
                PublicDashboardEnabled = table.Column<bool>(type: "boolean", nullable: false),
                PublicStatusEnabled = table.Column<bool>(type: "boolean", nullable: false),
                Theme = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                UpdatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
            },
            constraints: table => table.PrimaryKey("PK_app_settings", x => x.Id));

        migrationBuilder.CreateTable(
            name: "setup_state",
            columns: table => new
            {
                Id = table.Column<int>(type: "integer", nullable: false),
                IsComplete = table.Column<bool>(type: "boolean", nullable: false),
                CurrentStep = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                CompletedStepsJson = table.Column<string>(type: "text", nullable: false),
                BootstrapUsername = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: true),
                BootstrapPasswordHash = table.Column<string>(type: "text", nullable: true),
                UpdatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
            },
            constraints: table => table.PrimaryKey("PK_setup_state", x => x.Id));

        migrationBuilder.CreateTable(
            name: "audit_events",
            columns: table => new
            {
                Id = table.Column<Guid>(type: "uuid", nullable: false),
                Category = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                Action = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                SubjectType = table.Column<string>(type: "text", nullable: true),
                SubjectId = table.Column<string>(type: "text", nullable: true),
                Outcome = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                MetadataJson = table.Column<string>(type: "text", nullable: true),
                CreatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
            },
            constraints: table => table.PrimaryKey("PK_audit_events", x => x.Id));

        migrationBuilder.CreateIndex(
            name: "IX_audit_events_CreatedAtUtc",
            table: "audit_events",
            column: "CreatedAtUtc");

        migrationBuilder.CreateTable(
            name: "sync_runs",
            columns: table => new
            {
                Id = table.Column<Guid>(type: "uuid", nullable: false),
                Subsystem = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                Status = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                RiskLevel = table.Column<string>(type: "text", nullable: true),
                StartedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                CompletedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                ErrorSummary = table.Column<string>(type: "text", nullable: true),
            },
            constraints: table => table.PrimaryKey("PK_sync_runs", x => x.Id));

        migrationBuilder.CreateIndex(
            name: "IX_sync_runs_StartedAtUtc",
            table: "sync_runs",
            column: "StartedAtUtc");

        migrationBuilder.CreateTable(
            name: "sync_steps",
            columns: table => new
            {
                Id = table.Column<Guid>(type: "uuid", nullable: false),
                SyncRunId = table.Column<Guid>(type: "uuid", nullable: false),
                Name = table.Column<string>(type: "text", nullable: false),
                Status = table.Column<string>(type: "text", nullable: false),
                StartedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                CompletedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                Message = table.Column<string>(type: "text", nullable: true),
            },
            constraints: table =>
            {
                table.PrimaryKey("PK_sync_steps", x => x.Id);
                table.ForeignKey(
                    name: "FK_sync_steps_sync_runs_SyncRunId",
                    column: x => x.SyncRunId,
                    principalTable: "sync_runs",
                    principalColumn: "Id",
                    onDelete: ReferentialAction.Cascade);
            });

        migrationBuilder.CreateTable(
            name: "sync_diffs",
            columns: table => new
            {
                Id = table.Column<Guid>(type: "uuid", nullable: false),
                SyncRunId = table.Column<Guid>(type: "uuid", nullable: false),
                ResourceType = table.Column<string>(type: "text", nullable: false),
                ResourceKey = table.Column<string>(type: "text", nullable: false),
                ChangeKind = table.Column<string>(type: "text", nullable: false),
                Summary = table.Column<string>(type: "text", nullable: true),
                BeforeJson = table.Column<string>(type: "text", nullable: true),
                AfterJson = table.Column<string>(type: "text", nullable: true),
            },
            constraints: table =>
            {
                table.PrimaryKey("PK_sync_diffs", x => x.Id);
                table.ForeignKey(
                    name: "FK_sync_diffs_sync_runs_SyncRunId",
                    column: x => x.SyncRunId,
                    principalTable: "sync_runs",
                    principalColumn: "Id",
                    onDelete: ReferentialAction.Cascade);
            });

        migrationBuilder.CreateIndex(
            name: "IX_sync_steps_SyncRunId",
            table: "sync_steps",
            column: "SyncRunId");

        migrationBuilder.CreateIndex(
            name: "IX_sync_diffs_SyncRunId",
            table: "sync_diffs",
            column: "SyncRunId");
    }

    /// <inheritdoc />
    protected override void Down(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.DropTable(name: "app_settings");
        migrationBuilder.DropTable(name: "audit_events");
        migrationBuilder.DropTable(name: "setup_state");
        migrationBuilder.DropTable(name: "sync_diffs");
        migrationBuilder.DropTable(name: "sync_steps");
        migrationBuilder.DropTable(name: "sync_runs");
    }
}
