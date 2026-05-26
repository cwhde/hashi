using Npgsql;
using Hashi.Infrastructure.Persistence;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Testcontainers.PostgreSql;

namespace Hashi.IntegrationTests;

internal static class IntegrationTestApp
{
    public static WebApplicationFactory<Program> CreateFactory(string connectionString)
        => new WebApplicationFactory<Program>()
            .WithWebHostBuilder(builder =>
            {
                builder.UseSetting("ConnectionStrings:Hashi", connectionString);
                builder.UseSetting("Hashi:SkipStartupHooks", "true");
            });

    public static async Task MigrateAsync(IServiceProvider services, CancellationToken cancellationToken = default)
    {
        using var scope = services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        await db.Database.MigrateAsync(cancellationToken);
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

    public async Task StartAsync(CancellationToken cancellationToken = default)
    {
        var ciConnection = Environment.GetEnvironmentVariable("ConnectionStrings__Hashi");
        if (!string.IsNullOrWhiteSpace(ciConnection))
        {
            if (await WaitForConnectionAsync(ciConnection, cancellationToken))
            {
                _connectionString = ciConnection;
                IsAvailable = true;
            }

            return;
        }

        if (!File.Exists("/var/run/docker.sock"))
        {
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
            Console.WriteLine($"Skipping PostgreSQL integration tests: {ex.Message}");
            IsAvailable = false;
            if (_container is not null)
            {
                await _container.DisposeAsync();
                _container = null;
            }
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
}
