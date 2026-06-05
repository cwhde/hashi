using System.Text;
using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Connections;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Connections;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Platform;

public sealed class AccessLogIngestWorker(
    IServiceScopeFactory scopeFactory,
    ILogger<AccessLogIngestWorker> logger) : BackgroundService
{
    private const string DefaultAccessLogPath = "/var/log/hashi/traefik/access.log";
    private const int IntervalSeconds = 60;

    public async Task<AccessLogIngestRunResult> ProcessOnceAsync(CancellationToken cancellationToken = default)
    {
        using var scope = scopeFactory.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var secrets = scope.ServiceProvider.GetRequiredService<SecretRecordService>();
        var targets = scope.ServiceProvider.GetRequiredService<ConnectionTargetResolver>();
        var ssh = scope.ServiceProvider.GetRequiredService<ISshRemoteExecutor>();
        var security = scope.ServiceProvider.GetRequiredService<SecurityIngestionService>();
        var connections = await db.Connections
            .Where(x => x.Type == ConnectionTypeNames.TraefikHost && x.Enabled)
            .ToListAsync(cancellationToken);

        if (connections.Count == 0)
        {
            return new AccessLogIngestRunResult(0, 0, 0, 0);
        }

        var connectionIds = connections.Select(x => x.Id).ToList();
        var cursors = await db.AccessLogCursors
            .Where(x => connectionIds.Contains(x.ConnectionId))
            .ToDictionaryAsync(x => x.ConnectionId, cancellationToken);

        var hostsProcessed = 0;
        var linesProcessed = 0;
        var linesSkipped = 0;
        var hostErrors = 0;

        foreach (var connection in connections)
        {
            var credentials = await ConnectionSshCredentialResolver.ResolveAsync(
                connection,
                secrets,
                targets,
                cancellationToken: cancellationToken);
            if (credentials is null)
            {
                hostErrors++;
                logger.LogWarning("Access log ingest skipped {ConnectionId}: missing SSH credentials.", connection.Id);
                continue;
            }

            var logPath = ResolveAccessLogPath(connection.SettingsJson);
            var read = await ssh.ReadFileAsync(
                credentials.Settings,
                credentials.AuthMode,
                credentials.Password,
                credentials.PrivateKeyPem,
                credentials.PrivateKeyPassphrase,
                logPath,
                cancellationToken);

            if (!read.Succeeded || read.Content is null)
            {
                hostErrors++;
                logger.LogWarning(
                    "Access log ingest failed for {ConnectionId} ({Path}): {Error}",
                    connection.Id,
                    logPath,
                    read.Error ?? "unknown read failure");
                continue;
            }

            var bytes = read.Content;
            var cursor = cursors.GetValueOrDefault(connection.Id);
            var previousOffset = cursor?.ByteOffset ?? 0;
            if (previousOffset < 0 || previousOffset > bytes.LongLength)
            {
                previousOffset = 0;
            }

            var segment = bytes[(int)previousOffset..];
            var consumed = ConsumeCompleteLines(segment, out var lines);
            foreach (var line in lines)
            {
                if (!TryParseTraefikJsonLine(line, out var request))
                {
                    linesSkipped++;
                    continue;
                }

                await security.IngestAccessLogAsync(request, cancellationToken);
                linesProcessed++;
            }

            var newOffset = previousOffset + consumed;
            if (cursor is null)
            {
                cursor = new AccessLogCursorEntity
                {
                    ConnectionId = connection.Id,
                    ByteOffset = newOffset,
                    UpdatedAtUtc = DateTimeOffset.UtcNow,
                };
                db.AccessLogCursors.Add(cursor);
                cursors[connection.Id] = cursor;
            }
            else
            {
                cursor.ByteOffset = newOffset;
                cursor.UpdatedAtUtc = DateTimeOffset.UtcNow;
            }

            hostsProcessed++;
        }

        await db.SaveChangesAsync(cancellationToken);
        return new AccessLogIngestRunResult(hostsProcessed, linesProcessed, linesSkipped, hostErrors);
    }

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                using var scope = scopeFactory.CreateScope();
                var jobs = scope.ServiceProvider.GetRequiredService<BackgroundJobService>();
                await jobs.BeginRunAsync(BackgroundJobKeys.AccessLogIngest, stoppingToken);
                var result = await ProcessOnceAsync(stoppingToken);
                var succeeded = result.HostErrors == 0;
                await jobs.CompleteRunAsync(
                    BackgroundJobKeys.AccessLogIngest,
                    succeeded,
                    $"Processed {result.LinesProcessed} access-log line(s) from {result.HostsProcessed} host(s); skipped {result.LinesSkipped}; host errors {result.HostErrors}.",
                    succeeded ? null : $"Encountered {result.HostErrors} host ingestion error(s).",
                    IntervalSeconds,
                    stoppingToken);
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "Access log ingestion worker failed.");
                try
                {
                    using var scope = scopeFactory.CreateScope();
                    var jobs = scope.ServiceProvider.GetRequiredService<BackgroundJobService>();
                    await jobs.CompleteRunAsync(
                        BackgroundJobKeys.AccessLogIngest,
                        false,
                        null,
                        ex.Message,
                        IntervalSeconds,
                        stoppingToken);
                }
                catch
                {
                }
            }

            await Task.Delay(TimeSpan.FromSeconds(IntervalSeconds), stoppingToken);
        }
    }

    private static string ResolveAccessLogPath(string settingsJson)
    {
        try
        {
            using var document = JsonDocument.Parse(settingsJson);
            var root = document.RootElement;
            if (root.TryGetProperty("AccessLogPath", out var accessLogPath) && !string.IsNullOrWhiteSpace(accessLogPath.GetString()))
            {
                return accessLogPath.GetString()!;
            }

            if (root.TryGetProperty("accessLogPath", out var accessLogPathLower) && !string.IsNullOrWhiteSpace(accessLogPathLower.GetString()))
            {
                return accessLogPathLower.GetString()!;
            }
        }
        catch
        {
        }

        return DefaultAccessLogPath;
    }

    private static long ConsumeCompleteLines(byte[] segment, out List<string> lines)
    {
        lines = [];
        var lastNewline = Array.LastIndexOf(segment, (byte)'\n');
        if (lastNewline < 0)
        {
            return 0;
        }

        var complete = Encoding.UTF8.GetString(segment, 0, lastNewline + 1);
        foreach (var raw in complete.Split('\n', StringSplitOptions.RemoveEmptyEntries))
        {
            var line = raw.TrimEnd('\r');
            if (!string.IsNullOrWhiteSpace(line))
            {
                lines.Add(line);
            }
        }

        return lastNewline + 1;
    }

    private static bool TryParseTraefikJsonLine(string line, out AccessLogIngestRequest request)
    {
        request = default!;
        try
        {
            using var document = JsonDocument.Parse(line);
            var root = document.RootElement;

            var client = GetString(root, "ClientHost");
            if (string.IsNullOrWhiteSpace(client))
            {
                var clientAddr = GetString(root, "ClientAddr");
                client = NormalizeClientAddress(clientAddr);
            }

            if (string.IsNullOrWhiteSpace(client))
            {
                return false;
            }

            var host = GetString(root, "RequestHost")
                ?? GetString(root, "Host")
                ?? string.Empty;
            var path = GetString(root, "RequestPath")
                ?? GetString(root, "Path")
                ?? "/";
            var statusCode = GetInt(root, "DownstreamStatus")
                ?? GetInt(root, "Status")
                ?? 0;
            if (statusCode <= 0)
            {
                return false;
            }

            var country = GetString(root, "CountryCode");
            var asn = GetString(root, "Asn");
            var requestId = GetString(root, "RequestID")
                ?? GetString(root, "RequestId")
                ?? GetString(root, "X-Request-Id")
                ?? GetRequestHeader(root, "X-Request-Id", "X-Request-ID", "X-Correlation-Id", "Request-Id");
            var userAgent = GetString(root, "UserAgent")
                ?? GetString(root, "User-Agent")
                ?? GetString(root, "RequestUserAgent")
                ?? GetRequestHeader(root, "User-Agent");
            var method = GetString(root, "RequestMethod")
                ?? GetString(root, "Method");

            request = new AccessLogIngestRequest(
                client,
                host,
                path,
                statusCode,
                country,
                asn,
                Method: method,
                RequestId: requestId,
                UserAgent: userAgent);
            return true;
        }
        catch
        {
            return false;
        }
    }

    private static string? NormalizeClientAddress(string? value)
    {
        if (string.IsNullOrWhiteSpace(value))
        {
            return null;
        }

        if (value.StartsWith("[", StringComparison.Ordinal))
        {
            var end = value.IndexOf(']');
            return end > 1 ? value[1..end] : value;
        }

        var firstColon = value.IndexOf(':');
        var lastColon = value.LastIndexOf(':');
        if (firstColon > 0 && firstColon == lastColon)
        {
            return value[..firstColon];
        }

        return value;
    }

    private static string? GetString(JsonElement root, string propertyName)
        => root.TryGetProperty(propertyName, out var property) && property.ValueKind == JsonValueKind.String
            ? property.GetString()
            : null;

    private static string? GetRequestHeader(JsonElement root, params string[] headerNames)
    {
        foreach (var containerName in new[] { "RequestHeaders", "RequestHeader", "Headers" })
        {
            if (!root.TryGetProperty(containerName, out var container) || container.ValueKind != JsonValueKind.Object)
            {
                continue;
            }

            foreach (var property in container.EnumerateObject())
            {
                if (headerNames.Any(name => string.Equals(name, property.Name, StringComparison.OrdinalIgnoreCase))
                    && property.Value.ValueKind == JsonValueKind.String)
                {
                    return property.Value.GetString();
                }
            }
        }

        return null;
    }

    private static int? GetInt(JsonElement root, string propertyName)
    {
        if (!root.TryGetProperty(propertyName, out var property))
        {
            return null;
        }

        return property.ValueKind switch
        {
            JsonValueKind.Number when property.TryGetInt32(out var intValue) => intValue,
            JsonValueKind.String when int.TryParse(property.GetString(), out var parsed) => parsed,
            _ => null,
        };
    }
}

public sealed record AccessLogIngestRunResult(
    int HostsProcessed,
    int LinesProcessed,
    int LinesSkipped,
    int HostErrors);
