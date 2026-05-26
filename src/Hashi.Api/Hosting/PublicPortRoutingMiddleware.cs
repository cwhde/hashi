using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Persistence;
using Microsoft.AspNetCore.Http;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Api.Hosting;

public sealed class PublicPortRoutingMiddleware(RequestDelegate next)
{
    public async Task InvokeAsync(HttpContext context, MonitoringService monitoring, ResourceService resources)
    {
        var port = context.Connection.LocalPort;
        if (port == 8081)
        {
            var items = await resources.ListAsync(context.RequestAborted);
            var payload = items.Where(x => x.DashboardEnabled).Select(ResourceService.ToResponse);
            context.Response.ContentType = "application/json";
            await context.Response.WriteAsJsonAsync(payload);
            return;
        }

        if (port == 8082)
        {
            var status = await monitoring.PublicStatusAsync(context.RequestAborted);
            context.Response.ContentType = "application/json";
            await context.Response.WriteAsJsonAsync(status);
            return;
        }

        await next(context);
    }
}
