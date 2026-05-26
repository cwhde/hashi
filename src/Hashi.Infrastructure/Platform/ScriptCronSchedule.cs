using Cronos;

namespace Hashi.Infrastructure.Platform;

internal static class ScriptCronSchedule
{
    public static bool IsDue(string cronExpression, DateTimeOffset? lastRunAtUtc, DateTimeOffset nowUtc)
    {
        if (string.IsNullOrWhiteSpace(cronExpression))
        {
            return false;
        }

        try
        {
            var expression = CronExpression.Parse(cronExpression, CronFormat.Standard);
            var from = (lastRunAtUtc ?? nowUtc.AddMinutes(-2)).UtcDateTime;
            var next = expression.GetNextOccurrence(from, TimeZoneInfo.Utc, inclusive: false);
            return next.HasValue && next.Value <= nowUtc.UtcDateTime;
        }
        catch (CronFormatException)
        {
            return false;
        }
    }
}
