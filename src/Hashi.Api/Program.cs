using Hashi.Api.Features.Auth;
using Hashi.Api.Features.Connections;
using Hashi.Api.Features.Dns;
using Hashi.Api.Features.Resources;
using Hashi.Api.Features.Setup;
using Hashi.Api.Features.Sync;
using Hashi.Api.Features.Vault;
using Hashi.Api.Hosting;
using Hashi.Infrastructure;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Bootstrap;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Persistence;
using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.OpenApi;
using Microsoft.EntityFrameworkCore;
using Microsoft.OpenApi.Models;
using Serilog;

var builder = WebApplication.CreateBuilder(args);

if (!builder.Environment.IsEnvironment("OpenApiExport"))
{
    builder.WebHost.ConfigureKestrel(options =>
    {
        options.ListenAnyIP(8080);
        options.ListenAnyIP(8081);
        options.ListenAnyIP(8082);
    });
}

builder.Host.UseSerilog((context, services, configuration) =>
    configuration.ReadFrom.Configuration(context.Configuration)
        .ReadFrom.Services(services)
        .Enrich.FromLogContext()
        .WriteTo.Console());

builder.Services.AddOpenApi(options =>
{
    options.AddDocumentTransformer((document, _, _) =>
    {
        document.Info = new OpenApiInfo
        {
            Title = "Hashi API",
            Version = "2.0.0-alpha",
            Description = "Hashi V2 admin and edge API. Frontend clients must consume this contract only.",
        };
        return Task.CompletedTask;
    });
});
builder.Services.AddHashiInfrastructure(builder.Configuration);
builder.Services.AddScoped<BootstrapInitializer>();

builder.Services.AddAuthentication(CookieAuthenticationDefaults.AuthenticationScheme)
    .AddCookie(options =>
    {
        options.Cookie.Name = "hashi.session";
        options.Cookie.HttpOnly = true;
        options.Cookie.SecurePolicy = CookieSecurePolicy.SameAsRequest;
        options.SlidingExpiration = true;
        options.ExpireTimeSpan = TimeSpan.FromHours(8);
    });
builder.Services.AddAuthorization();
builder.Services.AddCors(options =>
{
    options.AddPolicy("PublicRead", policy =>
        policy.SetIsOriginAllowed(static origin =>
        {
            if (!Uri.TryCreate(origin, UriKind.Absolute, out var uri))
            {
                return false;
            }

            return uri.Port is HashiPorts.PublicDashboard or HashiPorts.PublicStatus;
        })
        .AllowAnyHeader()
        .AllowAnyMethod());
});
builder.Services.AddAntiforgery(options =>
{
    options.HeaderName = "X-CSRF-TOKEN";
    options.Cookie.Name = "hashi.csrf";
    options.Cookie.HttpOnly = true;
    options.Cookie.SecurePolicy = CookieSecurePolicy.SameAsRequest;
});

var app = builder.Build();

if (app.Environment.IsDevelopment()
    || app.Environment.EnvironmentName == "OpenApiExport"
    || string.Equals(Environment.GetEnvironmentVariable("HASHI_EXPORT_OPENAPI"), "1", StringComparison.Ordinal))
{
    app.MapOpenApi();
}

app.UseSerilogRequestLogging();
app.UseCors();
app.UseMiddleware<PublicPortRoutingMiddleware>();
app.UseAuthentication();
app.UseAuthorization();
app.UseMiddleware<AdminApiAuthMiddleware>();
app.UseMiddleware<AdminCsrfMiddleware>();
app.UseDefaultFiles();
app.UseStaticFiles();

app.MapHealthEndpoints();
app.MapSetupEndpoints();
app.MapSetupAdvanceEndpoints();
app.MapSetupCompletionEndpoints();
app.MapSettingsEndpoints();
app.MapActivityEndpoints();
app.MapAuthEndpoints();
app.MapVaultEndpoints();
app.MapDnsEndpoints();
app.MapConnectionEndpoints();
app.MapResourceEndpoints();
app.MapTraefikEndpoints();
app.MapFirewallEndpoints();
app.MapStatusEndpoints();
app.MapPublicEndpoints();
app.MapEdgeAuthEndpoints();
app.MapEdgeSsoAdminEndpoints();
app.MapSecurityEndpoints();
app.MapPulseEndpoints();
app.MapScriptEndpoints();
app.MapNotificationEndpoints();
app.MapAdGuardEndpoints();
app.MapWafEndpoints();
app.MapSyncEndpoints();

var skipStartupHooks = builder.Configuration.GetValue<bool>("Hashi:SkipStartupHooks")
    || string.Equals(Environment.GetEnvironmentVariable("HASHI_SKIP_STARTUP_HOOKS"), "1", StringComparison.Ordinal);
if (!skipStartupHooks)
{
    using var scope = app.Services.CreateScope();
    var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
    await db.Database.MigrateAsync();
    await scope.ServiceProvider.GetRequiredService<BootstrapInitializer>().EnsureBootstrapCredentialsAsync();
    await scope.ServiceProvider.GetRequiredService<VaultService>().EnsureServiceSyncWrapAsync();
    await scope.ServiceProvider.GetRequiredService<BackgroundJobService>().EnsureJobsAsync();
}

app.MapFallbackToFile("index.html");

app.Run();

public partial class Program;
