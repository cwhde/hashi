using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Npgsql;
using Testcontainers.PostgreSql;

namespace Hashi.IntegrationTests;

internal static class IntegrationTestApp
{
    public static WebApplicationFactoryClientOptions HttpsClientOptions(
        bool handleCookies = true,
        bool allowAutoRedirect = true)
        => new()
        {
            BaseAddress = new Uri("https://localhost"),
            HandleCookies = handleCookies,
            AllowAutoRedirect = allowAutoRedirect,
        };

    public static WebApplicationFactory<Program> CreateFactory(
        string connectionString,
        Action<IServiceCollection>? configureServices = null)
        => new WebApplicationFactory<Program>()
            .WithWebHostBuilder(builder =>
            {
                builder.UseSetting("ConnectionStrings:Hashi", connectionString);
                builder.UseSetting("Hashi:SkipStartupHooks", "true");
                builder.ConfigureServices(services =>
                {
                    services.AddSingleton<IStartupFilter, LoopbackRemoteIpStartupFilter>();
                    configureServices?.Invoke(services);
                });
            });

    public static async Task MigrateAsync(IServiceProvider services, CancellationToken cancellationToken = default)
    {
        using var scope = services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        await db.Database.MigrateAsync(cancellationToken);
    }

    public static async Task EnsureSeededAsync(IServiceProvider services, CancellationToken cancellationToken = default)
    {
        using var scope = services.CreateScope();
        var setupState = scope.ServiceProvider.GetRequiredService<SetupStateService>();
        var appSettings = scope.ServiceProvider.GetRequiredService<AppSettingsService>();
        await setupState.GetOrCreateAsync(cancellationToken);
        await appSettings.GetOrCreateAsync(cancellationToken);
    }
}

/// <summary>
/// PostgreSQL for integration tests. Uses CI service connection string when provided;
/// otherwise starts a Testcontainers instance when Docker is available.
/// </summary>
internal sealed class IntegrationTestPostgres : IAsyncDisposable
{
    private PostgreSqlContainer? _container;
    private string? _connectionString;

    public bool IsAvailable { get; private set; }

    public string ConnectionString =>
        _connectionString
        ?? throw new InvalidOperationException("PostgreSQL test database is not running.");

    public async Task<string> CreateIsolatedDatabaseAsync(CancellationToken cancellationToken = default)
    {
        if (_connectionString is null)
        {
            throw new InvalidOperationException("PostgreSQL test database is not running.");
        }

        var databaseName = $"hashi_{Guid.NewGuid():N}"[..20];
        await using (var admin = new NpgsqlConnection(_connectionString))
        {
            await admin.OpenAsync(cancellationToken);
            await using var create = new NpgsqlCommand(
                $"CREATE DATABASE \"{databaseName}\"",
                admin);
            await create.ExecuteNonQueryAsync(cancellationToken);
        }

        var builder = new NpgsqlConnectionStringBuilder(_connectionString)
        {
            Database = databaseName,
        };
        var isolatedConnection = builder.ConnectionString;

        await using var factory = IntegrationTestApp.CreateFactory(isolatedConnection);
        await IntegrationTestApp.MigrateAsync(factory.Services, cancellationToken);
        await IntegrationTestApp.EnsureSeededAsync(factory.Services, cancellationToken);

        return isolatedConnection;
    }

    public async Task StartAsync(CancellationToken cancellationToken = default)
    {
        var ciConnection = Environment.GetEnvironmentVariable("ConnectionStrings__Hashi");
        if (!string.IsNullOrWhiteSpace(ciConnection)
            && await WaitForConnectionAsync(ciConnection, cancellationToken))
        {
            _connectionString = ciConnection;
            IsAvailable = true;
            return;
        }

        // Gitea act_runner often does not wire GitHub-style service containers to localhost.
        // Fall through to Testcontainers when Docker is available on the runner.

        if (!File.Exists("/var/run/docker.sock"))
        {
            FailIfCi("PostgreSQL integration tests require ConnectionStrings__Hashi or Docker in CI.");
            return;
        }

        try
        {
            _container = new PostgreSqlBuilder()
                .WithImage("postgres:18")
                .WithDatabase("hashi")
                .WithUsername("hashi")
                .WithPassword("hashi")
                .Build();
            await _container.StartAsync(cancellationToken);
            _connectionString = _container.GetConnectionString();
            IsAvailable = true;
        }
        catch (Exception ex)
        {
            var message = $"PostgreSQL integration tests unavailable: {ex.Message}";
            Console.WriteLine(message);
            IsAvailable = false;
            if (_container is not null)
            {
                await _container.DisposeAsync();
                _container = null;
            }

            FailIfCi(message);
        }
    }

    public async ValueTask DisposeAsync()
    {
        if (_container is not null)
        {
            await _container.DisposeAsync();
            _container = null;
        }
    }

    private static async Task<bool> WaitForConnectionAsync(string connectionString, CancellationToken cancellationToken)
    {
        for (var attempt = 1; attempt <= 60; attempt++)
        {
            try
            {
                await using var connection = new NpgsqlConnection(connectionString);
                await connection.OpenAsync(cancellationToken);
                return true;
            }
            catch (Exception ex) when (attempt < 60 && IsTransientConnectionFailure(ex))
            {
                Console.WriteLine($"PostgreSQL not ready (attempt {attempt}/60): {ex.Message}");
                await Task.Delay(TimeSpan.FromSeconds(1), cancellationToken);
            }
        }

        Console.WriteLine("PostgreSQL did not become ready within 60 seconds.");
        return false;
    }

    private static bool IsTransientConnectionFailure(Exception ex)
        => ex is NpgsqlException or InvalidOperationException
           || ex.InnerException is System.Net.Sockets.SocketException;

    private static void FailIfCi(string message)
    {
        if (string.Equals(Environment.GetEnvironmentVariable("CI"), "true", StringComparison.OrdinalIgnoreCase))
        {
            throw new InvalidOperationException(message);
        }
    }
}
