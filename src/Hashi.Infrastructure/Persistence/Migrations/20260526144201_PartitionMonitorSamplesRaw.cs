using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations;

/// <inheritdoc />
/// <remarks>
/// security_request_buckets partitioning is deferred to feat/gap-2-request-buckets (table not on main).
/// </remarks>
public partial class PartitionMonitorSamplesRaw : Migration
{
    /// <inheritdoc />
    protected override void Up(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.Sql(
            """
            ALTER TABLE monitor_samples RENAME TO monitor_samples_legacy;

            CREATE TABLE monitor_samples_raw (
                "Id" uuid NOT NULL,
                "MonitorEndpointId" uuid NOT NULL,
                partition_date date NOT NULL,
                "CheckedAtUtc" timestamp with time zone NOT NULL,
                "Status" text NOT NULL,
                "LatencyMs" integer NOT NULL,
                CONSTRAINT "PK_monitor_samples_raw" PRIMARY KEY ("Id", partition_date)
            ) PARTITION BY RANGE (partition_date);

            CREATE INDEX "IX_monitor_samples_raw_MonitorEndpointId_partition_date_CheckedAtUtc"
                ON monitor_samples_raw ("MonitorEndpointId", partition_date, "CheckedAtUtc");

            DO $$
            DECLARE
                week_start date;
                week_end date;
                partition_name text;
                i int;
            BEGIN
                FOR week_start IN
                    SELECT DISTINCT date_trunc('week', "PartitionDate"::timestamp)::date
                    FROM monitor_samples_legacy
                LOOP
                    week_end := week_start + interval '7 days';
                    partition_name := 'monitor_samples_raw_' || to_char(week_start, 'YYYYMMDD');
                    EXECUTE format(
                        'CREATE TABLE IF NOT EXISTS %I PARTITION OF monitor_samples_raw FOR VALUES FROM (%L) TO (%L)',
                        partition_name, week_start, week_end);
                END LOOP;

                week_start := date_trunc('week', CURRENT_DATE)::date;
                FOR i IN 0..1 LOOP
                    week_end := week_start + interval '7 days';
                    partition_name := 'monitor_samples_raw_' || to_char(week_start, 'YYYYMMDD');
                    EXECUTE format(
                        'CREATE TABLE IF NOT EXISTS %I PARTITION OF monitor_samples_raw FOR VALUES FROM (%L) TO (%L)',
                        partition_name, week_start, week_end);
                    week_start := week_end;
                END LOOP;
            END $$;

            INSERT INTO monitor_samples_raw ("Id", "MonitorEndpointId", partition_date, "CheckedAtUtc", "Status", "LatencyMs")
            SELECT "Id", "MonitorEndpointId", "PartitionDate", "CheckedAtUtc", "Status", "LatencyMs"
            FROM monitor_samples_legacy;

            DROP TABLE monitor_samples_legacy;
            """);
    }

    /// <inheritdoc />
    protected override void Down(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.Sql(
            """
            CREATE TABLE monitor_samples (
                "Id" uuid NOT NULL,
                "MonitorEndpointId" uuid NOT NULL,
                "PartitionDate" date NOT NULL,
                "CheckedAtUtc" timestamp with time zone NOT NULL,
                "Status" text NOT NULL,
                "LatencyMs" integer NOT NULL,
                CONSTRAINT "PK_monitor_samples" PRIMARY KEY ("Id")
            );

            CREATE INDEX "IX_monitor_samples_MonitorEndpointId_PartitionDate_CheckedAtUtc"
                ON monitor_samples ("MonitorEndpointId", "PartitionDate", "CheckedAtUtc");

            INSERT INTO monitor_samples ("Id", "MonitorEndpointId", "PartitionDate", "CheckedAtUtc", "Status", "LatencyMs")
            SELECT "Id", "MonitorEndpointId", partition_date, "CheckedAtUtc", "Status", "LatencyMs"
            FROM monitor_samples_raw;

            DROP TABLE monitor_samples_raw CASCADE;
            """);
    }
}
