using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Persistence;

public sealed class HashiDbContext(DbContextOptions<HashiDbContext> options) : DbContext(options)
{
    public DbSet<AppSettingsEntity> AppSettings => Set<AppSettingsEntity>();

    public DbSet<SetupStateEntity> SetupStates => Set<SetupStateEntity>();

    public DbSet<AuditEventEntity> AuditEvents => Set<AuditEventEntity>();

    public DbSet<SyncRunEntity> SyncRuns => Set<SyncRunEntity>();

    public DbSet<SyncStepEntity> SyncSteps => Set<SyncStepEntity>();

    public DbSet<SyncDiffEntity> SyncDiffs => Set<SyncDiffEntity>();

    public DbSet<PasskeyCredentialEntity> PasskeyCredentials => Set<PasskeyCredentialEntity>();

    public DbSet<VaultWrappedKeyEntity> VaultWrappedKeys => Set<VaultWrappedKeyEntity>();

    public DbSet<SecretRecordEntity> SecretRecords => Set<SecretRecordEntity>();

    public DbSet<ConnectionEntity> Connections => Set<ConnectionEntity>();

    public DbSet<DnsZoneEntity> DnsZones => Set<DnsZoneEntity>();

    public DbSet<DnsRecordEntity> DnsRecords => Set<DnsRecordEntity>();

    public DbSet<DnsImportDecisionEntity> DnsImportDecisions => Set<DnsImportDecisionEntity>();

    public DbSet<ResourceEntity> Resources => Set<ResourceEntity>();

    public DbSet<MonitorEndpointEntity> MonitorEndpoints => Set<MonitorEndpointEntity>();

    public DbSet<PulseAgentEntity> PulseAgents => Set<PulseAgentEntity>();

    public DbSet<TraefikHostStateEntity> TraefikHostStates => Set<TraefikHostStateEntity>();

    public DbSet<TraefikUserMiddlewareEntity> TraefikUserMiddlewares => Set<TraefikUserMiddlewareEntity>();

    public DbSet<FirewallHostEntity> FirewallHosts => Set<FirewallHostEntity>();

    public DbSet<MonitorSampleEntity> MonitorSamples => Set<MonitorSampleEntity>();

    public DbSet<MonitorRollupEntity> MonitorRollups => Set<MonitorRollupEntity>();

    public DbSet<OidcProviderEntity> OidcProviders => Set<OidcProviderEntity>();

    public DbSet<EdgeAuthRuleEntity> EdgeAuthRules => Set<EdgeAuthRuleEntity>();

    public DbSet<AccessLogEventEntity> AccessLogEvents => Set<AccessLogEventEntity>();

    public DbSet<AbuseBucketEntity> AbuseBuckets => Set<AbuseBucketEntity>();

    public DbSet<BlocklistEntryEntity> BlocklistEntries => Set<BlocklistEntryEntity>();

    public DbSet<AdGuardConnectionEntity> AdGuardConnections => Set<AdGuardConnectionEntity>();

    public DbSet<AdGuardRewriteEntity> AdGuardRewrites => Set<AdGuardRewriteEntity>();

    public DbSet<NotificationProviderEntity> NotificationProviders => Set<NotificationProviderEntity>();

    public DbSet<ScriptEntity> Scripts => Set<ScriptEntity>();

    protected override void OnModelCreating(ModelBuilder modelBuilder)
    {
        modelBuilder.Entity<AppSettingsEntity>(entity =>
        {
            entity.ToTable("app_settings");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Theme).HasMaxLength(32);
        });

        modelBuilder.Entity<SetupStateEntity>(entity =>
        {
            entity.ToTable("setup_state");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.CurrentStep).HasMaxLength(64);
            entity.Property(x => x.BootstrapUsername).HasMaxLength(128);
        });

        modelBuilder.Entity<AuditEventEntity>(entity =>
        {
            entity.ToTable("audit_events");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Category).HasMaxLength(64);
            entity.Property(x => x.Action).HasMaxLength(128);
            entity.Property(x => x.Outcome).HasMaxLength(32);
            entity.HasIndex(x => x.CreatedAtUtc);
        });

        modelBuilder.Entity<SyncRunEntity>(entity =>
        {
            entity.ToTable("sync_runs");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Subsystem).HasMaxLength(64);
            entity.Property(x => x.Status).HasMaxLength(32);
            entity.HasIndex(x => x.StartedAtUtc);
        });

        modelBuilder.Entity<SyncStepEntity>(entity =>
        {
            entity.ToTable("sync_steps");
            entity.HasKey(x => x.Id);
            entity.HasOne(x => x.SyncRun).WithMany(x => x.Steps).HasForeignKey(x => x.SyncRunId);
        });

        modelBuilder.Entity<SyncDiffEntity>(entity =>
        {
            entity.ToTable("sync_diffs");
            entity.HasKey(x => x.Id);
            entity.HasOne(x => x.SyncRun).WithMany(x => x.Diffs).HasForeignKey(x => x.SyncRunId);
        });

        modelBuilder.Entity<PasskeyCredentialEntity>(entity =>
        {
            entity.ToTable("passkey_credentials");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Nickname).HasMaxLength(128);
            entity.HasIndex(x => x.CredentialId).IsUnique();
        });

        modelBuilder.Entity<VaultWrappedKeyEntity>(entity =>
        {
            entity.ToTable("vault_wrapped_keys");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.WrapMethod).HasMaxLength(32);
            entity.HasOne(x => x.PasskeyCredential)
                .WithMany()
                .HasForeignKey(x => x.PasskeyCredentialId)
                .OnDelete(DeleteBehavior.SetNull);
            entity.HasIndex(x => x.WrapMethod);
        });

        modelBuilder.Entity<SecretRecordEntity>(entity =>
        {
            entity.ToTable("secret_records");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Purpose).HasMaxLength(64);
            entity.Property(x => x.Label).HasMaxLength(256);
            entity.HasIndex(x => x.Purpose);
        });

        modelBuilder.Entity<ConnectionEntity>(entity =>
        {
            entity.ToTable("connections");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Name).HasMaxLength(128);
            entity.Property(x => x.Type).HasMaxLength(64);
            entity.Property(x => x.HealthState).HasMaxLength(32);
            entity.Property(x => x.DeletionPolicy).HasMaxLength(32);
            entity.HasIndex(x => x.Type);
        });

        modelBuilder.Entity<DnsZoneEntity>(entity =>
        {
            entity.ToTable("dns_zones");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.ProviderZoneId).HasMaxLength(128);
            entity.Property(x => x.Name).HasMaxLength(256);
            entity.HasOne(x => x.Connection).WithMany().HasForeignKey(x => x.ConnectionId);
            entity.HasIndex(x => x.ConnectionId);
        });

        modelBuilder.Entity<DnsRecordEntity>(entity =>
        {
            entity.ToTable("dns_records");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.ProviderRecordId).HasMaxLength(128);
            entity.Property(x => x.Name).HasMaxLength(256);
            entity.Property(x => x.Type).HasMaxLength(16);
            entity.Property(x => x.Ownership).HasMaxLength(32);
            entity.HasOne(x => x.Zone).WithMany().HasForeignKey(x => x.ZoneId);
            entity.HasIndex(x => x.ZoneId);
        });

        modelBuilder.Entity<DnsImportDecisionEntity>(entity =>
        {
            entity.ToTable("dns_import_decisions");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.ProviderRecordId).HasMaxLength(128);
            entity.Property(x => x.Name).HasMaxLength(256);
            entity.Property(x => x.Type).HasMaxLength(16);
            entity.HasOne(x => x.Zone).WithMany().HasForeignKey(x => x.ZoneId);
            entity.HasIndex(x => x.ZoneId);
        });

        modelBuilder.Entity<ResourceEntity>(entity =>
        {
            entity.ToTable("resources");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Name).HasMaxLength(128);
            entity.Property(x => x.Slug).HasMaxLength(128);
            entity.Property(x => x.Kind).HasMaxLength(16);
            entity.HasIndex(x => x.Slug).IsUnique();
        });

        modelBuilder.Entity<MonitorEndpointEntity>(entity =>
        {
            entity.ToTable("monitor_endpoints");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Name).HasMaxLength(128);
            entity.Property(x => x.CheckType).HasMaxLength(16);
            entity.Property(x => x.Status).HasMaxLength(16);
        });

        modelBuilder.Entity<PulseAgentEntity>(entity =>
        {
            entity.ToTable("pulse_agents");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Name).HasMaxLength(128);
            entity.Property(x => x.Status).HasMaxLength(32);
            entity.Property(x => x.TokenHash).HasMaxLength(128);
        });

        modelBuilder.Entity<TraefikHostStateEntity>(entity =>
        {
            entity.ToTable("traefik_host_states");
            entity.HasKey(x => x.Id);
            entity.HasIndex(x => x.ConnectionId).IsUnique();
        });

        modelBuilder.Entity<TraefikUserMiddlewareEntity>(entity =>
        {
            entity.ToTable("traefik_user_middlewares");
            entity.HasKey(x => x.Id);
        });

        modelBuilder.Entity<FirewallHostEntity>(entity =>
        {
            entity.ToTable("firewall_hosts");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Name).HasMaxLength(128);
            entity.HasIndex(x => new { x.ConnectionId, x.Name }).IsUnique();
        });

        modelBuilder.Entity<MonitorSampleEntity>(entity =>
        {
            entity.ToTable("monitor_samples");
            entity.HasKey(x => x.Id);
            entity.HasIndex(x => new { x.MonitorEndpointId, x.PartitionDate, x.CheckedAtUtc });
        });

        modelBuilder.Entity<MonitorRollupEntity>(entity =>
        {
            entity.ToTable("monitor_rollups");
            entity.HasKey(x => x.Id);
            entity.HasIndex(x => new { x.MonitorEndpointId, x.BucketStartUtc, x.IntervalMinutes }).IsUnique();
        });

        modelBuilder.Entity<OidcProviderEntity>(entity =>
        {
            entity.ToTable("oidc_providers");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Name).HasMaxLength(128);
        });

        modelBuilder.Entity<EdgeAuthRuleEntity>(entity =>
        {
            entity.ToTable("edge_auth_rules");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Name).HasMaxLength(128);
            entity.Property(x => x.Action).HasMaxLength(32);
        });

        modelBuilder.Entity<AccessLogEventEntity>(entity =>
        {
            entity.ToTable("access_log_events");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.ClientIp).HasMaxLength(64);
            entity.HasIndex(x => x.ReceivedAtUtc);
        });

        modelBuilder.Entity<AbuseBucketEntity>(entity =>
        {
            entity.ToTable("abuse_buckets");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.ClientIp).HasMaxLength(64);
            entity.HasIndex(x => x.ClientIp).IsUnique();
        });

        modelBuilder.Entity<BlocklistEntryEntity>(entity =>
        {
            entity.ToTable("blocklist_entries");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.ClientIp).HasMaxLength(64);
        });

        modelBuilder.Entity<AdGuardConnectionEntity>(entity =>
        {
            entity.ToTable("adguard_connections");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Name).HasMaxLength(128);
        });

        modelBuilder.Entity<AdGuardRewriteEntity>(entity =>
        {
            entity.ToTable("adguard_rewrites");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Domain).HasMaxLength(256);
            entity.HasIndex(x => new { x.ConnectionId, x.Domain }).IsUnique();
        });

        modelBuilder.Entity<NotificationProviderEntity>(entity =>
        {
            entity.ToTable("notification_providers");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Name).HasMaxLength(128);
            entity.Property(x => x.Type).HasMaxLength(32);
        });

        modelBuilder.Entity<ScriptEntity>(entity =>
        {
            entity.ToTable("scripts");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Name).HasMaxLength(128);
        });
    }
}
