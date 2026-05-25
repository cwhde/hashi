using Hashi.Api.Features.Auth;
using Hashi.Api.Features.Setup;
using Hashi.Api.Features.Vault;
using Hashi.Infrastructure;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Bootstrap;
using Hashi.Infrastructure.Persistence;
using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.OpenApi;
using Microsoft.EntityFrameworkCore;
using Microsoft.OpenApi.Models;
using Serilog;

var builder = WebApplication.CreateBuilder(args);

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

var app = builder.Build();

if (app.Environment.IsDevelopment()
    || app.Environment.EnvironmentName == "OpenApiExport"
    || string.Equals(Environment.GetEnvironmentVariable("HASHI_EXPORT_OPENAPI"), "1", StringComparison.Ordinal))
{
    app.MapOpenApi();
}

app.UseSerilogRequestLogging();
app.UseAuthentication();
app.UseAuthorization();
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

var skipStartupHooks = builder.Configuration.GetValue<bool>("Hashi:SkipStartupHooks")
    || string.Equals(Environment.GetEnvironmentVariable("HASHI_SKIP_STARTUP_HOOKS"), "1", StringComparison.Ordinal);
if (!skipStartupHooks)
{
    using var scope = app.Services.CreateScope();
    var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
    await db.Database.MigrateAsync();
    await scope.ServiceProvider.GetRequiredService<BootstrapInitializer>().EnsureBootstrapCredentialsAsync();
    await scope.ServiceProvider.GetRequiredService<VaultService>().EnsureServiceSyncWrapAsync();
}

app.MapFallbackToFile("index.html");

app.Run();

public partial class Program;
