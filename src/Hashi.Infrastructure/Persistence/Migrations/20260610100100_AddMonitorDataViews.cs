using Microsoft.EntityFrameworkCore.Migrations;

namespace Hashi.Infrastructure.Persistence.Migrations;

public partial class AddMonitorDataViews : Migration
{
    private const string View60MinuteBar = @"
CREATE OR REPLACE VIEW monitor_view_60_minute_bar AS
SELECT
    me.Id AS endpoint_id,
    me.Name AS endpoint_name,
    mr.BucketStartUtc,
    CASE WHEN mr.UpCount >= mr.DownCount THEN true ELSE false END AS is_up
FROM monitor_endpoints me
JOIN monitor_rollups mr ON mr.MonitorEndpointId = me.Id
WHERE mr.IntervalMinutes = 1
  AND mr.BucketStartUtc >= now() - interval '60 minutes';";

    private const string ViewLatencyUptime = @"
CREATE OR REPLACE VIEW monitor_view_latency_uptime AS
SELECT
    me.Id AS endpoint_id,
    me.Name AS endpoint_name,
    mr.IntervalMinutes,
    date_trunc('hour', mr.BucketStartUtc) AS bucket_hour,
    SUM(mr.UpCount) AS total_up,
    SUM(mr.DownCount) AS total_down,
    AVG(mr.AverageLatencyMs) AS avg_latency_ms,
    CASE WHEN SUM(mr.UpCount) + SUM(mr.DownCount) > 0
         THEN ROUND(SUM(mr.UpCount)::numeric / (SUM(mr.UpCount) + SUM(mr.DownCount)) * 100, 2)
         ELSE 0 END AS uptime_percent
FROM monitor_endpoints me
JOIN monitor_rollups mr ON mr.MonitorEndpointId = me.Id
GROUP BY me.Id, me.Name, mr.IntervalMinutes, date_trunc('hour', mr.BucketStartUtc);";

    private const string ViewEventTimeline = @"
CREATE OR REPLACE VIEW monitor_view_event_timeline AS
SELECT
    me.Id AS endpoint_id,
    me.Name AS endpoint_name,
    mev.Id AS event_id,
    mev.PreviousStatus,
    mev.NewStatus,
    mev.LatencyMs,
    mev.OccurredAtUtc
FROM monitor_endpoints me
JOIN monitor_events mev ON mev.MonitorEndpointId = me.Id
ORDER BY mev.OccurredAtUtc DESC;";

    protected override void Up(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.Sql(View60MinuteBar);
        migrationBuilder.Sql(ViewLatencyUptime);
        migrationBuilder.Sql(ViewEventTimeline);
    }

    protected override void Down(MigrationBuilder migrationBuilder)
    {
        migrationBuilder.Sql("DROP VIEW IF EXISTS monitor_view_60_minute_bar");
        migrationBuilder.Sql("DROP VIEW IF EXISTS monitor_view_latency_uptime");
        migrationBuilder.Sql("DROP VIEW IF EXISTS monitor_view_event_timeline");
    }
}
