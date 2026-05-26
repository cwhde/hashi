using Testcontainers.PostgreSql;

namespace Hashi.IntegrationTests;

/// <summary>
/// Lazy PostgreSQL test container — avoids Testcontainers constructor failures when Docker is unavailable.
/// </summary>
internal sealed class IntegrationTestPostgres : IAsyncDisposable
{
    private PostgreSqlContainer? _container;

    public bool IsAvailable { get; private set; }

    public string ConnectionString =>
        _container?.GetConnectionString()
        ?? throw new InvalidOperationException("PostgreSQL test container is not running.");

    public async Task StartAsync(CancellationToken cancellationToken = default)
    {
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
