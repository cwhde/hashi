using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Metadata;
using Xunit;

namespace Hashi.UnitTests;

public sealed class SecurityDataFoundationTests
{
    [Fact]
    public void Manual_allow_defaults_match_addendum_bypass_semantics()
    {
        var entry = new ManualSecurityEntryEntity();

        Assert.Equal(ManualSecurityEntryTypeNames.Allow, entry.EntryType);
        Assert.True(entry.BypassBlocking);
        Assert.True(entry.BypassAdaptiveEscalation);
        Assert.False(entry.BypassRateLimit);
        Assert.False(entry.BypassChallenge);
        Assert.False(entry.BypassSso);
    }

    [Fact]
    public async Task Manual_block_entries_have_database_constraint_for_bypass_flags()
    {
        await using var db = CreateDb();

        var entityType = db.GetService<IDesignTimeModel>().Model.FindEntityType(typeof(ManualSecurityEntryEntity));
        Assert.NotNull(entityType);

        var constraint = Assert.Single(
            entityType.GetCheckConstraints(),
            x => x.Name == "CK_manual_security_entries_block_bypass_flags_false");
        Assert.Contains("\"EntryType\" <> 'block'", constraint.Sql);
        Assert.Contains("NOT \"BypassBlocking\"", constraint.Sql);
        Assert.Contains("NOT \"BypassAdaptiveEscalation\"", constraint.Sql);
        Assert.Contains("NOT \"BypassRateLimit\"", constraint.Sql);
        Assert.Contains("NOT \"BypassChallenge\"", constraint.Sql);
        Assert.Contains("NOT \"BypassSso\"", constraint.Sql);
    }

    [Fact]
    public void Blocklist_entries_do_not_expose_manual_allow_bypass_flags()
    {
        var bypassProperties = typeof(BlocklistEntryEntity)
            .GetProperties()
            .Where(x => x.Name.Contains("Bypass", StringComparison.OrdinalIgnoreCase))
            .Select(x => x.Name)
            .ToList();

        Assert.Empty(bypassProperties);
    }

    [Fact]
    public async Task Security_subject_unique_index_uses_type_and_normalized_value()
    {
        await using var db = CreateDb();

        var entityType = db.Model.FindEntityType(typeof(SecuritySubjectEntity));
        Assert.NotNull(entityType);

        var index = Assert.Single(entityType.GetIndexes(), x =>
            x.IsUnique
            && x.Properties.Select(p => p.Name).SequenceEqual([
                nameof(SecuritySubjectEntity.SubjectType),
                nameof(SecuritySubjectEntity.NormalizedValue),
            ]));
        Assert.True(index.IsUnique);
    }

    [Fact]
    public async Task Existing_security_tables_keep_compatibility_columns_and_add_safe_defaults()
    {
        await using var db = CreateDb();

        var blocklistEntry = db.Model.FindEntityType(typeof(BlocklistEntryEntity));
        Assert.NotNull(blocklistEntry);
        Assert.Equal(true, blocklistEntry.FindProperty(nameof(BlocklistEntryEntity.Enabled))?.GetDefaultValue());
        Assert.Equal(BlocklistEnforcementModeNames.Middleware, blocklistEntry.FindProperty(nameof(BlocklistEntryEntity.EnforcementMode))?.GetDefaultValue());
        Assert.Equal(SecuritySubjectTypeNames.Ip, blocklistEntry.FindProperty(nameof(BlocklistEntryEntity.SubjectType))?.GetDefaultValue());

        var requestBucket = db.Model.FindEntityType(typeof(SecurityRequestBucketEntity));
        Assert.NotNull(requestBucket);
        Assert.Equal(60, requestBucket.FindProperty(nameof(SecurityRequestBucketEntity.BucketSizeSeconds))?.GetDefaultValue());
        Assert.Equal(SecuritySubjectTypeNames.Ip, requestBucket.FindProperty(nameof(SecurityRequestBucketEntity.SubjectType))?.GetDefaultValue());

        db.AbuseBuckets.Add(new AbuseBucketEntity { ClientIp = "198.51.100.10", Score = 2 });
        db.SecurityEvents.Add(new SecurityEventEntity
        {
            Category = "access",
            Action = "allowed",
            ClientIp = "198.51.100.10",
            Host = "app.example.com",
            Path = "/",
        });
        db.SecurityRequestBuckets.Add(new SecurityRequestBucketEntity
        {
            BucketStartUtc = DateTimeOffset.UtcNow,
            ClientIp = "198.51.100.10",
            Resource = "app.example.com",
            TraefikInstance = "default",
            StatusClass = 2,
        });
        db.BlocklistEntries.Add(new BlocklistEntryEntity
        {
            ClientIp = "198.51.100.10",
            Type = BlocklistTypeNames.Ip,
            Value = "198.51.100.10",
            Reason = "compatibility",
        });

        db.SaveChanges();

        Assert.Equal(1, db.AbuseBuckets.Count());
        Assert.Equal(1, db.SecurityEvents.Count());
        Assert.Equal(1, db.SecurityRequestBuckets.Count());
        Assert.Equal(1, db.BlocklistEntries.Count());
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
