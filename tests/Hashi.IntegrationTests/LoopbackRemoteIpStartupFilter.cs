using System.Net;
using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Hosting;

namespace Hashi.IntegrationTests;

internal sealed class LoopbackRemoteIpStartupFilter : IStartupFilter
{
    public Action<IApplicationBuilder> Configure(Action<IApplicationBuilder> next)
        => app =>
        {
            app.Use(async (context, nextMiddleware) =>
            {
                context.Connection.RemoteIpAddress ??= IPAddress.Loopback;
                if (context.Request.Host.Port is int port)
                {
                    context.Connection.LocalPort = port;
                }

                await nextMiddleware();
            });
            next(app);
        };
}
