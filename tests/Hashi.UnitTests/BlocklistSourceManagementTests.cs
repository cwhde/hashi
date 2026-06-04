using System.Net;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Xunit;

namespace Hashi.UnitTests;

public sealed class BlocklistSourceManagementTests
{
    [Fact]
    public void Parser_supports_text_firehol_tsv_json_and_json_lines()
    {
        var parser = new BlocklistParser();

        var text = parser.Parse(Source(BlocklistSourceFormatNames.Text), """
            # comment
            203.0.113.10
            203.0.113.10 # duplicate
            2001:db8::1
            """);
        Assert.Equal(2, text.Entries.Count);

        var firehol = parser.Parse(Source(BlocklistSourceFormatNames.Netset), """
            #
            # firehol_level1
            192.0.2.0/24
            2001:db8:abcd::/48
            """);
        Assert.Contains(firehol.Entries, x => x.SubjectType == SecuritySubjectTypeNames.Cidr && x.NormalizedValue == "192.0.2.0/24");

        var tsv = parser.Parse(Source(
            BlocklistSourceFormatNames.Tsv,
            """{"parser":{"valueColumnIndex":0,"cidrPrefixColumnIndex":2}}"""), """
            # start	end	prefix
            198.51.100.0	198.51.100.255	24	10	EXAMPLE	US	abuse@example.com
            """);
        Assert.Equal("198.51.100.0/24", Assert.Single(tsv.Entries).NormalizedValue);

        var jsonArray = parser.Parse(Source(
            BlocklistSourceFormatNames.Json,
            """{"parser":{"jsonValueField":"ip_address"}}"""), """
            [{"ip_address":"203.0.113.15"},{"ip_address":"203.0.113.16"}]
            """);
        Assert.Equal(2, jsonArray.Entries.Count);

        var jsonObjectArray = parser.Parse(Source(
            BlocklistSourceFormatNames.Json,
            """{"parser":{"jsonArrayField":"items","jsonValueField":"cidr"}}"""), """
            {"items":["203.0.113.0/24",{"cidr":"2001:db8::/48"}]}
            """);
        Assert.Equal(2, jsonObjectArray.Entries.Count);

        var jsonLines = parser.Parse(Source(
            BlocklistSourceFormatNames.JsonLines,
            """{"parser":{"jsonValueField":"cidr"}}"""), """
            {"cidr":"203.0.113.0/24","sblid":"SBL1"}
            {"cidr":"2001:db8::/48","sblid":"SBL2"}
            """);
        Assert.Equal(2, jsonLines.Entries.Count);
    }

    [Fact]
    public async Task Fetcher_rejects_private_dns_targets_and_private_redirects()
    {
        var resolver = new FakeDnsResolver();
        resolver.Add("internal.example", IPAddress.Parse("127.0.0.1"));
        resolver.Add("feed.example", IPAddress.Parse("203.0.113.10"));
        var transport = new FakeTransport();
        var fetcher = new BlocklistSafeHttpFetcher(resolver, transport);

        await Assert.ThrowsAsync<InvalidOperationException>(() => fetcher.ValidateSourceAsync(new BlocklistSourceEntity
        {
            SourceUrl = "https://internal.example/list.txt",
        }));

        transport.Enqueue(new BlocklistHttpTransportResponse(
            302,
            new Dictionary<string, string> { ["Location"] = "http://127.0.0.1/latest.txt" },
            string.Empty));
        await Assert.ThrowsAsync<InvalidOperationException>(() => fetcher.FetchAsync(new BlocklistSourceEntity
        {
            SourceUrl = "https://feed.example/list.txt",
            MaxRedirects = 3,
            MaxResponseBytes = 1024,
            TimeoutSeconds = 5,
        }, conditional: false));
    }

    [Fact]
    public async Task Refresh_failure_preserves_last_known_good_entries()
    {
        await using var db = CreateDb();
        var resolver = PublicResolver();
        var transport = new FakeTransport();
        var service = CreateService(db, resolver, transport);
        var source = new BlocklistSourceEntity
        {
            Name = "custom",
            SourceUrl = "https://feed.example/list.txt",
            Format = BlocklistSourceFormatNames.Text,
            Enabled = true,
        };
        db.BlocklistSources.Add(source);
        await db.SaveChangesAsync();

        transport.Enqueue(new BlocklistHttpTransportResponse(200, new Dictionary<string, string>(), "203.0.113.44\n"));
        var first = await service.RefreshAsync(source.Id);
        Assert.Equal(BlocklistFetchStatusNames.Succeeded, first!.Run!.Status);
        Assert.Single(await db.BlocklistEntries.Where(x => x.SourceId == source.Id && x.Enabled).ToListAsync());

        transport.FailNext = "upstream unavailable";
        var second = await service.RefreshAsync(source.Id);

        Assert.Equal(BlocklistFetchStatusNames.Failed, second!.Run!.Status);
        var entry = Assert.Single(await db.BlocklistEntries.Where(x => x.SourceId == source.Id).ToListAsync());
        Assert.True(entry.Enabled);
        Assert.Equal("203.0.113.44", entry.NormalizedValue);
    }

    [Fact]
    public async Task Disable_marks_entries_inactive_and_firewall_preview_hook_reports_pending()
    {
        await using var db = CreateDb();
        var transport = new FakeTransport();
        var service = CreateService(db, PublicResolver(), transport);
        var source = new BlocklistSourceEntity
        {
            Name = "firewall feed",
            SourceUrl = "https://feed.example/list.txt",
            Format = BlocklistSourceFormatNames.Text,
            Enabled = true,
            CanFirewallEnforce = true,
            EnforcementMode = BlocklistEnforcementModeNames.Firewall,
        };
        db.BlocklistSources.Add(source);
        await db.SaveChangesAsync();

        transport.Enqueue(new BlocklistHttpTransportResponse(200, new Dictionary<string, string>(), "198.51.100.0/24\n"));
        var refresh = await service.RefreshAsync(source.Id);

        Assert.True(refresh!.FirewallSyncRecommended);
        Assert.Equal(1, refresh.PendingFirewallEntryCount);
        var disable = await service.DisableAsync(source.Id);
        Assert.False(disable!.Source.Enabled);
        Assert.All(await db.BlocklistEntries.Where(x => x.SourceId == source.Id).ToListAsync(), x => Assert.False(x.Enabled));
    }

    [Fact]
    public async Task Recommended_sources_are_seeded_disabled_with_warnings()
    {
        await using var db = CreateDb();
        var service = CreateService(db, PublicResolver(), new FakeTransport());

        var sources = await service.ListAsync();

        Assert.Contains(sources, x => x.Name.Contains("Feodo", StringComparison.OrdinalIgnoreCase));
        Assert.Contains(sources, x => x.Name.Contains("Spamhaus DROP", StringComparison.OrdinalIgnoreCase));
        Assert.Contains(sources, x => x.Name.Contains("DShield", StringComparison.OrdinalIgnoreCase));
        Assert.Contains(sources, x => x.Name.Contains("FireHOL", StringComparison.OrdinalIgnoreCase));
        Assert.All(sources, x => Assert.False(x.Enabled));
        Assert.All(sources, x => Assert.Contains("falsePositiveWarning", x.MetadataJson ?? string.Empty, StringComparison.OrdinalIgnoreCase));
    }

    private static BlocklistSourceEntity Source(string format, string metadataJson = "{}")
        => new()
        {
            Name = "test",
            SourceUrl = "https://feed.example/list.txt",
            Format = format,
            MetadataJson = metadataJson,
        };

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private static BlocklistSourceManagementService CreateService(
        HashiDbContext db,
        FakeDnsResolver resolver,
        FakeTransport transport)
        => new(
            db,
            new BlocklistSafeHttpFetcher(resolver, transport),
            new BlocklistParser(),
            new AuditService(db),
            NullLogger<BlocklistSourceManagementService>.Instance);

    private static FakeDnsResolver PublicResolver()
    {
        var resolver = new FakeDnsResolver();
        resolver.Add("feed.example", IPAddress.Parse("203.0.113.10"));
        return resolver;
    }

    private sealed class FakeDnsResolver : IBlocklistDnsResolver
    {
        private readonly Dictionary<string, IReadOnlyList<IPAddress>> addresses = new(StringComparer.OrdinalIgnoreCase);

        public void Add(string host, params IPAddress[] values)
            => addresses[host] = values;

        public Task<IReadOnlyList<IPAddress>> ResolveAsync(string host, CancellationToken cancellationToken)
            => Task.FromResult(addresses.TryGetValue(host, out var values) ? values : [IPAddress.Parse("203.0.113.10")]);
    }

    private sealed class FakeTransport : IBlocklistHttpTransport
    {
        private readonly Queue<BlocklistHttpTransportResponse> responses = new();

        public string? FailNext { get; set; }

        public void Enqueue(BlocklistHttpTransportResponse response)
            => responses.Enqueue(response);

        public Task<BlocklistHttpTransportResponse> GetAsync(
            Uri uri,
            IReadOnlyDictionary<string, string> headers,
            int timeoutSeconds,
            int maxBytes,
            CancellationToken cancellationToken)
        {
            if (FailNext is not null)
            {
                var message = FailNext;
                FailNext = null;
                throw new InvalidOperationException(message);
            }

            return Task.FromResult(responses.Dequeue());
        }
    }
}
