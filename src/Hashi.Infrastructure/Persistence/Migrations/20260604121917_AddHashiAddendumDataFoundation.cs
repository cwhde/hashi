using System;
using Microsoft.EntityFrameworkCore.Migrations;
using Npgsql.EntityFrameworkCore.PostgreSQL.Metadata;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddHashiAddendumDataFoundation : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<int>(
                name: "BucketSizeSeconds",
                table: "security_request_buckets",
                type: "integer",
                nullable: false,
                defaultValue: 60);

            migrationBuilder.AddColumn<long>(
                name: "ChallengeIgnoredCount",
                table: "security_request_buckets",
                type: "bigint",
                nullable: false,
                defaultValue: 0L);

            migrationBuilder.AddColumn<string>(
                name: "Country",
                table: "security_request_buckets",
                type: "character varying(32)",
                maxLength: 32,
                nullable: true);

            migrationBuilder.AddColumn<long>(
                name: "FailedChallengeCount",
                table: "security_request_buckets",
                type: "bigint",
                nullable: false,
                defaultValue: 0L);

            migrationBuilder.AddColumn<string>(
                name: "NormalizedSubjectValue",
                table: "security_request_buckets",
                type: "character varying(256)",
                maxLength: 256,
                nullable: false,
                defaultValue: "");

            migrationBuilder.AddColumn<string>(
                name: "Region",
                table: "security_request_buckets",
                type: "character varying(64)",
                maxLength: 64,
                nullable: true);

            migrationBuilder.AddColumn<long>(
                name: "RequestCount",
                table: "security_request_buckets",
                type: "bigint",
                nullable: false,
                defaultValue: 0L);

            migrationBuilder.AddColumn<Guid>(
                name: "ResourceId",
                table: "security_request_buckets",
                type: "uuid",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "RootDomain",
                table: "security_request_buckets",
                type: "character varying(256)",
                maxLength: 256,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "SubjectType",
                table: "security_request_buckets",
                type: "character varying(32)",
                maxLength: 32,
                nullable: false,
                defaultValue: "ip");

            migrationBuilder.AddColumn<Guid>(
                name: "ConnectionId",
                table: "security_events",
                type: "uuid",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "Decision",
                table: "security_events",
                type: "character varying(64)",
                maxLength: 64,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "EventType",
                table: "security_events",
                type: "character varying(64)",
                maxLength: 64,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "MetadataJson",
                table: "security_events",
                type: "jsonb",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "NormalizedSubjectValue",
                table: "security_events",
                type: "character varying(256)",
                maxLength: 256,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "Reason",
                table: "security_events",
                type: "text",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "RequestId",
                table: "security_events",
                type: "character varying(128)",
                maxLength: 128,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "RequestMethod",
                table: "security_events",
                type: "character varying(16)",
                maxLength: 16,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "RequestPath",
                table: "security_events",
                type: "text",
                nullable: true);

            migrationBuilder.AddColumn<Guid>(
                name: "ResourceId",
                table: "security_events",
                type: "uuid",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "Severity",
                table: "security_events",
                type: "character varying(32)",
                maxLength: 32,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "Source",
                table: "security_events",
                type: "character varying(64)",
                maxLength: 64,
                nullable: true);

            migrationBuilder.AddColumn<int>(
                name: "StatusCode",
                table: "security_events",
                type: "integer",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "SubjectType",
                table: "security_events",
                type: "character varying(32)",
                maxLength: 32,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "SubjectValue",
                table: "security_events",
                type: "character varying(256)",
                maxLength: 256,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "UserAgentHash",
                table: "security_events",
                type: "character varying(128)",
                maxLength: 128,
                nullable: true);

            migrationBuilder.AddColumn<bool>(
                name: "Enabled",
                table: "blocklist_entries",
                type: "boolean",
                nullable: false,
                defaultValue: true);

            migrationBuilder.AddColumn<string>(
                name: "EnforcementMode",
                table: "blocklist_entries",
                type: "character varying(32)",
                maxLength: 32,
                nullable: false,
                defaultValue: "middleware");

            migrationBuilder.AddColumn<string>(
                name: "MetadataJson",
                table: "blocklist_entries",
                type: "jsonb",
                nullable: false,
                defaultValueSql: "'{}'::jsonb");

            migrationBuilder.AddColumn<string>(
                name: "NormalizedValue",
                table: "blocklist_entries",
                type: "character varying(256)",
                maxLength: 256,
                nullable: false,
                defaultValue: "");

            migrationBuilder.AddColumn<Guid>(
                name: "SourceId",
                table: "blocklist_entries",
                type: "uuid",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "SubjectType",
                table: "blocklist_entries",
                type: "character varying(32)",
                maxLength: 32,
                nullable: false,
                defaultValue: "ip");

            migrationBuilder.CreateTable(
                name: "blocklist_sources",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    Name = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    SourceUrl = table.Column<string>(type: "character varying(1024)", maxLength: 1024, nullable: false),
                    Description = table.Column<string>(type: "character varying(1024)", maxLength: 1024, nullable: false),
                    Format = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false, defaultValue: "text"),
                    EnforcementMode = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false, defaultValue: "middleware"),
                    CanFirewallEnforce = table.Column<bool>(type: "boolean", nullable: false),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false, defaultValue: false),
                    AllowHttp = table.Column<bool>(type: "boolean", nullable: false, defaultValue: false),
                    RefreshIntervalHours = table.Column<int>(type: "integer", nullable: false, defaultValue: 24),
                    MaxRedirects = table.Column<int>(type: "integer", nullable: false, defaultValue: 3),
                    MaxResponseBytes = table.Column<int>(type: "integer", nullable: false, defaultValue: 5242880),
                    TimeoutSeconds = table.Column<int>(type: "integer", nullable: false, defaultValue: 15),
                    ETag = table.Column<string>(type: "character varying(512)", maxLength: 512, nullable: true),
                    LastModified = table.Column<string>(type: "character varying(512)", maxLength: 512, nullable: true),
                    LastContentHash = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: true),
                    LastFetchedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    LastFetchStatus = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false, defaultValue: "never_run"),
                    LastFetchError = table.Column<string>(type: "text", nullable: true),
                    MetadataJson = table.Column<string>(type: "jsonb", nullable: false, defaultValueSql: "'{}'::jsonb"),
                    CreatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    UpdatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_blocklist_sources", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "captcha_settings",
                columns: table => new
                {
                    Id = table.Column<int>(type: "integer", nullable: false)
                        .Annotation("Npgsql:ValueGenerationStrategy", NpgsqlValueGenerationStrategy.IdentityByDefaultColumn),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false, defaultValue: false),
                    PublicChallengeBaseUrl = table.Column<string>(type: "character varying(512)", maxLength: 512, nullable: true),
                    SiteKey = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: true),
                    SecretKeySecretId = table.Column<Guid>(type: "uuid", nullable: true),
                    VerificationTimeoutSeconds = table.Column<int>(type: "integer", nullable: false, defaultValue: 5),
                    CapAdminResourceId = table.Column<Guid>(type: "uuid", nullable: true),
                    CapAdminDomain = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: true),
                    PublicChallengeResourceId = table.Column<Guid>(type: "uuid", nullable: true),
                    PublicChallengeDomain = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: true),
                    ChallengeResetMode = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false, defaultValue: "decay"),
                    ChallengeDecayPercent = table.Column<int>(type: "integer", nullable: false, defaultValue: 50),
                    MinimumRepeatChallengeSeconds = table.Column<int>(type: "integer", nullable: false, defaultValue: 300),
                    MaximumFailuresBeforeEscalation = table.Column<int>(type: "integer", nullable: false, defaultValue: 5),
                    MaximumRequestsWhileChallenged = table.Column<int>(type: "integer", nullable: false, defaultValue: 30),
                    UpdatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_captcha_settings", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "connection_targets",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    OwnerType = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                    OwnerId = table.Column<Guid>(type: "uuid", nullable: false),
                    TargetMode = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false, defaultValue: "static_host"),
                    StaticHost = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: true),
                    StaticIp = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: true),
                    PulseAgentId = table.Column<Guid>(type: "uuid", nullable: true),
                    PulseIpMode = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false, defaultValue: "selected"),
                    PrivateCandidateSelector = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false, defaultValue: "selected"),
                    Port = table.Column<int>(type: "integer", nullable: false),
                    Scheme = table.Column<string>(type: "character varying(16)", maxLength: 16, nullable: false, defaultValue: "http"),
                    PathPrefix = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: true),
                    TlsValidationMode = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false, defaultValue: "system"),
                    ExpectedTlsHostname = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: true),
                    ResolvedIpSnapshot = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: true),
                    LastResolvedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    Status = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false, defaultValue: "unresolved"),
                    LastError = table.Column<string>(type: "text", nullable: true),
                    CreatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    UpdatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_connection_targets", x => x.Id);
                    table.ForeignKey(
                        name: "FK_connection_targets_pulse_agents_PulseAgentId",
                        column: x => x.PulseAgentId,
                        principalTable: "pulse_agents",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.SetNull);
                });

            migrationBuilder.CreateTable(
                name: "internal_agent_dns_agent_settings",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    PulseAgentId = table.Column<Guid>(type: "uuid", nullable: false),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false, defaultValue: true),
                    NameOverride = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: true),
                    IpMode = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false, defaultValue: "selected"),
                    KeepLastRewriteWhenStale = table.Column<bool>(type: "boolean", nullable: false, defaultValue: true),
                    UpdatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_internal_agent_dns_agent_settings", x => x.Id);
                    table.ForeignKey(
                        name: "FK_internal_agent_dns_agent_settings_pulse_agents_PulseAgentId",
                        column: x => x.PulseAgentId,
                        principalTable: "pulse_agents",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateTable(
                name: "internal_agent_dns_settings",
                columns: table => new
                {
                    Id = table.Column<int>(type: "integer", nullable: false)
                        .Annotation("Npgsql:ValueGenerationStrategy", NpgsqlValueGenerationStrategy.IdentityByDefaultColumn),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false, defaultValue: false),
                    Domain = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false, defaultValue: "hashi.home.arpa"),
                    KeepLastRewriteWhenAgentStale = table.Column<bool>(type: "boolean", nullable: false, defaultValue: true),
                    AdGuardConnectionId = table.Column<Guid>(type: "uuid", nullable: true),
                    LastSyncStatus = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false, defaultValue: "never_run"),
                    LastAppliedHash = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: true),
                    UpdatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_internal_agent_dns_settings", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "manual_security_entries",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    SubjectType = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    SubjectValue = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false),
                    NormalizedValue = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false),
                    EntryType = table.Column<string>(type: "character varying(16)", maxLength: 16, nullable: false),
                    ScopeType = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                    ScopeId = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: true),
                    Reason = table.Column<string>(type: "character varying(512)", maxLength: 512, nullable: true),
                    CreatedByAdminId = table.Column<Guid>(type: "uuid", nullable: true),
                    CreatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    ExpiresAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    IsPermanent = table.Column<bool>(type: "boolean", nullable: false, defaultValue: true),
                    BypassBlocking = table.Column<bool>(type: "boolean", nullable: false, defaultValue: true),
                    BypassAdaptiveEscalation = table.Column<bool>(type: "boolean", nullable: false, defaultValue: true),
                    BypassRateLimit = table.Column<bool>(type: "boolean", nullable: false, defaultValue: false),
                    BypassChallenge = table.Column<bool>(type: "boolean", nullable: false, defaultValue: false),
                    BypassSso = table.Column<bool>(type: "boolean", nullable: false, defaultValue: false),
                    Enabled = table.Column<bool>(type: "boolean", nullable: false, defaultValue: true),
                    LastHitAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_manual_security_entries", x => x.Id);
                    table.CheckConstraint("CK_manual_security_entries_block_bypass_flags_false", "\"EntryType\" <> 'block' OR (NOT \"BypassBlocking\" AND NOT \"BypassAdaptiveEscalation\" AND NOT \"BypassRateLimit\" AND NOT \"BypassChallenge\" AND NOT \"BypassSso\")");
                });

            migrationBuilder.CreateTable(
                name: "security_policy_settings",
                columns: table => new
                {
                    Id = table.Column<int>(type: "integer", nullable: false)
                        .Annotation("Npgsql:ValueGenerationStrategy", NpgsqlValueGenerationStrategy.IdentityByDefaultColumn),
                    DefaultSoftBlockPolicyJson = table.Column<string>(type: "jsonb", nullable: false, defaultValue: "{\"policyType\":\"constant\",\"baseDurationSeconds\":600,\"linearMultiplier\":1,\"exponentialMultiplier\":2,\"maxDurationSeconds\":3600,\"permanentAfterCount\":0,\"countWindowSeconds\":86400,\"resetCountAfterSeconds\":604800}"),
                    DefaultFirewallBlockPolicyJson = table.Column<string>(type: "jsonb", nullable: false, defaultValue: "{\"policyType\":\"capped_exponential\",\"baseDurationSeconds\":3600,\"linearMultiplier\":1,\"exponentialMultiplier\":2,\"maxDurationSeconds\":86400,\"permanentAfterCount\":0,\"countWindowSeconds\":604800,\"resetCountAfterSeconds\":2592000}"),
                    RepeatOffenderPolicyJson = table.Column<string>(type: "jsonb", nullable: false, defaultValue: "{\"policyType\":\"permanent_after_count\",\"baseDurationSeconds\":3600,\"linearMultiplier\":1,\"exponentialMultiplier\":2,\"maxDurationSeconds\":86400,\"permanentAfterCount\":5,\"countWindowSeconds\":2592000,\"resetCountAfterSeconds\":7776000}"),
                    ChallengeIgnoredThreshold = table.Column<int>(type: "integer", nullable: false, defaultValue: 30),
                    ChallengeIgnoredWindowSeconds = table.Column<int>(type: "integer", nullable: false, defaultValue: 300),
                    FirewallBlockThresholdWhileChallenged = table.Column<int>(type: "integer", nullable: false, defaultValue: 100),
                    CaptchaSuccessDecaysTriggeringBuckets = table.Column<bool>(type: "boolean", nullable: false, defaultValue: true),
                    CaptchaSuccessBucketDecayPercent = table.Column<int>(type: "integer", nullable: false, defaultValue: 50),
                    UpdatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_security_policy_settings", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "security_subjects",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    SubjectType = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    SubjectValue = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false),
                    NormalizedValue = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false),
                    FirstSeenAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    LastSeenAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    LastCountry = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: true),
                    LastRegion = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: true),
                    LastAsn = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: true),
                    LastAsOrg = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: true),
                    CurrentState = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false, defaultValue: "observed"),
                    MetadataJson = table.Column<string>(type: "jsonb", nullable: false, defaultValueSql: "'{}'::jsonb")
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_security_subjects", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "blocklist_fetch_runs",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    BlocklistSourceId = table.Column<Guid>(type: "uuid", nullable: false),
                    StartedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    CompletedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    Status = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    HttpStatusCode = table.Column<int>(type: "integer", nullable: true),
                    EntryCount = table.Column<int>(type: "integer", nullable: false),
                    AddedCount = table.Column<int>(type: "integer", nullable: false),
                    RemovedCount = table.Column<int>(type: "integer", nullable: false),
                    UnchangedCount = table.Column<int>(type: "integer", nullable: false),
                    ContentHash = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: true),
                    ETag = table.Column<string>(type: "character varying(512)", maxLength: 512, nullable: true),
                    LastModified = table.Column<string>(type: "character varying(512)", maxLength: 512, nullable: true),
                    Error = table.Column<string>(type: "text", nullable: true),
                    MetadataJson = table.Column<string>(type: "jsonb", nullable: false, defaultValueSql: "'{}'::jsonb")
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_blocklist_fetch_runs", x => x.Id);
                    table.ForeignKey(
                        name: "FK_blocklist_fetch_runs_blocklist_sources_BlocklistSourceId",
                        column: x => x.BlocklistSourceId,
                        principalTable: "blocklist_sources",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateTable(
                name: "security_subject_states",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    SecuritySubjectId = table.Column<Guid>(type: "uuid", nullable: false),
                    ChallengeRequired = table.Column<bool>(type: "boolean", nullable: false, defaultValue: false),
                    ChallengeRequiredSinceUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    ChallengeReason = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: true),
                    ChallengeResourceId = table.Column<Guid>(type: "uuid", nullable: true),
                    ChallengeAttempts = table.Column<int>(type: "integer", nullable: false),
                    RequestsWhileChallenged = table.Column<int>(type: "integer", nullable: false),
                    FailedChallengeCount = table.Column<int>(type: "integer", nullable: false),
                    SuccessfulChallengeCount = table.Column<int>(type: "integer", nullable: false),
                    LastChallengeSolvedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    SoftBlockedUntilUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    FirewallBlockedUntilUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    ManualAllowActive = table.Column<bool>(type: "boolean", nullable: false, defaultValue: false),
                    ManualBlockActive = table.Column<bool>(type: "boolean", nullable: false, defaultValue: false),
                    LastEscalationReason = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: true),
                    LastEscalationAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    UpdatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_security_subject_states", x => x.Id);
                    table.ForeignKey(
                        name: "FK_security_subject_states_security_subjects_SecuritySubjectId",
                        column: x => x.SecuritySubjectId,
                        principalTable: "security_subjects",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateIndex(
                name: "IX_security_request_buckets_NormalizedSubjectValue_BucketStart~",
                table: "security_request_buckets",
                columns: new[] { "NormalizedSubjectValue", "BucketStartUtc" });

            migrationBuilder.CreateIndex(
                name: "IX_security_request_buckets_ResourceId_BucketStartUtc",
                table: "security_request_buckets",
                columns: new[] { "ResourceId", "BucketStartUtc" });

            migrationBuilder.CreateIndex(
                name: "IX_security_events_EventType_OccurredAtUtc",
                table: "security_events",
                columns: new[] { "EventType", "OccurredAtUtc" });

            migrationBuilder.CreateIndex(
                name: "IX_security_events_NormalizedSubjectValue_OccurredAtUtc",
                table: "security_events",
                columns: new[] { "NormalizedSubjectValue", "OccurredAtUtc" });

            migrationBuilder.CreateIndex(
                name: "IX_security_events_ResourceId_OccurredAtUtc",
                table: "security_events",
                columns: new[] { "ResourceId", "OccurredAtUtc" });

            migrationBuilder.CreateIndex(
                name: "IX_security_events_Severity_OccurredAtUtc",
                table: "security_events",
                columns: new[] { "Severity", "OccurredAtUtc" });

            migrationBuilder.CreateIndex(
                name: "IX_blocklist_entries_Enabled",
                table: "blocklist_entries",
                column: "Enabled");

            migrationBuilder.CreateIndex(
                name: "IX_blocklist_entries_SourceId",
                table: "blocklist_entries",
                column: "SourceId");

            migrationBuilder.CreateIndex(
                name: "IX_blocklist_entries_SubjectType_NormalizedValue",
                table: "blocklist_entries",
                columns: new[] { "SubjectType", "NormalizedValue" });

            migrationBuilder.CreateIndex(
                name: "IX_blocklist_fetch_runs_BlocklistSourceId_StartedAtUtc",
                table: "blocklist_fetch_runs",
                columns: new[] { "BlocklistSourceId", "StartedAtUtc" });

            migrationBuilder.CreateIndex(
                name: "IX_blocklist_fetch_runs_Status",
                table: "blocklist_fetch_runs",
                column: "Status");

            migrationBuilder.CreateIndex(
                name: "IX_blocklist_sources_Enabled",
                table: "blocklist_sources",
                column: "Enabled");

            migrationBuilder.CreateIndex(
                name: "IX_blocklist_sources_SourceUrl",
                table: "blocklist_sources",
                column: "SourceUrl");

            migrationBuilder.CreateIndex(
                name: "IX_captcha_settings_CapAdminResourceId",
                table: "captcha_settings",
                column: "CapAdminResourceId");

            migrationBuilder.CreateIndex(
                name: "IX_captcha_settings_PublicChallengeResourceId",
                table: "captcha_settings",
                column: "PublicChallengeResourceId");

            migrationBuilder.CreateIndex(
                name: "IX_captcha_settings_SecretKeySecretId",
                table: "captcha_settings",
                column: "SecretKeySecretId");

            migrationBuilder.CreateIndex(
                name: "IX_connection_targets_OwnerType_OwnerId",
                table: "connection_targets",
                columns: new[] { "OwnerType", "OwnerId" },
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_connection_targets_PulseAgentId",
                table: "connection_targets",
                column: "PulseAgentId");

            migrationBuilder.CreateIndex(
                name: "IX_connection_targets_Status",
                table: "connection_targets",
                column: "Status");

            migrationBuilder.CreateIndex(
                name: "IX_internal_agent_dns_agent_settings_PulseAgentId",
                table: "internal_agent_dns_agent_settings",
                column: "PulseAgentId",
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_internal_agent_dns_settings_AdGuardConnectionId",
                table: "internal_agent_dns_settings",
                column: "AdGuardConnectionId");

            migrationBuilder.CreateIndex(
                name: "IX_manual_security_entries_EntryType_SubjectType_NormalizedVal~",
                table: "manual_security_entries",
                columns: new[] { "EntryType", "SubjectType", "NormalizedValue" });

            migrationBuilder.CreateIndex(
                name: "IX_manual_security_entries_ExpiresAtUtc",
                table: "manual_security_entries",
                column: "ExpiresAtUtc");

            migrationBuilder.CreateIndex(
                name: "IX_manual_security_entries_ScopeType_ScopeId",
                table: "manual_security_entries",
                columns: new[] { "ScopeType", "ScopeId" });

            migrationBuilder.CreateIndex(
                name: "IX_security_subject_states_ChallengeRequired",
                table: "security_subject_states",
                column: "ChallengeRequired");

            migrationBuilder.CreateIndex(
                name: "IX_security_subject_states_FirewallBlockedUntilUtc",
                table: "security_subject_states",
                column: "FirewallBlockedUntilUtc");

            migrationBuilder.CreateIndex(
                name: "IX_security_subject_states_SecuritySubjectId",
                table: "security_subject_states",
                column: "SecuritySubjectId",
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_security_subject_states_SoftBlockedUntilUtc",
                table: "security_subject_states",
                column: "SoftBlockedUntilUtc");

            migrationBuilder.CreateIndex(
                name: "IX_security_subjects_CurrentState",
                table: "security_subjects",
                column: "CurrentState");

            migrationBuilder.CreateIndex(
                name: "IX_security_subjects_LastSeenAtUtc",
                table: "security_subjects",
                column: "LastSeenAtUtc");

            migrationBuilder.CreateIndex(
                name: "IX_security_subjects_SubjectType_NormalizedValue",
                table: "security_subjects",
                columns: new[] { "SubjectType", "NormalizedValue" },
                unique: true);

            migrationBuilder.AddForeignKey(
                name: "FK_blocklist_entries_blocklist_sources_SourceId",
                table: "blocklist_entries",
                column: "SourceId",
                principalTable: "blocklist_sources",
                principalColumn: "Id",
                onDelete: ReferentialAction.SetNull);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropForeignKey(
                name: "FK_blocklist_entries_blocklist_sources_SourceId",
                table: "blocklist_entries");

            migrationBuilder.DropTable(
                name: "blocklist_fetch_runs");

            migrationBuilder.DropTable(
                name: "captcha_settings");

            migrationBuilder.DropTable(
                name: "connection_targets");

            migrationBuilder.DropTable(
                name: "internal_agent_dns_agent_settings");

            migrationBuilder.DropTable(
                name: "internal_agent_dns_settings");

            migrationBuilder.DropTable(
                name: "manual_security_entries");

            migrationBuilder.DropTable(
                name: "security_policy_settings");

            migrationBuilder.DropTable(
                name: "security_subject_states");

            migrationBuilder.DropTable(
                name: "blocklist_sources");

            migrationBuilder.DropTable(
                name: "security_subjects");

            migrationBuilder.DropIndex(
                name: "IX_security_request_buckets_NormalizedSubjectValue_BucketStart~",
                table: "security_request_buckets");

            migrationBuilder.DropIndex(
                name: "IX_security_request_buckets_ResourceId_BucketStartUtc",
                table: "security_request_buckets");

            migrationBuilder.DropIndex(
                name: "IX_security_events_EventType_OccurredAtUtc",
                table: "security_events");

            migrationBuilder.DropIndex(
                name: "IX_security_events_NormalizedSubjectValue_OccurredAtUtc",
                table: "security_events");

            migrationBuilder.DropIndex(
                name: "IX_security_events_ResourceId_OccurredAtUtc",
                table: "security_events");

            migrationBuilder.DropIndex(
                name: "IX_security_events_Severity_OccurredAtUtc",
                table: "security_events");

            migrationBuilder.DropIndex(
                name: "IX_blocklist_entries_Enabled",
                table: "blocklist_entries");

            migrationBuilder.DropIndex(
                name: "IX_blocklist_entries_SourceId",
                table: "blocklist_entries");

            migrationBuilder.DropIndex(
                name: "IX_blocklist_entries_SubjectType_NormalizedValue",
                table: "blocklist_entries");

            migrationBuilder.DropColumn(
                name: "BucketSizeSeconds",
                table: "security_request_buckets");

            migrationBuilder.DropColumn(
                name: "ChallengeIgnoredCount",
                table: "security_request_buckets");

            migrationBuilder.DropColumn(
                name: "Country",
                table: "security_request_buckets");

            migrationBuilder.DropColumn(
                name: "FailedChallengeCount",
                table: "security_request_buckets");

            migrationBuilder.DropColumn(
                name: "NormalizedSubjectValue",
                table: "security_request_buckets");

            migrationBuilder.DropColumn(
                name: "Region",
                table: "security_request_buckets");

            migrationBuilder.DropColumn(
                name: "RequestCount",
                table: "security_request_buckets");

            migrationBuilder.DropColumn(
                name: "ResourceId",
                table: "security_request_buckets");

            migrationBuilder.DropColumn(
                name: "RootDomain",
                table: "security_request_buckets");

            migrationBuilder.DropColumn(
                name: "SubjectType",
                table: "security_request_buckets");

            migrationBuilder.DropColumn(
                name: "ConnectionId",
                table: "security_events");

            migrationBuilder.DropColumn(
                name: "Decision",
                table: "security_events");

            migrationBuilder.DropColumn(
                name: "EventType",
                table: "security_events");

            migrationBuilder.DropColumn(
                name: "MetadataJson",
                table: "security_events");

            migrationBuilder.DropColumn(
                name: "NormalizedSubjectValue",
                table: "security_events");

            migrationBuilder.DropColumn(
                name: "Reason",
                table: "security_events");

            migrationBuilder.DropColumn(
                name: "RequestId",
                table: "security_events");

            migrationBuilder.DropColumn(
                name: "RequestMethod",
                table: "security_events");

            migrationBuilder.DropColumn(
                name: "RequestPath",
                table: "security_events");

            migrationBuilder.DropColumn(
                name: "ResourceId",
                table: "security_events");

            migrationBuilder.DropColumn(
                name: "Severity",
                table: "security_events");

            migrationBuilder.DropColumn(
                name: "Source",
                table: "security_events");

            migrationBuilder.DropColumn(
                name: "StatusCode",
                table: "security_events");

            migrationBuilder.DropColumn(
                name: "SubjectType",
                table: "security_events");

            migrationBuilder.DropColumn(
                name: "SubjectValue",
                table: "security_events");

            migrationBuilder.DropColumn(
                name: "UserAgentHash",
                table: "security_events");

            migrationBuilder.DropColumn(
                name: "Enabled",
                table: "blocklist_entries");

            migrationBuilder.DropColumn(
                name: "EnforcementMode",
                table: "blocklist_entries");

            migrationBuilder.DropColumn(
                name: "MetadataJson",
                table: "blocklist_entries");

            migrationBuilder.DropColumn(
                name: "NormalizedValue",
                table: "blocklist_entries");

            migrationBuilder.DropColumn(
                name: "SourceId",
                table: "blocklist_entries");

            migrationBuilder.DropColumn(
                name: "SubjectType",
                table: "blocklist_entries");
        }
    }
}
