using FluentValidation;
using Hashi.Api.Hosting;
using Hashi.Contracts.Api;
using Hashi.Core.Connections;
using Hashi.Infrastructure.Connections;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.AspNetCore.Http.HttpResults;

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

        return app;
    }
}
