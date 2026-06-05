using Hashi.Api.Features.Auth;
using Hashi.Api.Features.Connections;
using Hashi.Api.Features.Dns;
using Hashi.Api.Features.Resources;
using Hashi.Api.Features.Setup;
using Hashi.Api.Features.Sync;
using Hashi.Api.Features.Vault;
using Hashi.Api.Hosting;
using Hashi.Core.Hosting;
using Hashi.Infrastructure;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Bootstrap;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Persistence;
using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.OpenApi;
using Microsoft.EntityFrameworkCore;
using Microsoft.OpenApi;
using Serilog;

var builder = WebApplication.CreateBuilder(args);
var portOptions = HashiPortOptions.FromConfiguration(builder.Configuration);

if (!builder.Environment.IsEnvironment("OpenApiExport"))
{
    builder.WebHost.ConfigureKestrel(options =>
    {
        options.ListenAnyIP(portOptions.Admin);
        options.ListenAnyIP(portOptions.PublicDashboard);
        options.ListenAnyIP(portOptions.PublicStatus);
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
builder.Services.AddSingleton(portOptions);
builder.Services.AddHashiInfrastructure(builder.Configuration);
builder.Services.AddSingleton<ForwardedClientContextResolver>();
builder.Services.AddScoped<BootstrapInitializer>();

builder.Services.AddAuthentication(CookieAuthenticationDefaults.AuthenticationScheme)
    .AddCookie(options =>
    {
        options.Cookie.Name = "hashi.session";
        options.Cookie.HttpOnly = true;
        options.Cookie.SecurePolicy = CookieSecurePolicy.Always;
        options.SlidingExpiration = true;
        options.ExpireTimeSpan = TimeSpan.FromHours(8);
    });
builder.Services.AddAuthorization();
builder.Services.AddCors(options =>
{
    options.AddPolicy("PublicRead", policy =>
        policy.SetIsOriginAllowed(origin =>
        {
            if (!Uri.TryCreate(origin, UriKind.Absolute, out var uri))
            {
                return false;
            }

            return portOptions.IsPublicPort(uri.Port);
        })
        .AllowAnyHeader()
        .AllowAnyMethod());
});
builder.Services.AddAntiforgery(options =>
{
    options.HeaderName = "X-CSRF-TOKEN";
    options.Cookie.Name = "hashi.csrf";
    options.Cookie.HttpOnly = true;
    options.Cookie.SecurePolicy = CookieSecurePolicy.Always;
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
app.MapGet("/hashi-runtime-config.js", (HashiPortOptions ports) =>
    Results.Text(
        $$"""
        window.__HASHI_RUNTIME_CONFIG__ = {
          ports: {
            admin: {{ports.Admin}},
            publicDashboard: {{ports.PublicDashboard}},
            publicStatus: {{ports.PublicStatus}}
          }
        };
        """,
        "application/javascript; charset=utf-8"));
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
app.MapPublicEndpoints();
app.MapEdgeAuthEndpoints();
app.MapEdgeChallengeEndpoints();
app.MapEdgeSsoAdminEndpoints();
app.MapSecurityEndpoints();
app.MapPulseEndpoints();
app.MapInternalAgentDnsEndpoints();
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
