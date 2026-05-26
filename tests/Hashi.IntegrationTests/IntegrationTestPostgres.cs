using Testcontainers.PostgreSql;

namespace Hashi.IntegrationTests;

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
            _connectionString = ciConnection;
            IsAvailable = true;
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
}
