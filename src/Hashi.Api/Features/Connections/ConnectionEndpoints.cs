using FluentValidation;
using Hashi.Api.Hosting;
using Hashi.Contracts.Api;
using Hashi.Core.Connections;
using Hashi.Infrastructure.Connections;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Sync;
using Microsoft.AspNetCore.Http.HttpResults;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Api.Features.Connections;

public static class ConnectionEndpoints
{
    public static IEndpointRouteBuilder MapConnectionEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/connections").WithTags("Connections");

        group.MapGet("/", async (string? type, SshConnectionService connections, CancellationToken ct) =>
        {
            var items = await connections.ListAsync(type, ct);
            return TypedResults.Ok(items.Select(x => new ConnectionSummaryResponse(
                x.Id, x.Name, x.Type, x.Enabled, x.HealthState, x.LastValidationMessage, x.LastValidatedAtUtc)));
        });

        group.MapPost("/ssh", async Task<IResult> (
            CreateSshConnectionRequest request,
            IValidator<CreateSshConnectionRequest> validator,
            SshConnectionService connections,
            SyncOrchestratorService sync,
            CancellationToken ct) =>
        {
            var validationErrors = await validator!.ValidateRequestAsync(request, ct);
            if (validationErrors is not null)
            {
                return TypedResults.ValidationProblem(validationErrors);
            }

            var settings = new SshConnectionSettings(
                request.Host,
                request.Port <= 0 ? 22 : request.Port,
                request.Username,
                OsFamily.Unknown,
                null,
                null);
            ConnectionEntity connection;
            try
            {
                connection = await connections.CreateAsync(
                    request.Name,
                    request.ConnectionType,
                    settings,
                    request.AuthMode,
                    request.Password,
                    request.PrivateKeyPem,
                    request.PrivateKeyPassphrase,
                    request.Target,
                    ct);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new { error = ex.Message });
            }

            await sync.TriggerImmediateSyncAsync(ct);

            return TypedResults.Ok(new ConnectionSummaryResponse(
                connection.Id, connection.Name, connection.Type, connection.Enabled,
                connection.HealthState, connection.LastValidationMessage, connection.LastValidatedAtUtc));
        });

        group.MapPost("/{connectionId:guid}/validate", async Task<IResult> (
            Guid connectionId,
            SshConnectionService connections,
            CancellationToken ct) =>
        {
            var result = await connections.ValidateAsync(connectionId, ct);
            return TypedResults.Ok(new SshValidationResponse(
                result.Succeeded,
                result.OsFamily.ToString(),
                result.PackageManager,
                result.Error));
        });

        group.MapPost("/{connectionId:guid}/write", async Task<IResult> (
            Guid connectionId,
            RemoteWriteRequest request,
            IValidator<RemoteWriteRequest> validator,
            SshConnectionService connections,
            CancellationToken ct) =>
        {
            var validationErrors = await validator!.ValidateRequestAsync(request, ct);
            if (validationErrors is not null)
            {
                return TypedResults.ValidationProblem(validationErrors);
            }

            var settings = new SshConnectionSettings(
                request.Host,
                request.Port <= 0 ? 22 : request.Port,
                request.Username,
                OsFamily.Unknown,
                null,
                null);
            var content = Convert.FromBase64String(request.ContentBase64);
            var result = await connections.WriteAtomicAsync(
                connectionId,
                settings,
                request.AuthMode,
                request.Password,
                request.PrivateKeyPem,
                request.PrivateKeyPassphrase,
                request.RemotePath,
                content,
                ct);
            return TypedResults.Ok(new RemoteWriteResponse(result.Succeeded, result.RemotePath, result.Error));
        });

        group.MapDelete("/{connectionId:guid}", async Task<IResult> (
            Guid connectionId,
            HashiDbContext db,
            SyncOrchestratorService sync,
            CancellationToken ct) =>
        {
            var connection = await db.Connections.SingleOrDefaultAsync(x => x.Id == connectionId, ct);
            if (connection is null)
            {
                return TypedResults.NotFound();
            }

            if (connection.DeletionPolicy == ConnectionDeletionPolicyNames.Required)
            {
                return TypedResults.BadRequest(new { error = "This connection has a required deletion policy and cannot be deleted." });
            }

            var setupComplete = await db.SetupStates.AnyAsync(x => x.IsComplete, ct);
            if (setupComplete)
            {
                var typeCount = await db.Connections
                    .Where(x => x.Type == connection.Type && x.Enabled && x.Id != connectionId)
                    .CountAsync(ct);
                var minimums = new Dictionary<string, int>
                {
                    [ConnectionTypeNames.DnsProvider] = 1,
                    [ConnectionTypeNames.TraefikHost] = 1,
                    [ConnectionTypeNames.FirewallHost] = 1,
                };
                if (minimums.TryGetValue(connection.Type, out var min) && typeCount < min)
                {
                    return TypedResults.BadRequest(new { error = $"Cannot delete the last {connection.Type} connection. At least {min} {connection.Type} connection(s) required after setup." });
                }
            }

            db.Connections.Remove(connection);
            await db.SaveChangesAsync(ct);
            await sync.TriggerImmediateSyncAsync(ct);
            return TypedResults.Ok(new { deleted = true });
        });

        return app;
    }
}
