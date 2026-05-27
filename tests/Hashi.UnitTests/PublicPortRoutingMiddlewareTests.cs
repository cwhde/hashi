using Hashi.Api.Hosting;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Http;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class PublicPortRoutingMiddlewareTests
{
    [Theory]
    [InlineData(HashiPorts.PublicDashboard, "/api/public/apps")]
    [InlineData(HashiPorts.PublicStatus, "/api/public/status")]
    [InlineData(HashiPorts.PublicStatus, "/api/public/status/summary")]
    public async Task Public_ports_allow_only_their_public_api_contract(int port, string path)
    {
        var (context, invoked) = await InvokeMiddlewareAsync(port, path);

        Assert.True(invoked);
        Assert.Equal(StatusCodes.Status204NoContent, context.Response.StatusCode);
    }

    [Theory]
    [InlineData(HashiPorts.PublicDashboard, "/api/resources")]
    [InlineData(HashiPorts.PublicDashboard, "/api/public/status")]
    [InlineData(HashiPorts.PublicDashboard, "/openapi/v1.json")]
    [InlineData(HashiPorts.PublicStatus, "/api/resources")]
    [InlineData(HashiPorts.PublicStatus, "/api/public/apps")]
    [InlineData(HashiPorts.PublicStatus, "/openapi/v1.json")]
    public async Task Public_ports_block_admin_and_cross_port_api_paths(int port, string path)
    {
        var (context, invoked) = await InvokeMiddlewareAsync(port, path);

        Assert.False(invoked);
        Assert.Equal(StatusCodes.Status404NotFound, context.Response.StatusCode);
    }

    [Fact]
    public async Task Disabled_public_dashboard_blocks_page_and_public_api()
    {
        var (pageContext, pageInvoked) = await InvokeMiddlewareAsync(
            HashiPorts.PublicDashboard,
            "/",
            configureSettings: s => s.PublicDashboardEnabled = false);
        var (apiContext, apiInvoked) = await InvokeMiddlewareAsync(
            HashiPorts.PublicDashboard,
            "/api/public/apps",
            configureSettings: s => s.PublicDashboardEnabled = false);

        Assert.False(pageInvoked);
        Assert.Equal(StatusCodes.Status404NotFound, pageContext.Response.StatusCode);
        Assert.False(apiInvoked);
        Assert.Equal(StatusCodes.Status404NotFound, apiContext.Response.StatusCode);
    }

    [Fact]
    public async Task Disabled_public_status_blocks_page_and_public_api()
    {
        var (pageContext, pageInvoked) = await InvokeMiddlewareAsync(
            HashiPorts.PublicStatus,
            "/",
            configureSettings: s => s.PublicStatusEnabled = false);
        var (apiContext, apiInvoked) = await InvokeMiddlewareAsync(
            HashiPorts.PublicStatus,
            "/api/public/status",
            configureSettings: s => s.PublicStatusEnabled = false);

        Assert.False(pageInvoked);
        Assert.Equal(StatusCodes.Status404NotFound, pageContext.Response.StatusCode);
        Assert.False(apiInvoked);
        Assert.Equal(StatusCodes.Status404NotFound, apiContext.Response.StatusCode);
    }

    private static async Task<(DefaultHttpContext Context, bool Invoked)> InvokeMiddlewareAsync(
        int port,
        string path,
        Action<AppSettingsEntity>? configureSettings = null)
    {
        await using var db = CreateDb();
        var settings = new AppSettingsService(db);
        var appSettings = await settings.GetOrCreateAsync();
        configureSettings?.Invoke(appSettings);
        await settings.SaveAsync();

        var context = new DefaultHttpContext();
        context.Connection.LocalPort = port;
        context.Request.Path = path;

        var invoked = false;
        var middleware = new PublicPortRoutingMiddleware(httpContext =>
        {
            invoked = true;
            httpContext.Response.StatusCode = StatusCodes.Status204NoContent;
            return Task.CompletedTask;
        });

        await middleware.InvokeAsync(context, settings);
        return (context, invoked);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
