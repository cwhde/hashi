using Hashi.Contracts.Api;
using Hashi.Infrastructure.Dns;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Sync;

namespace Hashi.Api.Features.Dns;

internal static class DnsRecordEndpoints
{
    public static void Map(RouteGroupBuilder group)
    {
        group.MapGet("/records", async (DnsRecordService records, CancellationToken ct) =>
        {
            var items = await records.ListAsync(ct);
            return TypedResults.Ok(items.Select(ToRecord));
        });

        group.MapPost("/records", async Task<IResult> (
            UpsertDnsRecordRequest request,
            DnsRecordService records,
            SyncOrchestratorService sync,
            CancellationToken ct) =>
        {
            try
            {
                var created = await records.CreateManualAsync(
                    request.ZoneId, request.Name, request.Type, request.Value, request.Ttl,
                    request.Enabled, request.DashboardEnabled, request.DashboardDisplayName,
                    request.MonitoringEnabled, request.MonitoringDisplayName, ct);
                await sync.TriggerImmediateSyncAsync(ct);
                return TypedResults.Ok(ToRecord(created));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });

        group.MapPut("/records/{recordId:guid}", async Task<IResult> (
            Guid recordId,
            UpsertDnsRecordRequest request,
            DnsRecordService records,
            SyncOrchestratorService sync,
            CancellationToken ct) =>
        {
            try
            {
                var updated = await records.UpdateManualAsync(
                    recordId, request.ZoneId, request.Name, request.Type, request.Value, request.Ttl,
                    request.Enabled, request.DashboardEnabled, request.DashboardDisplayName,
                    request.MonitoringEnabled, request.MonitoringDisplayName, ct);
                if (updated is not null)
                {
                    await sync.TriggerImmediateSyncAsync(ct);
                }
                return updated is null
                    ? TypedResults.NotFound(new ApiErrorResponse("Manual DNS record not found."))
                    : TypedResults.Ok(ToRecord(updated));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });

        group.MapDelete("/records/{recordId:guid}", async Task<IResult> (
            Guid recordId,
            DnsRecordService records,
            SyncOrchestratorService sync,
            CancellationToken ct) =>
        {
            var deleted = await records.DeleteManualAsync(recordId, ct);
            if (deleted)
            {
                await sync.TriggerImmediateSyncAsync(ct);
            }
            return deleted
                ? TypedResults.NoContent()
                : TypedResults.NotFound(new ApiErrorResponse("Manual DNS record not found."));
        });
    }

    private static DnsRecordResponse ToRecord(DnsRecordEntity record)
        => new(
            record.Id, record.ZoneId, record.Name, record.Type, record.Value, record.Ttl,
            record.Ownership, record.Enabled, record.DashboardEnabled, record.DashboardDisplayName,
            record.MonitoringEnabled, record.MonitoringDisplayName);
}
