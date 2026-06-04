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

    public DbSet<ConnectionHealthEntity> ConnectionHealth => Set<ConnectionHealthEntity>();

    public DbSet<DnsZoneEntity> DnsZones => Set<DnsZoneEntity>();

    public DbSet<DnsRecordEntity> DnsRecords => Set<DnsRecordEntity>();

    public DbSet<DnsRecordOwnershipEntity> DnsRecordOwnership => Set<DnsRecordOwnershipEntity>();

    public DbSet<DnsImportDecisionEntity> DnsImportDecisions => Set<DnsImportDecisionEntity>();

    public DbSet<ResourceEntity> Resources => Set<ResourceEntity>();

    public DbSet<ResourceRouteEntity> ResourceRoutes => Set<ResourceRouteEntity>();

    public DbSet<ResourceTargetEntity> ResourceTargets => Set<ResourceTargetEntity>();

    public DbSet<ResourceRuleEntity> ResourceRules => Set<ResourceRuleEntity>();

    public DbSet<ResourcePortEntity> ResourcePorts => Set<ResourcePortEntity>();

    public DbSet<SystemResourceEntity> SystemResources => Set<SystemResourceEntity>();

    public DbSet<TraefikEntryPointEntity> TraefikEntryPoints => Set<TraefikEntryPointEntity>();

    public DbSet<MonitorEventEntity> MonitorEvents => Set<MonitorEventEntity>();

    public DbSet<SecurityEventEntity> SecurityEvents => Set<SecurityEventEntity>();

    public DbSet<EdgeSessionEntity> EdgeSessions => Set<EdgeSessionEntity>();

    public DbSet<MonitorEndpointEntity> MonitorEndpoints => Set<MonitorEndpointEntity>();

    public DbSet<PulseAgentEntity> PulseAgents => Set<PulseAgentEntity>();

    public DbSet<PulseHeartbeatEntity> PulseHeartbeats => Set<PulseHeartbeatEntity>();

    public DbSet<TraefikHostStateEntity> TraefikHostStates => Set<TraefikHostStateEntity>();

    public DbSet<TraefikUserMiddlewareEntity> TraefikUserMiddlewares => Set<TraefikUserMiddlewareEntity>();

    public DbSet<FirewallHostEntity> FirewallHosts => Set<FirewallHostEntity>();

    public DbSet<FirewallSubnetEntity> FirewallSubnets => Set<FirewallSubnetEntity>();

    public DbSet<FirewallPortEntity> FirewallPorts => Set<FirewallPortEntity>();

    public DbSet<FirewallAllowedSubjectEntity> FirewallAllowedSubjects => Set<FirewallAllowedSubjectEntity>();

    public DbSet<FirewallBlockSubjectEntity> FirewallBlockSubjects => Set<FirewallBlockSubjectEntity>();

    public DbSet<FirewallGeneratedScriptEntity> FirewallGeneratedScripts => Set<FirewallGeneratedScriptEntity>();

    public DbSet<MonitorSampleEntity> MonitorSamples => Set<MonitorSampleEntity>();

    public DbSet<MonitorRollupEntity> MonitorRollups => Set<MonitorRollupEntity>();

    public DbSet<OidcProviderEntity> OidcProviders => Set<OidcProviderEntity>();

    public DbSet<EdgeAuthRuleEntity> EdgeAuthRules => Set<EdgeAuthRuleEntity>();

    public DbSet<AccessLogEventEntity> AccessLogEvents => Set<AccessLogEventEntity>();

    public DbSet<AbuseBucketEntity> AbuseBuckets => Set<AbuseBucketEntity>();

    public DbSet<AccessLogCursorEntity> AccessLogCursors => Set<AccessLogCursorEntity>();
    public DbSet<SecurityRequestBucketEntity> SecurityRequestBuckets => Set<SecurityRequestBucketEntity>();

    public DbSet<BlocklistEntryEntity> BlocklistEntries => Set<BlocklistEntryEntity>();

    public DbSet<BlocklistAppliedHostEntity> BlocklistAppliedHosts => Set<BlocklistAppliedHostEntity>();

    public DbSet<AdGuardConnectionEntity> AdGuardConnections => Set<AdGuardConnectionEntity>();

    public DbSet<AdGuardRewriteEntity> AdGuardRewrites => Set<AdGuardRewriteEntity>();

    public DbSet<NotificationProviderEntity> NotificationProviders => Set<NotificationProviderEntity>();

    public DbSet<NotificationRouteEntity> NotificationRoutes => Set<NotificationRouteEntity>();

    public DbSet<NotificationDeliveryEntity> NotificationDeliveries => Set<NotificationDeliveryEntity>();

    public DbSet<ScriptEntity> Scripts => Set<ScriptEntity>();

    public DbSet<ScriptTargetEntity> ScriptTargets => Set<ScriptTargetEntity>();

    public DbSet<ScriptEnvironmentVariableEntity> ScriptEnvironmentVariables => Set<ScriptEnvironmentVariableEntity>();

    public DbSet<ScriptRunEntity> ScriptRuns => Set<ScriptRunEntity>();

    public DbSet<ScriptOutputEntity> ScriptOutputs => Set<ScriptOutputEntity>();

    public DbSet<BackgroundJobEntity> BackgroundJobs => Set<BackgroundJobEntity>();

    public DbSet<GeoIpDatabaseEntity> GeoIpDatabases => Set<GeoIpDatabaseEntity>();

    protected override void OnModelCreating(ModelBuilder modelBuilder)
    {
        modelBuilder.Entity<AppSettingsEntity>(entity =>
        {
            entity.ToTable("app_settings");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Theme).HasMaxLength(32);
            entity.Property(x => x.AcmeEmail).HasMaxLength(256);
            entity.Property(x => x.GeoIpEnabled).HasDefaultValue(false);
            entity.Property(x => x.GeoIpAccountId).HasMaxLength(128);
            entity.Property(x => x.GeoIpUpdateIntervalHours).HasDefaultValue(72);
            entity.Property(x => x.GeoIpLastUpdateStatus).HasMaxLength(32).HasDefaultValue(GeoIpUpdateStatusNames.NeverRun);
            entity.HasIndex(x => x.AcmeDnsProviderConnectionId);
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
            entity.Property(x => x.IsServiceSyncEligible).HasDefaultValue(false);
            entity.HasIndex(x => x.Purpose);
            entity.HasIndex(x => x.IsServiceSyncEligible);
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

        modelBuilder.Entity<ConnectionHealthEntity>(entity =>
        {
            entity.ToTable("connection_health");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.State).HasMaxLength(32);
            entity.Property(x => x.CheckKind).HasMaxLength(64);
            entity.HasOne(x => x.Connection).WithMany().HasForeignKey(x => x.ConnectionId).OnDelete(DeleteBehavior.Cascade);
            entity.HasIndex(x => new { x.ConnectionId, x.CheckedAtUtc });
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
            entity.Property(x => x.DashboardDisplayName).HasMaxLength(128);
            entity.HasOne(x => x.Zone).WithMany().HasForeignKey(x => x.ZoneId);
            entity.HasIndex(x => x.ZoneId);
        });

        modelBuilder.Entity<DnsRecordOwnershipEntity>(entity =>
        {
            entity.ToTable("dns_record_ownership");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.ProviderRecordId).HasMaxLength(128);
            entity.Property(x => x.Name).HasMaxLength(256);
            entity.Property(x => x.Type).HasMaxLength(16);
            entity.Property(x => x.Ownership).HasMaxLength(32);
            entity.Property(x => x.OwnerWorkflow).HasMaxLength(64);
            entity.Property(x => x.SyncState).HasMaxLength(32);
            entity.HasOne(x => x.Zone).WithMany().HasForeignKey(x => x.ZoneId).OnDelete(DeleteBehavior.Cascade);
            entity.HasOne(x => x.DnsRecord).WithMany().HasForeignKey(x => x.DnsRecordId).OnDelete(DeleteBehavior.SetNull);
            entity.HasOne(x => x.Resource).WithMany().HasForeignKey(x => x.ResourceId).OnDelete(DeleteBehavior.SetNull);
            entity.HasIndex(x => new { x.ZoneId, x.Name, x.Type, x.Value }).IsUnique();
            entity.HasIndex(x => x.ProviderRecordId);
            entity.HasIndex(x => x.ResourceId);
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
            entity.Property(x => x.Ownership).HasMaxLength(32).HasDefaultValue(ResourceOwnershipNames.UserCreated);
            entity.Property(x => x.OwningWorkflow).HasMaxLength(64);
            entity.Property(x => x.DeletionPolicy).HasMaxLength(32).HasDefaultValue(ResourceDeletionPolicyNames.Optional);
            entity.Property(x => x.SyncState).HasMaxLength(32).HasDefaultValue(ResourceSyncStateNames.Desired);
            entity.Property(x => x.DomainMode).HasMaxLength(32).HasDefaultValue("custom");
            entity.Property(x => x.MonitoringProtocolHint).HasMaxLength(16);
            entity.Property(x => x.ForwardAuthPolicy).HasMaxLength(32);
            entity.Property(x => x.WafMode).HasMaxLength(32);
            entity.Property(x => x.PathRewriteMode).HasMaxLength(32);
            entity.Property(x => x.WafExclusionsJson);
            entity.HasIndex(x => x.Slug).IsUnique();
        });

        modelBuilder.Entity<ResourceRouteEntity>(entity =>
        {
            entity.ToTable("resource_routes");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.PathMatchType).HasMaxLength(16);
            entity.HasIndex(x => x.ResourceId);
        });

        modelBuilder.Entity<ResourceTargetEntity>(entity =>
        {
            entity.ToTable("resource_targets");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Scheme).HasMaxLength(16);
            entity.Property(x => x.Host).HasMaxLength(256);
            entity.HasOne(x => x.Resource).WithMany().HasForeignKey(x => x.ResourceId).OnDelete(DeleteBehavior.Cascade);
            entity.HasOne(x => x.FirewallHost).WithMany().HasForeignKey(x => x.FirewallHostId).OnDelete(DeleteBehavior.SetNull);
            entity.HasIndex(x => new { x.ResourceId, x.Priority });
        });

        modelBuilder.Entity<ResourceRuleEntity>(entity =>
        {
            entity.ToTable("resource_rules");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Action).HasMaxLength(32);
            entity.Property(x => x.MatchType).HasMaxLength(32);
            entity.HasIndex(x => x.ResourceId);
        });

        modelBuilder.Entity<ResourcePortEntity>(entity =>
        {
            entity.ToTable("resource_ports");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Protocol).HasMaxLength(8);
            entity.Property(x => x.Ownership).HasMaxLength(32);
            entity.HasOne(x => x.Resource).WithMany().HasForeignKey(x => x.ResourceId).OnDelete(DeleteBehavior.Cascade);
            entity.HasIndex(x => new { x.PublicPort, x.Protocol }).IsUnique();
            entity.HasIndex(x => x.ResourceId);
        });

        modelBuilder.Entity<SystemResourceEntity>(entity =>
        {
            entity.ToTable("system_resources");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.SystemKey).HasMaxLength(128);
            entity.Property(x => x.OwningWorkflow).HasMaxLength(64);
            entity.HasOne(x => x.Resource).WithMany().HasForeignKey(x => x.ResourceId).OnDelete(DeleteBehavior.Cascade);
            entity.HasIndex(x => x.ResourceId).IsUnique();
            entity.HasIndex(x => x.SystemKey).IsUnique();
        });

        modelBuilder.Entity<TraefikEntryPointEntity>(entity =>
        {
            entity.ToTable("traefik_entrypoints");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Protocol).HasMaxLength(8);
            entity.HasIndex(x => new { x.Port, x.Protocol }).IsUnique();
        });

        modelBuilder.Entity<MonitorEventEntity>(entity =>
        {
            entity.ToTable("monitor_events");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.PreviousStatus).HasMaxLength(16);
            entity.Property(x => x.NewStatus).HasMaxLength(16);
            entity.HasIndex(x => x.OccurredAtUtc);
            entity.HasIndex(x => x.MonitorEndpointId);
        });

        modelBuilder.Entity<SecurityEventEntity>(entity =>
        {
            entity.ToTable("security_events");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Category).HasMaxLength(32);
            entity.Property(x => x.Action).HasMaxLength(64);
            entity.HasIndex(x => x.OccurredAtUtc);
        });

        modelBuilder.Entity<EdgeSessionEntity>(entity =>
        {
            entity.ToTable("edge_sessions");
            entity.HasKey(x => x.SessionKey);
            entity.Property(x => x.SessionKey).HasMaxLength(128);
            entity.Property(x => x.Subject).HasMaxLength(256);
            entity.HasIndex(x => x.ExpiresAtUtc);
            entity.HasIndex(x => x.LastSeenAtUtc);
        });

        modelBuilder.Entity<MonitorEndpointEntity>(entity =>
        {
            entity.ToTable("monitor_endpoints");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Name).HasMaxLength(128);
            entity.Property(x => x.CheckType).HasMaxLength(16);
            entity.Property(x => x.Status).HasMaxLength(16);
            entity.Property(x => x.PublicStatusEnabled).HasDefaultValue(false);
        });

        modelBuilder.Entity<PulseAgentEntity>(entity =>
        {
            entity.ToTable("pulse_agents");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Name).HasMaxLength(128);
            entity.Property(x => x.Status).HasMaxLength(32);
            entity.Property(x => x.TokenHash).HasMaxLength(128);
            entity.Property(x => x.InstallType).HasMaxLength(32).HasDefaultValue("linux_service");
            entity.Property(x => x.AllowedScopesJson).HasColumnType("jsonb").HasDefaultValueSql("'[\"heartbeat\"]'::jsonb");
            entity.Property(x => x.HeartbeatIntervalSeconds).HasDefaultValue(60);
            entity.Property(x => x.LastPrivateIpv4CandidatesJson).HasColumnType("jsonb").HasDefaultValueSql("'[]'::jsonb");
            entity.Property(x => x.LastPrivateIpv6CandidatesJson).HasColumnType("jsonb").HasDefaultValueSql("'[]'::jsonb");
            entity.Property(x => x.LastSelectedInterface).HasMaxLength(128);
            entity.Property(x => x.LastDockerMetadataJson).HasColumnType("jsonb");
            entity.HasIndex(x => x.Status);
        });

        modelBuilder.Entity<PulseHeartbeatEntity>(entity =>
        {
            entity.ToTable("pulse_heartbeats");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Version).HasMaxLength(64);
            entity.Property(x => x.Hostname).HasMaxLength(256);
            entity.Property(x => x.PrivateIpv4CandidatesJson).HasColumnType("jsonb");
            entity.Property(x => x.PrivateIpv6CandidatesJson).HasColumnType("jsonb");
            entity.Property(x => x.SelectedInterface).HasMaxLength(128);
            entity.Property(x => x.DockerMetadataJson).HasColumnType("jsonb");
            entity.HasOne(x => x.PulseAgent)
                .WithMany()
                .HasForeignKey(x => x.PulseAgentId)
                .OnDelete(DeleteBehavior.Cascade);
            entity.HasIndex(x => new { x.PulseAgentId, x.ReceivedAtUtc });
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

        modelBuilder.Entity<FirewallSubnetEntity>(entity =>
        {
            entity.ToTable("firewall_subnets");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Cidr).HasMaxLength(128);
            entity.Property(x => x.Purpose).HasMaxLength(32);
            entity.Property(x => x.Ownership).HasMaxLength(32);
            entity.HasOne(x => x.FirewallHost).WithMany().HasForeignKey(x => x.FirewallHostId).OnDelete(DeleteBehavior.Cascade);
            entity.HasIndex(x => new { x.FirewallHostId, x.Cidr, x.Purpose }).IsUnique();
        });

        modelBuilder.Entity<FirewallPortEntity>(entity =>
        {
            entity.ToTable("firewall_ports");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Protocol).HasMaxLength(8);
            entity.Property(x => x.TargetHost).HasMaxLength(256);
            entity.Property(x => x.Ownership).HasMaxLength(32);
            entity.HasOne(x => x.FirewallHost).WithMany().HasForeignKey(x => x.FirewallHostId).OnDelete(DeleteBehavior.Cascade);
            entity.HasOne(x => x.Resource).WithMany().HasForeignKey(x => x.ResourceId).OnDelete(DeleteBehavior.SetNull);
            entity.HasIndex(x => new { x.FirewallHostId, x.PublicPort, x.Protocol }).IsUnique();
            entity.HasIndex(x => x.ResourceId);
        });

        modelBuilder.Entity<FirewallAllowedSubjectEntity>(entity =>
        {
            entity.ToTable("firewall_allowed_subjects");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.SubjectKind).HasMaxLength(32);
            entity.Property(x => x.SubjectValue).HasMaxLength(256);
            entity.Property(x => x.Ownership).HasMaxLength(32);
            entity.HasOne(x => x.FirewallHost).WithMany().HasForeignKey(x => x.FirewallHostId).OnDelete(DeleteBehavior.Cascade);
            entity.HasIndex(x => new { x.FirewallHostId, x.SubjectKind, x.SubjectValue }).IsUnique();
        });

        modelBuilder.Entity<FirewallBlockSubjectEntity>(entity =>
        {
            entity.ToTable("firewall_block_subjects");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.SubjectKind).HasMaxLength(32);
            entity.Property(x => x.SubjectValue).HasMaxLength(256);
            entity.Property(x => x.Ownership).HasMaxLength(32);
            entity.HasOne(x => x.FirewallHost).WithMany().HasForeignKey(x => x.FirewallHostId).OnDelete(DeleteBehavior.Cascade);
            entity.HasOne(x => x.BlocklistEntry).WithMany().HasForeignKey(x => x.BlocklistEntryId).OnDelete(DeleteBehavior.SetNull);
            entity.HasIndex(x => new { x.FirewallHostId, x.SubjectKind, x.SubjectValue }).IsUnique();
            entity.HasIndex(x => x.BlocklistEntryId);
        });

        modelBuilder.Entity<FirewallGeneratedScriptEntity>(entity =>
        {
            entity.ToTable("firewall_generated_scripts");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.ScriptPath).HasMaxLength(512);
            entity.Property(x => x.DesiredContentHash).HasMaxLength(128);
            entity.Property(x => x.AppliedContentHash).HasMaxLength(128);
            entity.Property(x => x.Status).HasMaxLength(32);
            entity.HasOne(x => x.FirewallHost).WithMany().HasForeignKey(x => x.FirewallHostId).OnDelete(DeleteBehavior.Cascade);
            entity.HasOne(x => x.SyncRun).WithMany().HasForeignKey(x => x.SyncRunId).OnDelete(DeleteBehavior.SetNull);
            entity.HasIndex(x => new { x.FirewallHostId, x.CreatedAtUtc });
            entity.HasIndex(x => x.SyncRunId);
        });

        modelBuilder.Entity<MonitorSampleEntity>(entity =>
        {
            entity.ToTable("monitor_samples_raw");
            entity.HasKey(x => new { x.Id, x.PartitionDate });
            entity.Property(x => x.PartitionDate).HasColumnName("partition_date");
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

        modelBuilder.Entity<AccessLogCursorEntity>(entity =>
        {
            entity.ToTable("access_log_cursors");
            entity.HasKey(x => x.Id);
            entity.HasIndex(x => x.ConnectionId).IsUnique();
        });

        modelBuilder.Entity<SecurityRequestBucketEntity>(entity =>
        {
            entity.ToTable("security_request_buckets");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.ClientIp).HasMaxLength(64);
            entity.Property(x => x.Resource).HasMaxLength(256);
            entity.Property(x => x.TraefikInstance).HasMaxLength(128);
            entity.Property(x => x.CountryCode).HasMaxLength(32);
            entity.Property(x => x.RegionCode).HasMaxLength(64);
            entity.Property(x => x.Asn).HasMaxLength(64);
            entity.Property(x => x.Method).HasMaxLength(16);
            entity.Property(x => x.PathPrefix).HasMaxLength(256);
            entity.HasIndex(x => x.BucketStartUtc);
            entity.HasIndex(x => x.ClientIp);
            entity.HasIndex(x => new
            {
                x.BucketStartUtc,
                x.ClientIp,
                x.Resource,
                x.TraefikInstance,
                x.CountryCode,
                x.RegionCode,
                x.Asn,
                x.StatusClass,
                x.Method,
                x.PathPrefix,
            }).IsUnique();
        });

        modelBuilder.Entity<BlocklistEntryEntity>(entity =>
        {
            entity.ToTable("blocklist_entries");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.ClientIp).HasMaxLength(64);
            entity.Property(x => x.Scope).HasMaxLength(32);
            entity.Property(x => x.Type).HasMaxLength(32);
            entity.Property(x => x.Value).HasMaxLength(128);
            entity.Property(x => x.Reason).HasMaxLength(256);
            entity.Property(x => x.Source).HasMaxLength(64);
            entity.Property(x => x.CreatedBy).HasMaxLength(128);
            entity.HasIndex(x => new { x.Scope, x.Type, x.Value });
            entity.HasIndex(x => x.ExpiresAtUtc);
        });

        modelBuilder.Entity<BlocklistAppliedHostEntity>(entity =>
        {
            entity.ToTable("blocklist_applied_hosts");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Status).HasMaxLength(32);
            entity.HasOne(x => x.BlocklistEntry).WithMany().HasForeignKey(x => x.BlocklistEntryId).OnDelete(DeleteBehavior.Cascade);
            entity.HasOne(x => x.FirewallHost).WithMany().HasForeignKey(x => x.FirewallHostId).OnDelete(DeleteBehavior.Cascade);
            entity.HasIndex(x => new { x.BlocklistEntryId, x.FirewallHostId }).IsUnique();
            entity.HasIndex(x => x.FirewallHostId);
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
            entity.Property(x => x.Source).HasMaxLength(32);
            entity.HasIndex(x => new { x.ConnectionId, x.Domain }).IsUnique();
        });

        modelBuilder.Entity<NotificationProviderEntity>(entity =>
        {
            entity.ToTable("notification_providers");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Name).HasMaxLength(128);
            entity.Property(x => x.Type).HasMaxLength(32);
        });

        modelBuilder.Entity<NotificationRouteEntity>(entity =>
        {
            entity.ToTable("notification_routes");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Name).HasMaxLength(128);
            entity.Property(x => x.EventKind).HasMaxLength(64);
            entity.Property(x => x.Severity).HasMaxLength(32);
            entity.Property(x => x.CooldownMinutes).HasDefaultValue(0);
            entity.Property(x => x.SendRecovery).HasDefaultValue(true);
            entity.HasOne(x => x.Provider).WithMany().HasForeignKey(x => x.ProviderId).OnDelete(DeleteBehavior.Cascade);
            entity.HasIndex(x => new { x.ProviderId, x.EventKind });
        });

        modelBuilder.Entity<NotificationDeliveryEntity>(entity =>
        {
            entity.ToTable("notification_deliveries");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.EventKind).HasMaxLength(64);
            entity.Property(x => x.Subject).HasMaxLength(256);
            entity.Property(x => x.Status).HasMaxLength(32);
            entity.Property(x => x.ProviderMessageId).HasMaxLength(256);
            entity.HasOne(x => x.Route).WithMany().HasForeignKey(x => x.RouteId).OnDelete(DeleteBehavior.SetNull);
            entity.HasOne(x => x.Provider).WithMany().HasForeignKey(x => x.ProviderId).OnDelete(DeleteBehavior.Cascade);
            entity.HasIndex(x => new { x.ProviderId, x.CreatedAtUtc });
            entity.HasIndex(x => new { x.RouteId, x.CreatedAtUtc });
        });

        modelBuilder.Entity<ScriptEntity>(entity =>
        {
            entity.ToTable("scripts");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Name).HasMaxLength(128);
            entity.Property(x => x.LastRunStatus).HasMaxLength(32).HasDefaultValue(ScriptRunStatusNames.NeverRun);
            entity.Property(x => x.RunTimeoutSeconds).HasDefaultValue(300);
            entity.HasIndex(x => x.ConnectionId);
            entity.HasOne<ConnectionEntity>().WithMany().HasForeignKey(x => x.ConnectionId).OnDelete(DeleteBehavior.Restrict);
        });

        modelBuilder.Entity<ScriptTargetEntity>(entity =>
        {
            entity.ToTable("host_script_targets");
            entity.HasKey(x => x.Id);
            entity.HasOne(x => x.Script).WithMany().HasForeignKey(x => x.ScriptId).OnDelete(DeleteBehavior.Cascade);
            entity.HasOne(x => x.Connection).WithMany().HasForeignKey(x => x.ConnectionId).OnDelete(DeleteBehavior.Restrict);
            entity.HasIndex(x => new { x.ScriptId, x.ConnectionId }).IsUnique();
        });

        modelBuilder.Entity<ScriptEnvironmentVariableEntity>(entity =>
        {
            entity.ToTable("host_script_environment_variables");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Name).HasMaxLength(128);
            entity.HasOne(x => x.Script).WithMany().HasForeignKey(x => x.ScriptId).OnDelete(DeleteBehavior.Cascade);
            entity.HasOne<SecretRecordEntity>().WithMany().HasForeignKey(x => x.SecretId).OnDelete(DeleteBehavior.Restrict);
            entity.HasIndex(x => new { x.ScriptId, x.Name }).IsUnique();
        });

        modelBuilder.Entity<ScriptRunEntity>(entity =>
        {
            entity.ToTable("host_script_runs");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Status).HasMaxLength(32);
            entity.HasOne(x => x.Script).WithMany().HasForeignKey(x => x.ScriptId).OnDelete(DeleteBehavior.Cascade);
            entity.HasOne(x => x.Connection).WithMany().HasForeignKey(x => x.ConnectionId).OnDelete(DeleteBehavior.Restrict);
            entity.HasIndex(x => new { x.ScriptId, x.StartedAtUtc });
        });

        modelBuilder.Entity<ScriptOutputEntity>(entity =>
        {
            entity.ToTable("host_script_outputs");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.Stream).HasMaxLength(16);
            entity.HasOne(x => x.Run).WithMany().HasForeignKey(x => x.RunId).OnDelete(DeleteBehavior.Cascade);
            entity.HasIndex(x => x.RunId);
        });

        modelBuilder.Entity<BackgroundJobEntity>(entity =>
        {
            entity.ToTable("background_jobs");
            entity.HasKey(x => x.JobKey);
            entity.Property(x => x.JobKey).HasMaxLength(64);
            entity.Property(x => x.DisplayName).HasMaxLength(128);
            entity.Property(x => x.Status).HasMaxLength(32);
        });

        modelBuilder.Entity<GeoIpDatabaseEntity>(entity =>
        {
            entity.ToTable("geoip_databases");
            entity.HasKey(x => x.Id);
            entity.Property(x => x.EditionId).HasMaxLength(64);
            entity.Property(x => x.FileName).HasMaxLength(128);
            entity.Property(x => x.Path).HasMaxLength(512);
            entity.Property(x => x.Status).HasMaxLength(32).HasDefaultValue(GeoIpUpdateStatusNames.NeverRun);
            entity.Property(x => x.ContentHash).HasMaxLength(128);
            entity.HasIndex(x => x.EditionId).IsUnique();
        });
    }
}
