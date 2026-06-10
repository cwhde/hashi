using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.HttpResults;
using Microsoft.AspNetCore.Routing;

namespace Hashi.Api.Features.Waf;

public static class WafEndpoints
{
    public static IEndpointRouteBuilder MapWafEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/api/waf/{slug}/middleware", (string slug) =>
        {
            var yaml = Hashi.Core.Security.WafMiddlewareRenderer.RenderCorazaMiddleware(slug, Hashi.Core.Security.WafMode.On);
            return TypedResults.Ok(new { slug, yaml });
        }).WithTags("Security");
        return app;
    }
}
