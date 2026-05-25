using Hashi.Api.Features.Setup;
using Hashi.Infrastructure;
using Hashi.Infrastructure.Bootstrap;
using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;
using Serilog;

var builder = WebApplication.CreateBuilder(args);

builder.Host.UseSerilog((context, services, configuration) =>
    configuration.ReadFrom.Configuration(context.Configuration)
        .ReadFrom.Services(services)
        .Enrich.FromLogContext()
        .WriteTo.Console());

builder.Services.AddOpenApi();
builder.Services.AddHashiInfrastructure(builder.Configuration);
builder.Services.AddScoped<BootstrapInitializer>();

var app = builder.Build();

if (app.Environment.IsDevelopment())
{
    app.MapOpenApi();
}

app.UseSerilogRequestLogging();
app.UseDefaultFiles();
app.UseStaticFiles();

app.MapHealthEndpoints();
app.MapSetupEndpoints();
app.MapSetupAdvanceEndpoints();
app.MapSettingsEndpoints();
app.MapActivityEndpoints();

using (var scope = app.Services.CreateScope())
{
    var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
    await db.Database.MigrateAsync();
    await scope.ServiceProvider.GetRequiredService<BootstrapInitializer>().EnsureBootstrapCredentialsAsync();
}

app.MapFallbackToFile("index.html");

app.Run();

public partial class Program;
