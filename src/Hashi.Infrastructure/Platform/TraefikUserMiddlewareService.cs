using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Traefik;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class TraefikUserMiddlewareService(HashiDbContext db)
{
    private static readonly Guid SingletonId = Guid.Parse("00000000-0000-0000-0000-000000000001");

    public async Task<TraefikUserMiddlewareResponse> GetAsync(CancellationToken cancellationToken = default)
    {
        var entity = await GetOrCreateAsync(cancellationToken);
        var parsed = TraefikUserMiddlewareParser.Parse(entity.LastValidYaml ?? entity.Yaml);
        return ToResponse(entity, parsed.MiddlewareNames);
    }

    public async Task<TraefikUserMiddlewareResponse> UpdateAsync(
        UpdateTraefikUserMiddlewareRequest request,
        CancellationToken cancellationToken = default)
    {
        var entity = await GetOrCreateAsync(cancellationToken);
        var parsed = TraefikUserMiddlewareParser.Parse(request.Yaml);
        if (!parsed.IsValid)
        {
            entity.LastParseError = parsed.Error;
            entity.UpdatedAtUtc = DateTimeOffset.UtcNow;
            await db.SaveChangesAsync(cancellationToken);
            throw new InvalidOperationException(parsed.Error ?? "Invalid user middleware YAML.");
        }

        entity.Yaml = parsed.NormalizedYaml;
        entity.LastValidYaml = parsed.NormalizedYaml;
        entity.LastParseError = null;
        entity.UpdatedAtUtc = DateTimeOffset.UtcNow;
        await db.SaveChangesAsync(cancellationToken);
        return ToResponse(entity, parsed.MiddlewareNames);
    }

    public TraefikUserMiddlewareValidationResponse Validate(string yaml)
    {
        var parsed = TraefikUserMiddlewareParser.Parse(yaml);
        return new TraefikUserMiddlewareValidationResponse(parsed.IsValid, parsed.Error, parsed.MiddlewareNames);
    }

    public async Task<string> GetAppliedYamlAsync(CancellationToken cancellationToken = default)
    {
        var entity = await GetOrCreateAsync(cancellationToken);
        return entity.LastValidYaml ?? entity.Yaml;
    }

    private async Task<TraefikUserMiddlewareEntity> GetOrCreateAsync(CancellationToken cancellationToken)
    {
        var entity = await db.TraefikUserMiddlewares.SingleOrDefaultAsync(x => x.Id == SingletonId, cancellationToken);
        if (entity is not null)
        {
            return entity;
        }

        entity = new TraefikUserMiddlewareEntity { Id = SingletonId };
        db.TraefikUserMiddlewares.Add(entity);
        await db.SaveChangesAsync(cancellationToken);
        return entity;
    }

    private static TraefikUserMiddlewareResponse ToResponse(
        TraefikUserMiddlewareEntity entity,
        IReadOnlyList<string> middlewareNames)
        => new(entity.Yaml, entity.LastParseError, middlewareNames, entity.UpdatedAtUtc);

    public static IReadOnlyList<string> ParseExtraMiddlewares(string? json)
    {
        if (string.IsNullOrWhiteSpace(json))
        {
            return [];
        }

        try
        {
            return JsonSerializer.Deserialize<List<string>>(json) ?? [];
        }
        catch
        {
            return [];
        }
    }
}
