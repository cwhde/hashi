using System.Globalization;
using System.Net;
using System.Net.Http.Headers;
using System.Net.Sockets;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Platform;

public sealed class BlocklistSourceManagementService(
    HashiDbContext db,
    BlocklistSafeHttpFetcher fetcher,
    BlocklistParser parser,
    AuditService audit,
    ILogger<BlocklistSourceManagementService> logger)
{
    public async Task<IReadOnlyList<BlocklistSourceResponse>> ListAsync(CancellationToken cancellationToken = default)
    {
        await EnsureRecommendedSourcesAsync(cancellationToken);
        var sources = await db.BlocklistSources.AsNoTracking()
            .OrderByDescending(x => RecommendedMetadata.IsRecommended(x.MetadataJson))
            .ThenBy(x => x.Name)
            .ToListAsync(cancellationToken);
        return await ToResponsesAsync(sources, cancellationToken);
    }

    public async Task<BlocklistSourceResponse?> GetAsync(Guid sourceId, CancellationToken cancellationToken = default)
    {
        await EnsureRecommendedSourcesAsync(cancellationToken);
        var source = await db.BlocklistSources.AsNoTracking().SingleOrDefaultAsync(x => x.Id == sourceId, cancellationToken);
        return source is null ? null : await ToResponseAsync(source, cancellationToken);
    }

    public async Task<BlocklistSourceResponse> CreateAsync(
        UpsertBlocklistSourceRequest request,
        CancellationToken cancellationToken = default)
    {
        var source = new BlocklistSourceEntity();
        ApplyRequest(source, request, isCreate: true);
        source.Enabled = false;
        source.MetadataJson = MergeParserMetadata(source.MetadataJson, request, recommended: false);
        await fetcher.ValidateSourceAsync(source, cancellationToken);
        db.BlocklistSources.Add(source);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("security", "blocklist_source_created", subjectType: "blocklist_source", subjectId: source.Id.ToString(), cancellationToken: cancellationToken);
        return await ToResponseAsync(source, cancellationToken);
    }

    public async Task<BlocklistSourceResponse?> UpdateAsync(
        Guid sourceId,
        UpsertBlocklistSourceRequest request,
        CancellationToken cancellationToken = default)
    {
        var source = await db.BlocklistSources.SingleOrDefaultAsync(x => x.Id == sourceId, cancellationToken);
        if (source is null)
        {
            return null;
        }

        var oldUrl = source.SourceUrl;
        var oldEnforcement = source.EnforcementMode;
        ApplyRequest(source, request, isCreate: false);
        source.MetadataJson = MergeParserMetadata(source.MetadataJson, request, RecommendedMetadata.IsRecommended(source.MetadataJson));
        await fetcher.ValidateSourceAsync(source, cancellationToken);
        source.UpdatedAtUtc = DateTimeOffset.UtcNow;
        if (!string.Equals(oldUrl, source.SourceUrl, StringComparison.OrdinalIgnoreCase)
            || !string.Equals(oldEnforcement, source.EnforcementMode, StringComparison.OrdinalIgnoreCase))
        {
            await MarkSourceEntriesForFirewallResyncAsync(source.Id, cancellationToken);
        }

        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("security", "blocklist_source_updated", subjectType: "blocklist_source", subjectId: source.Id.ToString(), cancellationToken: cancellationToken);
        return await ToResponseAsync(source, cancellationToken);
    }

    public async Task<bool> DeleteAsync(Guid sourceId, CancellationToken cancellationToken = default)
    {
        var source = await db.BlocklistSources.SingleOrDefaultAsync(x => x.Id == sourceId, cancellationToken);
        if (source is null)
        {
            return false;
        }

        var entries = await db.BlocklistEntries.Where(x => x.SourceId == sourceId).ToListAsync(cancellationToken);
        var runs = await db.BlocklistFetchRuns.Where(x => x.BlocklistSourceId == sourceId).ToListAsync(cancellationToken);
        db.BlocklistEntries.RemoveRange(entries);
        db.BlocklistFetchRuns.RemoveRange(runs);
        db.BlocklistSources.Remove(source);
        await db.SaveChangesAsync(cancellationToken);
        await audit.WriteAsync("security", "blocklist_source_deleted", subjectType: "blocklist_source", subjectId: sourceId.ToString(), cancellationToken: cancellationToken);
        return true;
    }

    public async Task<BlocklistFetchPreviewResponse?> PreviewAsync(Guid sourceId, CancellationToken cancellationToken = default)
    {
        var source = await db.BlocklistSources.AsNoTracking().SingleOrDefaultAsync(x => x.Id == sourceId, cancellationToken);
        if (source is null)
        {
            return null;
        }

        var content = await fetcher.FetchAsync(source, conditional: false, cancellationToken);
        var parse = content.NotModified
            ? BlocklistParseResult.Empty
            : parser.Parse(source, content.Content ?? string.Empty);
        return ToPreviewResponse(source, parse, content);
    }

    public async Task<BlocklistSourceMutationResponse?> EnableAsync(Guid sourceId, CancellationToken cancellationToken = default)
    {
        var source = await db.BlocklistSources.SingleOrDefaultAsync(x => x.Id == sourceId, cancellationToken);
        if (source is null)
        {
            return null;
        }

        source.Enabled = true;
        source.UpdatedAtUtc = DateTimeOffset.UtcNow;
        var run = await RefreshCoreAsync(source, requireEnabled: false, cancellationToken);
        var pending = await PendingFirewallEntryCountAsync(cancellationToken);
        await audit.WriteAsync("security", "blocklist_source_enabled", subjectType: "blocklist_source", subjectId: source.Id.ToString(), cancellationToken: cancellationToken);
        return new BlocklistSourceMutationResponse(
            await ToResponseAsync(source, cancellationToken),
            run,
            null,
            FirewallSyncRecommended(source),
            pending,
            BuildSourceWarnings(source));
    }

    public async Task<BlocklistSourceMutationResponse?> DisableAsync(Guid sourceId, CancellationToken cancellationToken = default)
    {
        var source = await db.BlocklistSources.SingleOrDefaultAsync(x => x.Id == sourceId, cancellationToken);
        if (source is null)
        {
            return null;
        }

        source.Enabled = false;
        source.LastFetchStatus = BlocklistFetchStatusNames.Succeeded;
        source.UpdatedAtUtc = DateTimeOffset.UtcNow;
        var entries = await db.BlocklistEntries.Where(x => x.SourceId == sourceId).ToListAsync(cancellationToken);
        foreach (var entry in entries)
        {
            entry.Enabled = false;
            entry.SyncedToFirewall = false;
        }

        await db.SaveChangesAsync(cancellationToken);
        var pending = await PendingFirewallEntryCountAsync(cancellationToken);
        await audit.WriteAsync("security", "blocklist_source_disabled", subjectType: "blocklist_source", subjectId: source.Id.ToString(), cancellationToken: cancellationToken);
        return new BlocklistSourceMutationResponse(
            await ToResponseAsync(source, cancellationToken),
            null,
            null,
            FirewallSyncRecommended(source),
            pending,
            ["Firewall sync preview/apply is recommended when a firewall-enforced source is disabled."]);
    }

    public async Task<BlocklistSourceMutationResponse?> RefreshAsync(Guid sourceId, CancellationToken cancellationToken = default)
    {
        var source = await db.BlocklistSources.SingleOrDefaultAsync(x => x.Id == sourceId, cancellationToken);
        if (source is null)
        {
            return null;
        }

        var run = await RefreshCoreAsync(source, requireEnabled: false, cancellationToken);
        var pending = await PendingFirewallEntryCountAsync(cancellationToken);
        return new BlocklistSourceMutationResponse(
            await ToResponseAsync(source, cancellationToken),
            run,
            null,
            FirewallSyncRecommended(source),
            pending,
            BuildSourceWarnings(source));
    }

    public async Task<IReadOnlyList<BlocklistFetchRunResponse>> ListRunsAsync(Guid sourceId, CancellationToken cancellationToken = default)
        => await db.BlocklistFetchRuns.AsNoTracking()
            .Where(x => x.BlocklistSourceId == sourceId)
            .OrderByDescending(x => x.StartedAtUtc)
            .Take(50)
            .Select(x => ToRunResponse(x))
            .ToListAsync(cancellationToken);

    public async Task<IReadOnlyList<BlocklistEntryResponse>> ListEntriesAsync(Guid sourceId, CancellationToken cancellationToken = default)
        => await db.BlocklistEntries.AsNoTracking()
            .Where(x => x.SourceId == sourceId)
            .OrderBy(x => x.NormalizedValue)
            .Take(500)
            .Select(x => ToEntryResponse(x))
            .ToListAsync(cancellationToken);

    private async Task<BlocklistFetchRunResponse> RefreshCoreAsync(
        BlocklistSourceEntity source,
        bool requireEnabled,
        CancellationToken cancellationToken)
    {
        if (requireEnabled && !source.Enabled)
        {
            throw new InvalidOperationException("Blocklist source is disabled.");
        }

        var run = new BlocklistFetchRunEntity
        {
            BlocklistSourceId = source.Id,
            StartedAtUtc = DateTimeOffset.UtcNow,
            Status = BlocklistFetchStatusNames.Running,
        };
        db.BlocklistFetchRuns.Add(run);
        await db.SaveChangesAsync(cancellationToken);

        try
        {
            var fetched = await fetcher.FetchAsync(source, conditional: true, cancellationToken);
            run.HttpStatusCode = fetched.HttpStatusCode;
            run.ETag = fetched.ETag;
            run.LastModified = fetched.LastModified;
            run.ContentHash = fetched.ContentHash;
            if (fetched.NotModified)
            {
                run.Status = BlocklistFetchStatusNames.SkippedNotModified;
                run.CompletedAtUtc = DateTimeOffset.UtcNow;
                source.LastFetchStatus = BlocklistFetchStatusNames.SkippedNotModified;
                source.LastFetchError = null;
                source.LastFetchedAtUtc = run.CompletedAtUtc;
                await db.SaveChangesAsync(cancellationToken);
                return ToRunResponse(run);
            }

            var parse = parser.Parse(source, fetched.Content ?? string.Empty);
            if (parse.Entries.Count == 0 && parse.Errors.Count > 0)
            {
                throw new InvalidOperationException("No valid blocklist entries were parsed.");
            }

            var oldEntries = await db.BlocklistEntries.Where(x => x.SourceId == source.Id).ToListAsync(cancellationToken);
            var oldValues = oldEntries.Select(x => x.NormalizedValue).ToHashSet(StringComparer.OrdinalIgnoreCase);
            var newValues = parse.Entries.Select(x => x.NormalizedValue).ToHashSet(StringComparer.OrdinalIgnoreCase);
            db.BlocklistEntries.RemoveRange(oldEntries);
            db.BlocklistEntries.AddRange(parse.Entries.Select(entry => ToEntity(source, entry)));

            run.Status = BlocklistFetchStatusNames.Succeeded;
            run.CompletedAtUtc = DateTimeOffset.UtcNow;
            run.EntryCount = parse.Entries.Count;
            run.AddedCount = newValues.Count(x => !oldValues.Contains(x));
            run.RemovedCount = oldValues.Count(x => !newValues.Contains(x));
            run.UnchangedCount = newValues.Count(x => oldValues.Contains(x));
            run.MetadataJson = JsonSerializer.Serialize(new { parse.IgnoredCount, errors = parse.Errors.Take(25).ToArray() });
            source.ETag = fetched.ETag;
            source.LastModified = fetched.LastModified;
            source.LastContentHash = fetched.ContentHash;
            source.LastFetchedAtUtc = run.CompletedAtUtc;
            source.LastFetchStatus = BlocklistFetchStatusNames.Succeeded;
            source.LastFetchError = null;
            source.UpdatedAtUtc = DateTimeOffset.UtcNow;
            await db.SaveChangesAsync(cancellationToken);
            await audit.WriteAsync(
                "security",
                "blocklist_source_refreshed",
                subjectType: "blocklist_source",
                subjectId: source.Id.ToString(),
                metadata: new { run.EntryCount, run.AddedCount, run.RemovedCount },
                cancellationToken: cancellationToken);
        }
        catch (Exception ex)
        {
            logger.LogWarning(ex, "Blocklist source {SourceId} refresh failed; preserving last known good entries.", source.Id);
            run.Status = BlocklistFetchStatusNames.Failed;
            run.CompletedAtUtc = DateTimeOffset.UtcNow;
            run.Error = ex.Message;
            source.LastFetchStatus = BlocklistFetchStatusNames.Failed;
            source.LastFetchError = ex.Message;
            source.UpdatedAtUtc = DateTimeOffset.UtcNow;
            await db.SaveChangesAsync(cancellationToken);
        }

        return ToRunResponse(run);
    }

    private BlocklistEntryEntity ToEntity(BlocklistSourceEntity source, BlocklistParsedEntry entry)
        => new()
        {
            SourceId = source.Id,
            ClientIp = entry.SubjectType == SecuritySubjectTypeNames.Ip ? entry.NormalizedValue : string.Empty,
            Scope = BlocklistScopeNames.Global,
            Type = entry.SubjectType == SecuritySubjectTypeNames.Cidr ? BlocklistTypeNames.Cidr : BlocklistTypeNames.Ip,
            Value = entry.NormalizedValue,
            NormalizedValue = entry.NormalizedValue,
            SubjectType = entry.SubjectType,
            Source = BlocklistSourceNames.Automatic,
            Reason = source.Name,
            Enabled = source.Enabled,
            EnforcementMode = source.EnforcementMode,
            MetadataJson = entry.MetadataJson ?? "{}",
            CreatedBy = "hashi:blocklist-source",
            SyncedToFirewall = !FirewallSyncRecommended(source),
        };

    private async Task EnsureRecommendedSourcesAsync(CancellationToken cancellationToken)
    {
        var existingUrls = await db.BlocklistSources
            .Select(x => x.SourceUrl)
            .ToListAsync(cancellationToken);
        foreach (var seed in RecommendedBlocklistSources.All)
        {
            if (existingUrls.Contains(seed.SourceUrl, StringComparer.OrdinalIgnoreCase))
            {
                continue;
            }

            db.BlocklistSources.Add(seed.ToEntity());
        }

        await db.SaveChangesAsync(cancellationToken);
    }

    private static void ApplyRequest(BlocklistSourceEntity source, UpsertBlocklistSourceRequest request, bool isCreate)
    {
        source.Name = request.Name.Trim();
        source.SourceUrl = request.SourceUrl.Trim();
        source.Description = (request.Description ?? string.Empty).Trim();
        source.Format = NormalizeFormat(request.Format);
        source.EnforcementMode = NormalizeEnforcement(request.EnforcementMode);
        source.CanFirewallEnforce = request.CanFirewallEnforce;
        source.AllowHttp = request.AllowHttp;
        source.RefreshIntervalHours = Math.Clamp(request.RefreshIntervalHours ?? source.RefreshIntervalHours, 1, 720);
        if (!isCreate)
        {
            source.Enabled = request.Enabled;
        }
    }

    private static string NormalizeFormat(string? format)
        => (format ?? string.Empty).Trim().ToLowerInvariant() switch
        {
            "csv" => BlocklistSourceFormatNames.Csv,
            "tsv" => BlocklistSourceFormatNames.Tsv,
            "json_lines" or "jsonl" or "ndjson" => BlocklistSourceFormatNames.JsonLines,
            "json" => BlocklistSourceFormatNames.Json,
            "netset" or "ipset" or "firehol" => BlocklistSourceFormatNames.Netset,
            _ => BlocklistSourceFormatNames.Text,
        };

    private static string NormalizeEnforcement(string? enforcement)
        => (enforcement ?? string.Empty).Trim().ToLowerInvariant() switch
        {
            BlocklistEnforcementModeNames.Observe => BlocklistEnforcementModeNames.Observe,
            BlocklistEnforcementModeNames.Firewall => BlocklistEnforcementModeNames.Firewall,
            _ => BlocklistEnforcementModeNames.Middleware,
        };

    private static string MergeParserMetadata(
        string metadataJson,
        UpsertBlocklistSourceRequest request,
        bool recommended)
    {
        var metadata = RecommendedMetadata.Read(metadataJson);
        metadata["recommended"] = recommended;
        metadata["parser"] = new Dictionary<string, object?>
        {
            ["csvColumnIndex"] = request.CsvColumnIndex,
            ["jsonArrayField"] = request.JsonArrayField,
            ["jsonValueField"] = request.JsonValueField,
        };
        return JsonSerializer.Serialize(metadata);
    }

    private static bool FirewallSyncRecommended(BlocklistSourceEntity source)
        => source.CanFirewallEnforce
           && string.Equals(source.EnforcementMode, BlocklistEnforcementModeNames.Firewall, StringComparison.OrdinalIgnoreCase);

    private async Task MarkSourceEntriesForFirewallResyncAsync(Guid sourceId, CancellationToken cancellationToken)
    {
        var entries = await db.BlocklistEntries.Where(x => x.SourceId == sourceId).ToListAsync(cancellationToken);
        foreach (var entry in entries)
        {
            entry.SyncedToFirewall = false;
        }
    }

    private async Task<int> PendingFirewallEntryCountAsync(CancellationToken cancellationToken)
    {
        var now = DateTimeOffset.UtcNow;
        return await db.BlocklistEntries.AsNoTracking()
            .Where(x => x.Enabled)
            .Where(x => x.EnforcementMode == BlocklistEnforcementModeNames.Firewall)
            .Where(x => x.ExpiresAtUtc == null || x.ExpiresAtUtc > now)
            .Where(x => x.Type == BlocklistTypeNames.Ip || x.Type == BlocklistTypeNames.Cidr || x.Type == string.Empty)
            .CountAsync(x => !x.SyncedToFirewall, cancellationToken);
    }

    private async Task<IReadOnlyList<BlocklistSourceResponse>> ToResponsesAsync(
        IReadOnlyList<BlocklistSourceEntity> sources,
        CancellationToken cancellationToken)
    {
        var responses = new List<BlocklistSourceResponse>();
        foreach (var source in sources)
        {
            responses.Add(await ToResponseAsync(source, cancellationToken));
        }

        return responses;
    }

    private async Task<BlocklistSourceResponse> ToResponseAsync(BlocklistSourceEntity source, CancellationToken cancellationToken)
    {
        var entryCount = await db.BlocklistEntries.AsNoTracking()
            .LongCountAsync(x => x.SourceId == source.Id && x.Enabled, cancellationToken);
        var staleAfter = source.LastFetchedAtUtc?.AddHours(Math.Max(1, source.RefreshIntervalHours) * 2);
        return new BlocklistSourceResponse(
            source.Id,
            source.Name,
            source.SourceUrl,
            source.Description,
            source.Format,
            source.EnforcementMode,
            source.CanFirewallEnforce,
            source.Enabled,
            source.AllowHttp,
            source.RefreshIntervalHours,
            source.LastFetchStatus,
            source.LastFetchError,
            source.LastFetchedAtUtc,
            entryCount,
            source.Enabled && staleAfter is not null && staleAfter < DateTimeOffset.UtcNow,
            source.MetadataJson,
            source.CreatedAtUtc,
            source.UpdatedAtUtc);
    }

    private static BlocklistFetchPreviewResponse ToPreviewResponse(
        BlocklistSourceEntity source,
        BlocklistParseResult parse,
        BlocklistFetchedContent content)
        => new(
            source.Id,
            source.Name,
            parse.Entries.Count,
            parse.IgnoredCount,
            parse.Errors.Count,
            content.ContentHash,
            content.NotModified,
            parse.Entries.Take(25).Select(x => new BlocklistPreviewEntryResponse(
                x.SubjectType,
                x.Value,
                x.NormalizedValue,
                x.LineNumber)).ToList(),
            parse.Errors.Take(25).ToList(),
            BuildSourceWarnings(source));

    private static IReadOnlyList<string> BuildSourceWarnings(BlocklistSourceEntity source)
    {
        var warnings = new List<string>
        {
            "Third-party blocklists can create false positives and may block legitimate traffic.",
        };
        if (source.AllowHttp)
        {
            warnings.Add("HTTP fetching is enabled for this source; HTTPS with certificate validation is safer.");
        }

        if (FirewallSyncRecommended(source))
        {
            warnings.Add("Firewall-enforced changes require a firewall sync preview/apply path.");
        }

        return warnings;
    }

    private static BlocklistFetchRunResponse ToRunResponse(BlocklistFetchRunEntity run)
        => new(
            run.Id,
            run.BlocklistSourceId,
            run.StartedAtUtc,
            run.CompletedAtUtc,
            run.Status,
            run.HttpStatusCode,
            run.EntryCount,
            run.AddedCount,
            run.RemovedCount,
            run.UnchangedCount,
            run.ContentHash,
            run.Error);

    private static BlocklistEntryResponse ToEntryResponse(BlocklistEntryEntity entry)
        => new(
            entry.Id,
            entry.SourceId,
            entry.SubjectType,
            entry.Value,
            entry.NormalizedValue,
            entry.Scope,
            entry.Type,
            entry.Reason,
            entry.Source,
            entry.Enabled,
            entry.EnforcementMode,
            entry.SyncedToFirewall,
            entry.CreatedAtUtc,
            entry.ExpiresAtUtc,
            entry.LastHitAtUtc,
            entry.MetadataJson);
}

public sealed class BlocklistParser
{
    public BlocklistParseResult Parse(BlocklistSourceEntity source, string content)
    {
        var metadata = RecommendedMetadata.Read(source.MetadataJson);
        var parserOptions = ParserOptions.FromMetadata(metadata);
        return source.Format switch
        {
            BlocklistSourceFormatNames.Csv => ParseDelimited(content, ',', parserOptions),
            BlocklistSourceFormatNames.Tsv => ParseDelimited(content, '\t', parserOptions),
            BlocklistSourceFormatNames.Json => ParseJson(content, parserOptions),
            BlocklistSourceFormatNames.JsonLines => ParseJsonLines(content, parserOptions),
            _ => ParseLines(content),
        };
    }

    private static BlocklistParseResult ParseLines(string content)
    {
        var result = new MutableParseResult();
        var lineNumber = 0;
        foreach (var rawLine in SplitLines(content))
        {
            lineNumber++;
            var line = StripComment(rawLine);
            if (string.IsNullOrWhiteSpace(line))
            {
                result.IgnoredCount++;
                continue;
            }

            var token = line.Split((char[]?)null, StringSplitOptions.RemoveEmptyEntries)
                .FirstOrDefault(x => x.Contains('.') || x.Contains(':'));
            result.AddToken(token, lineNumber);
        }

        return result.ToResult();
    }

    private static BlocklistParseResult ParseDelimited(string content, char delimiter, ParserOptions options)
    {
        var result = new MutableParseResult();
        var lineNumber = 0;
        foreach (var rawLine in SplitLines(content))
        {
            lineNumber++;
            var line = StripComment(rawLine);
            if (string.IsNullOrWhiteSpace(line))
            {
                result.IgnoredCount++;
                continue;
            }

            var columns = SplitDelimited(line, delimiter);
            if (options.ValueColumnIndex >= columns.Count)
            {
                result.Errors.Add($"Line {lineNumber}: missing value column {options.ValueColumnIndex}.");
                continue;
            }

            var value = columns[options.ValueColumnIndex].Trim();
            if (options.CidrPrefixColumnIndex is int prefixIndex && prefixIndex < columns.Count)
            {
                value = $"{value}/{columns[prefixIndex].Trim()}";
            }

            result.AddToken(value, lineNumber);
        }

        return result.ToResult();
    }

    private static BlocklistParseResult ParseJson(string content, ParserOptions options)
    {
        try
        {
            using var doc = JsonDocument.Parse(content);
            var root = doc.RootElement;
            if (root.ValueKind == JsonValueKind.Array)
            {
                return ParseJsonArray(root, options);
            }

            if (root.ValueKind == JsonValueKind.Object)
            {
                if (!string.IsNullOrWhiteSpace(options.JsonArrayField)
                    && root.TryGetProperty(options.JsonArrayField, out var array)
                    && array.ValueKind == JsonValueKind.Array)
                {
                    return ParseJsonArray(array, options);
                }

                var result = new MutableParseResult();
                result.AddToken(ReadJsonValue(root, options), null);
                return result.ToResult();
            }
        }
        catch (JsonException)
        {
            return ParseJsonLines(content, options);
        }

        return new MutableParseResult { Errors = { "JSON content must be an array, an object array field, or JSON lines." } }.ToResult();
    }

    private static BlocklistParseResult ParseJsonLines(string content, ParserOptions options)
    {
        var result = new MutableParseResult();
        var lineNumber = 0;
        foreach (var rawLine in SplitLines(content))
        {
            lineNumber++;
            var line = rawLine.Trim();
            if (string.IsNullOrWhiteSpace(line))
            {
                result.IgnoredCount++;
                continue;
            }

            try
            {
                using var doc = JsonDocument.Parse(line);
                result.AddToken(ReadJsonValue(doc.RootElement, options), lineNumber);
            }
            catch (JsonException ex)
            {
                result.Errors.Add($"Line {lineNumber}: invalid JSON object ({ex.Message}).");
            }
        }

        return result.ToResult();
    }

    private static BlocklistParseResult ParseJsonArray(JsonElement array, ParserOptions options)
    {
        var result = new MutableParseResult();
        var index = 0;
        foreach (var item in array.EnumerateArray())
        {
            index++;
            result.AddToken(item.ValueKind == JsonValueKind.String ? item.GetString() : ReadJsonValue(item, options), index);
        }

        return result.ToResult();
    }

    private static string? ReadJsonValue(JsonElement element, ParserOptions options)
    {
        if (element.ValueKind == JsonValueKind.String)
        {
            return element.GetString();
        }

        if (element.ValueKind != JsonValueKind.Object)
        {
            return null;
        }

        foreach (var field in options.JsonValueFields)
        {
            if (element.TryGetProperty(field, out var value) && value.ValueKind == JsonValueKind.String)
            {
                return value.GetString();
            }
        }

        return null;
    }

    private static IReadOnlyList<string> SplitDelimited(string line, char delimiter)
        => delimiter == '\t'
            ? line.Split('\t', StringSplitOptions.TrimEntries).ToList()
            : line.Split(',', StringSplitOptions.TrimEntries).Select(x => x.Trim('"')).ToList();

    private static string StripComment(string line)
    {
        var trimmed = line.Trim();
        if (trimmed.StartsWith("#", StringComparison.Ordinal)
            || trimmed.StartsWith(";", StringComparison.Ordinal))
        {
            return string.Empty;
        }

        var index = trimmed.IndexOf('#', StringComparison.Ordinal);
        return index >= 0 ? trimmed[..index].Trim() : trimmed;
    }

    private static IEnumerable<string> SplitLines(string content)
        => content.Replace("\r\n", "\n", StringComparison.Ordinal).Replace('\r', '\n').Split('\n');
}

public sealed record BlocklistParsedEntry(
    string SubjectType,
    string Value,
    string NormalizedValue,
    int? LineNumber,
    string? MetadataJson);

public sealed record BlocklistParseResult(
    IReadOnlyList<BlocklistParsedEntry> Entries,
    int IgnoredCount,
    IReadOnlyList<string> Errors)
{
    public static BlocklistParseResult Empty { get; } = new([], 0, []);
}

public sealed class BlocklistSafeHttpFetcher(
    IBlocklistDnsResolver resolver,
    IBlocklistHttpTransport transport)
{
    private const string UserAgent = "Hashi/2.0 blocklist-fetcher (+https://github.com/hashi)";

    public async Task ValidateSourceAsync(BlocklistSourceEntity source, CancellationToken cancellationToken = default)
    {
        var uri = ParseAndValidateUri(source.SourceUrl, source.AllowHttp);
        await ValidateResolvedHostAsync(uri.Host, resolver, cancellationToken);
    }

    public async Task<BlocklistFetchedContent> FetchAsync(
        BlocklistSourceEntity source,
        bool conditional,
        CancellationToken cancellationToken = default)
    {
        var current = ParseAndValidateUri(source.SourceUrl, source.AllowHttp);
        await ValidateResolvedHostAsync(current.Host, resolver, cancellationToken);
        var redirects = new List<Uri>();
        for (var redirectCount = 0; redirectCount <= source.MaxRedirects; redirectCount++)
        {
            var headers = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
            {
                ["User-Agent"] = UserAgent,
                ["Accept"] = "text/plain, application/json;q=0.9, */*;q=0.1",
            };
            if (conditional)
            {
                if (!string.IsNullOrWhiteSpace(source.ETag))
                {
                    headers["If-None-Match"] = source.ETag;
                }

                if (!string.IsNullOrWhiteSpace(source.LastModified))
                {
                    headers["If-Modified-Since"] = source.LastModified;
                }
            }

            var response = await transport.GetAsync(current, headers, source.TimeoutSeconds, source.MaxResponseBytes, cancellationToken);
            if (response.StatusCode == 304)
            {
                return new BlocklistFetchedContent(null, response.StatusCode, null, ReadHeader(response.Headers, "ETag"), ReadHeader(response.Headers, "Last-Modified"), true, redirects);
            }

            if (IsRedirect(response.StatusCode))
            {
                if (!response.Headers.TryGetValue("Location", out var location) || string.IsNullOrWhiteSpace(location))
                {
                    throw new InvalidOperationException("Redirect response did not include a Location header.");
                }

                if (redirectCount == source.MaxRedirects)
                {
                    throw new InvalidOperationException("Blocklist fetch exceeded the redirect limit.");
                }

                current = new Uri(current, location);
                ParseAndValidateUri(current.ToString(), source.AllowHttp);
                await ValidateResolvedHostAsync(current.Host, resolver, cancellationToken);
                redirects.Add(current);
                continue;
            }

            if (response.StatusCode < 200 || response.StatusCode > 299)
            {
                throw new InvalidOperationException($"Blocklist fetch returned HTTP {response.StatusCode}.");
            }

            var hash = Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(response.Content))).ToLowerInvariant();
            return new BlocklistFetchedContent(
                response.Content,
                response.StatusCode,
                hash,
                ReadHeader(response.Headers, "ETag"),
                ReadHeader(response.Headers, "Last-Modified"),
                false,
                redirects);
        }

        throw new InvalidOperationException("Blocklist fetch exceeded the redirect limit.");
    }

    private static Uri ParseAndValidateUri(string url, bool allowHttp)
    {
        if (!Uri.TryCreate(url, UriKind.Absolute, out var uri)
            || string.IsNullOrWhiteSpace(uri.Host)
            || uri.UserInfo.Length > 0)
        {
            throw new InvalidOperationException("Blocklist source URL must be an absolute URL without embedded credentials.");
        }

        if (uri.Scheme != Uri.UriSchemeHttps && !(allowHttp && uri.Scheme == Uri.UriSchemeHttp))
        {
            throw new InvalidOperationException("Blocklist source URL must use HTTPS unless HTTP is explicitly allowed.");
        }

        if (IPAddress.TryParse(uri.Host, out var ip) && BlocklistNetworkSafety.IsDeniedTarget(ip))
        {
            throw new InvalidOperationException("Blocklist source URL targets a private, local, metadata, link-local, or multicast address.");
        }

        return uri;
    }

    private static async Task ValidateResolvedHostAsync(
        string host,
        IBlocklistDnsResolver resolver,
        CancellationToken cancellationToken)
    {
        var addresses = await resolver.ResolveAsync(host, cancellationToken);
        if (addresses.Count == 0)
        {
            throw new InvalidOperationException("Blocklist source host did not resolve to an IP address.");
        }

        var denied = addresses.FirstOrDefault(BlocklistNetworkSafety.IsDeniedTarget);
        if (denied is not null)
        {
            throw new InvalidOperationException($"Blocklist source host resolves to a denied address: {denied}.");
        }
    }

    private static bool IsRedirect(int statusCode)
        => statusCode is 301 or 302 or 303 or 307 or 308;

    private static string? ReadHeader(IReadOnlyDictionary<string, string> headers, string name)
        => headers.TryGetValue(name, out var value) ? value : null;
}

public sealed record BlocklistFetchedContent(
    string? Content,
    int? HttpStatusCode,
    string? ContentHash,
    string? ETag,
    string? LastModified,
    bool NotModified,
    IReadOnlyList<Uri> Redirects);

public interface IBlocklistDnsResolver
{
    Task<IReadOnlyList<IPAddress>> ResolveAsync(string host, CancellationToken cancellationToken);
}

public sealed class DefaultBlocklistDnsResolver : IBlocklistDnsResolver
{
    public async Task<IReadOnlyList<IPAddress>> ResolveAsync(string host, CancellationToken cancellationToken)
        => await System.Net.Dns.GetHostAddressesAsync(host, cancellationToken);
}

public interface IBlocklistHttpTransport
{
    Task<BlocklistHttpTransportResponse> GetAsync(
        Uri uri,
        IReadOnlyDictionary<string, string> headers,
        int timeoutSeconds,
        int maxBytes,
        CancellationToken cancellationToken);
}

public sealed record BlocklistHttpTransportResponse(
    int StatusCode,
    IReadOnlyDictionary<string, string> Headers,
    string Content);

public sealed class SocketsBlocklistHttpTransport(IBlocklistDnsResolver resolver) : IBlocklistHttpTransport
{
    public async Task<BlocklistHttpTransportResponse> GetAsync(
        Uri uri,
        IReadOnlyDictionary<string, string> headers,
        int timeoutSeconds,
        int maxBytes,
        CancellationToken cancellationToken)
    {
        using var handler = new SocketsHttpHandler
        {
            AllowAutoRedirect = false,
            ConnectCallback = async (context, ct) =>
            {
                var addresses = await resolver.ResolveAsync(context.DnsEndPoint.Host, ct);
                if (addresses.Count == 0 || addresses.Any(BlocklistNetworkSafety.IsDeniedTarget))
                {
                    throw new InvalidOperationException("Final connection IP is not allowed for blocklist fetching.");
                }

                var endpoint = new IPEndPoint(addresses[0], context.DnsEndPoint.Port);
                var socket = new Socket(endpoint.AddressFamily, SocketType.Stream, ProtocolType.Tcp) { NoDelay = true };
                try
                {
                    await socket.ConnectAsync(endpoint, ct);
                    return new NetworkStream(socket, ownsSocket: true);
                }
                catch
                {
                    socket.Dispose();
                    throw;
                }
            },
        };
        using var client = new HttpClient(handler, disposeHandler: true)
        {
            Timeout = TimeSpan.FromSeconds(Math.Clamp(timeoutSeconds, 1, 120)),
        };
        using var request = new HttpRequestMessage(HttpMethod.Get, uri);
        foreach (var (name, value) in headers)
        {
            request.Headers.TryAddWithoutValidation(name, value);
        }

        using var response = await client.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, cancellationToken);
        var responseHeaders = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
        foreach (var header in response.Headers)
        {
            responseHeaders[header.Key] = string.Join(", ", header.Value);
        }

        foreach (var header in response.Content.Headers)
        {
            responseHeaders[header.Key] = string.Join(", ", header.Value);
        }

        var content = await ReadLimitedContentAsync(response.Content, maxBytes, cancellationToken);
        return new BlocklistHttpTransportResponse((int)response.StatusCode, responseHeaders, content);
    }

    private static async Task<string> ReadLimitedContentAsync(
        HttpContent content,
        int maxBytes,
        CancellationToken cancellationToken)
    {
        await using var stream = await content.ReadAsStreamAsync(cancellationToken);
        using var output = new MemoryStream();
        var buffer = new byte[8192];
        var total = 0;
        while (true)
        {
            var read = await stream.ReadAsync(buffer, cancellationToken);
            if (read == 0)
            {
                break;
            }

            total += read;
            if (total > maxBytes)
            {
                throw new InvalidOperationException("Blocklist response exceeded the configured size limit.");
            }

            output.Write(buffer, 0, read);
        }

        return Encoding.UTF8.GetString(output.ToArray());
    }
}

public static class BlocklistNetworkSafety
{
    public static bool IsDeniedTarget(IPAddress address)
    {
        if (address.IsIPv4MappedToIPv6)
        {
            address = address.MapToIPv4();
        }

        if (IPAddress.IsLoopback(address) || address.Equals(IPAddress.Any) || address.Equals(IPAddress.IPv6Any))
        {
            return true;
        }

        var bytes = address.GetAddressBytes();
        if (address.AddressFamily == AddressFamily.InterNetwork)
        {
            return bytes[0] == 0
                   || bytes[0] == 10
                   || bytes[0] == 127
                   || (bytes[0] == 100 && bytes[1] >= 64 && bytes[1] <= 127)
                   || (bytes[0] == 169 && bytes[1] == 254)
                   || (bytes[0] == 172 && bytes[1] >= 16 && bytes[1] <= 31)
                   || (bytes[0] == 192 && bytes[1] == 168)
                   || bytes[0] >= 224
                   || address.Equals(IPAddress.Parse("169.254.169.254"));
        }

        return address.IsIPv6LinkLocal
               || address.IsIPv6Multicast
               || (bytes[0] & 0xfe) == 0xfc;
    }
}

internal sealed record ParserOptions(
    int ValueColumnIndex,
    int? CidrPrefixColumnIndex,
    string? JsonArrayField,
    IReadOnlyList<string> JsonValueFields)
{
    public static ParserOptions FromMetadata(Dictionary<string, object?> metadata)
    {
        var parser = metadata.TryGetValue("parser", out var value) && value is JsonElement json
            ? json
            : default;
        var valueColumn = ReadInt(parser, "valueColumnIndex")
            ?? ReadInt(parser, "csvColumnIndex")
            ?? 0;
        var prefixColumn = ReadInt(parser, "cidrPrefixColumnIndex");
        var arrayField = ReadString(parser, "jsonArrayField");
        var valueField = ReadString(parser, "jsonValueField");
        var fields = string.IsNullOrWhiteSpace(valueField)
            ? ["cidr", "ip", "ip_address", "address", "value"]
            : new[] { valueField!, "cidr", "ip", "ip_address", "address", "value" }.Distinct(StringComparer.OrdinalIgnoreCase).ToArray();
        return new ParserOptions(valueColumn, prefixColumn, arrayField, fields);
    }

    private static int? ReadInt(JsonElement element, string name)
    {
        if (element.ValueKind == JsonValueKind.Object
            && element.TryGetProperty(name, out var value)
            && value.ValueKind == JsonValueKind.Number
            && value.TryGetInt32(out var number))
        {
            return number;
        }

        return null;
    }

    private static string? ReadString(JsonElement element, string name)
        => element.ValueKind == JsonValueKind.Object
           && element.TryGetProperty(name, out var value)
           && value.ValueKind == JsonValueKind.String
            ? value.GetString()
            : null;
}

internal sealed class MutableParseResult
{
    private readonly Dictionary<string, BlocklistParsedEntry> entries = new(StringComparer.OrdinalIgnoreCase);

    public int IgnoredCount { get; set; }

    public List<string> Errors { get; set; } = [];

    public void AddToken(string? token, int? lineNumber)
    {
        if (string.IsNullOrWhiteSpace(token))
        {
            IgnoredCount++;
            return;
        }

        if (!BlocklistSubjectNormalizer.TryNormalize(token, out var parsed, out var error))
        {
            Errors.Add(lineNumber is null ? error : $"Line {lineNumber}: {error}");
            return;
        }

        entries.TryAdd(parsed.NormalizedValue, new BlocklistParsedEntry(
            parsed.SubjectType,
            token.Trim(),
            parsed.NormalizedValue,
            lineNumber,
            JsonSerializer.Serialize(new { lineNumber })));
    }

    public BlocklistParseResult ToResult()
        => new(entries.Values.OrderBy(x => x.NormalizedValue, StringComparer.OrdinalIgnoreCase).ToList(), IgnoredCount, Errors);
}

public sealed record NormalizedBlocklistSubject(string SubjectType, string NormalizedValue);

public static class BlocklistSubjectNormalizer
{
    public static bool TryNormalize(string raw, out NormalizedBlocklistSubject subject, out string error)
    {
        subject = new NormalizedBlocklistSubject(SecuritySubjectTypeNames.Ip, string.Empty);
        error = string.Empty;
        var value = raw.Trim().Trim('"', '\'');
        if (value.Contains('/', StringComparison.Ordinal))
        {
            var parts = value.Split('/', 2);
            if (!IPAddress.TryParse(parts[0], out var network) || !int.TryParse(parts[1], NumberStyles.None, CultureInfo.InvariantCulture, out var prefix))
            {
                error = $"Invalid CIDR entry '{raw}'.";
                return false;
            }

            network = NormalizeMapped(network);
            var maxPrefix = network.AddressFamily == AddressFamily.InterNetwork ? 32 : 128;
            if (prefix < 0 || prefix > maxPrefix)
            {
                error = $"CIDR prefix is outside 0-{maxPrefix}: '{raw}'.";
                return false;
            }

            subject = new NormalizedBlocklistSubject(SecuritySubjectTypeNames.Cidr, $"{Mask(network, prefix)}/{prefix}");
            return true;
        }

        if (!IPAddress.TryParse(value, out var address))
        {
            error = $"Invalid IP entry '{raw}'.";
            return false;
        }

        subject = new NormalizedBlocklistSubject(SecuritySubjectTypeNames.Ip, NormalizeMapped(address).ToString());
        return true;
    }

    private static IPAddress NormalizeMapped(IPAddress address)
        => address.IsIPv4MappedToIPv6 ? address.MapToIPv4() : address;

    private static IPAddress Mask(IPAddress address, int prefixLength)
    {
        var bytes = address.GetAddressBytes();
        var remaining = prefixLength;
        for (var i = 0; i < bytes.Length; i++)
        {
            if (remaining >= 8)
            {
                remaining -= 8;
                continue;
            }

            if (remaining <= 0)
            {
                bytes[i] = 0;
                continue;
            }

            var mask = (byte)(0xff << (8 - remaining));
            bytes[i] &= mask;
            remaining = 0;
        }

        return new IPAddress(bytes);
    }
}

internal static class RecommendedMetadata
{
    public static Dictionary<string, object?> Read(string? json)
    {
        if (string.IsNullOrWhiteSpace(json))
        {
            return [];
        }

        try
        {
            return JsonSerializer.Deserialize<Dictionary<string, object?>>(json) ?? [];
        }
        catch (JsonException)
        {
            return [];
        }
    }

    public static bool IsRecommended(string? json)
        => Read(json).TryGetValue("recommended", out var value)
           && value is JsonElement { ValueKind: JsonValueKind.True };
}

internal sealed record RecommendedBlocklistSource(
    string Name,
    string SourceUrl,
    string Description,
    string Format,
    string EnforcementMode,
    bool CanFirewallEnforce,
    int RefreshIntervalHours,
    object Metadata)
{
    public BlocklistSourceEntity ToEntity()
        => new()
        {
            Name = Name,
            SourceUrl = SourceUrl,
            Description = Description,
            Format = Format,
            EnforcementMode = EnforcementMode,
            CanFirewallEnforce = CanFirewallEnforce,
            Enabled = false,
            AllowHttp = false,
            RefreshIntervalHours = RefreshIntervalHours,
            MetadataJson = JsonSerializer.Serialize(Metadata),
        };
}

internal static class RecommendedBlocklistSources
{
    public static IReadOnlyList<RecommendedBlocklistSource> All { get; } =
    [
        new(
            "Feodo Tracker Botnet C2 IP blocklist",
            "https://feodotracker.abuse.ch/downloads/ipblocklist_recommended.txt",
            "Active or recently active botnet C2 IPs with lower false-positive risk than broader Feodo IoC lists.",
            BlocklistSourceFormatNames.Text,
            BlocklistEnforcementModeNames.Middleware,
            true,
            1,
            new
            {
                recommended = true,
                docsUrl = "https://feodotracker.abuse.ch/blocklist/",
                falsePositiveWarning = "Feodo describes the recommended list as lower false-positive risk, but IP reuse can still affect legitimate traffic.",
                observedFormat = "Plain text with # comments and IP address lines; docs say generated every 5 minutes and recommend 5-15 minute updates.",
                parser = new { }
            }),
        new(
            "Spamhaus DROP",
            "https://www.spamhaus.org/drop/drop_v4.json",
            "High-confidence IPv4 netblocks controlled by serious abuse operations.",
            BlocklistSourceFormatNames.JsonLines,
            BlocklistEnforcementModeNames.Middleware,
            true,
            1,
            new
            {
                recommended = true,
                docsUrl = "https://www.spamhaus.org/blocklists/do-not-route-or-peer/",
                falsePositiveWarning = "Spamhaus describes DROP as high confidence, but firewall enforcement can still interrupt legitimate dependencies.",
                observedFormat = "Newline-delimited JSON objects with a cidr field; Spamhaus asks automated clients not to fetch more than once per hour.",
                parser = new { jsonValueField = "cidr" }
            }),
        new(
            "Spamhaus DROPv6",
            "https://www.spamhaus.org/drop/drop_v6.json",
            "High-confidence IPv6 netblocks controlled by serious abuse operations.",
            BlocklistSourceFormatNames.JsonLines,
            BlocklistEnforcementModeNames.Middleware,
            true,
            1,
            new
            {
                recommended = true,
                docsUrl = "https://www.spamhaus.org/blocklists/do-not-route-or-peer/",
                falsePositiveWarning = "Spamhaus describes DROP as high confidence, but firewall enforcement can still interrupt legitimate dependencies.",
                observedFormat = "Newline-delimited JSON objects with a cidr field; Spamhaus asks automated clients not to fetch more than once per hour.",
                parser = new { jsonValueField = "cidr" }
            }),
        new(
            "DShield recommended block list",
            "https://feeds.dshield.org/block.txt",
            "Top attacking /24 subnets observed by the SANS Internet Storm Center over the last several days.",
            BlocklistSourceFormatNames.Tsv,
            BlocklistEnforcementModeNames.Middleware,
            true,
            24,
            new
            {
                recommended = true,
                docsUrl = "https://www.dshield.org/hpbinfo.html",
                falsePositiveWarning = "DShield summarizes active scanning subnets; this is higher false-positive risk than precise botnet C2 feeds.",
                observedFormat = "Tab-delimited rows: start IP, end IP, prefix length, attack counts, network, country, contact.",
                parser = new { valueColumnIndex = 0, cidrPrefixColumnIndex = 2 }
            }),
        new(
            "FireHOL Level 1",
            "https://iplists.firehol.org/files/firehol_level1.netset",
            "Composite netset intended for broad baseline protection with minimum false positives.",
            BlocklistSourceFormatNames.Netset,
            BlocklistEnforcementModeNames.Middleware,
            true,
            2,
            new
            {
                recommended = true,
                docsUrl = "https://iplists.firehol.org/?ipset=firehol_level1",
                falsePositiveWarning = "FireHOL Level 1 is a composite feed; review included sources before firewall enforcement.",
                observedFormat = "FireHOL .netset file with # comments followed by IP/CIDR entries.",
                parser = new { }
            }),
    ];
}
