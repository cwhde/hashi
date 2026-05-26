using System.Text.Json;
using Hashi.Core.Setup;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Services;

public sealed class SetupStateService(HashiDbContext db, ILogger<SetupStateService> logger)
{
    public async Task<SetupStateEntity> GetOrCreateAsync(CancellationToken cancellationToken = default)
    {
        var state = await db.SetupStates.SingleOrDefaultAsync(cancellationToken);
        if (state is not null)
        {
            return state;
        }

        state = new SetupStateEntity();
        db.SetupStates.Add(state);
        await db.SaveChangesAsync(cancellationToken);
        logger.LogInformation("Initialized setup state at step {Step}", state.CurrentStep);
        return state;
    }

    public async Task<IReadOnlyList<string>> GetCompletedStepsAsync(CancellationToken cancellationToken = default)
    {
        var state = await GetOrCreateAsync(cancellationToken);
        return JsonSerializer.Deserialize<List<string>>(state.CompletedStepsJson) ?? [];
    }

    public async Task MarkStepCompleteAsync(SetupStep step, CancellationToken cancellationToken = default)
    {
        var state = await GetOrCreateAsync(cancellationToken);
        var completed = JsonSerializer.Deserialize<List<string>>(state.CompletedStepsJson) ?? [];
        var slug = SetupStepNames.ToSlug(step);
        if (!completed.Contains(slug, StringComparer.Ordinal))
        {
            completed.Add(slug);
        }

        state.CompletedStepsJson = JsonSerializer.Serialize(completed);
        var next = (SetupStep)Math.Min((int)step + 1, (int)SetupStep.Complete);
        state.CurrentStep = SetupStepNames.ToSlug(next);
        state.UpdatedAtUtc = DateTimeOffset.UtcNow;
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task MarkCompleteAsync(CancellationToken cancellationToken = default)
    {
        var state = await GetOrCreateAsync(cancellationToken);
        state.IsComplete = true;
        state.CurrentStep = SetupStepNames.ToSlug(SetupStep.Complete);
        state.BootstrapUsername = null;
        state.BootstrapPasswordHash = null;
        state.UpdatedAtUtc = DateTimeOffset.UtcNow;
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task MarkHttpsVerifiedAsync(CancellationToken cancellationToken = default)
    {
        var state = await GetOrCreateAsync(cancellationToken);
        state.HttpsDomainVerifiedAtUtc = DateTimeOffset.UtcNow;
        state.UpdatedAtUtc = DateTimeOffset.UtcNow;
        await db.SaveChangesAsync(cancellationToken);
    }
}

public sealed class AuditService(HashiDbContext db)
{
    public async Task WriteAsync(
        string category,
        string action,
        string outcome = "success",
        string? subjectType = null,
        string? subjectId = null,
        object? metadata = null,
        CancellationToken cancellationToken = default)
    {
        db.AuditEvents.Add(new AuditEventEntity
        {
            Category = category,
            Action = action,
            Outcome = outcome,
            SubjectType = subjectType,
            SubjectId = subjectId,
            MetadataJson = metadata is null ? null : JsonSerializer.Serialize(metadata),
            CreatedAtUtc = DateTimeOffset.UtcNow,
        });
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task<IReadOnlyList<AuditEventEntity>> ListRecentAsync(int limit = 50, CancellationToken cancellationToken = default)
    {
        return await db.AuditEvents
            .OrderByDescending(x => x.CreatedAtUtc)
            .Take(limit)
            .AsNoTracking()
            .ToListAsync(cancellationToken);
    }
}

public sealed class AppSettingsService(HashiDbContext db)
{
    public async Task<AppSettingsEntity> GetOrCreateAsync(CancellationToken cancellationToken = default)
    {
        var settings = await db.AppSettings.SingleOrDefaultAsync(cancellationToken);
        if (settings is not null)
        {
            return settings;
        }

        settings = new AppSettingsEntity();
        db.AppSettings.Add(settings);
        await db.SaveChangesAsync(cancellationToken);
        return settings;
    }

    public async Task SaveAsync(CancellationToken cancellationToken = default)
    {
        await db.SaveChangesAsync(cancellationToken);
    }
}
