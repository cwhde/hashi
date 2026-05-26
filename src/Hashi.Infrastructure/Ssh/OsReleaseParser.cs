using Hashi.Core.Connections;

namespace Hashi.Infrastructure.Ssh;

internal static class OsReleaseParser
{
    public static (OsFamily OsFamily, string? PackageManager) Parse(string osRelease)
    {
        var id = ReadValue(osRelease, "ID") ?? ReadValue(osRelease, "ID_LIKE");
        var os = id switch
        {
            null => OsFamily.Unknown,
            var value when value.Contains("alpine", StringComparison.OrdinalIgnoreCase) => OsFamily.Alpine,
            var value when value.Contains("ubuntu", StringComparison.OrdinalIgnoreCase) => OsFamily.Ubuntu,
            var value when value.Contains("debian", StringComparison.OrdinalIgnoreCase) => OsFamily.Debian,
            _ => OsFamily.Unknown,
        };

        var packageManager = os switch
        {
            OsFamily.Alpine => "apk",
            OsFamily.Debian or OsFamily.Ubuntu => "apt",
            _ => null,
        };

        return (os, packageManager);
    }

    private static string? ReadValue(string content, string key)
    {
        foreach (var line in content.Split('\n', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries))
        {
            if (!line.StartsWith($"{key}=", StringComparison.Ordinal))
            {
                continue;
            }

            return line[(key.Length + 1)..].Trim('"');
        }

        return null;
    }
}
