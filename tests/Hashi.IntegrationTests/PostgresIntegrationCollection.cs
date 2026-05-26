using Xunit;

namespace Hashi.IntegrationTests;

[CollectionDefinition(Name)]
public sealed class PostgresIntegrationCollection : ICollectionFixture<PostgresIntegrationFixture>
{
    public const string Name = "postgres-integration";
}

/// <summary>
/// One PostgreSQL container for the integration test assembly; each test class gets an isolated database.
/// </summary>
public sealed class PostgresIntegrationFixture : IAsyncLifetime
{
    private readonly IntegrationTestPostgres _postgres = new();
    private readonly SemaphoreSlim _databaseGate = new(1, 1);

    public bool IsAvailable => _postgres.IsAvailable;

    public async Task InitializeAsync()
    {
        await _postgres.StartAsync();
    }

    public async Task<string> CreateDatabaseAsync(CancellationToken cancellationToken = default)
    {
        if (!_postgres.IsAvailable)
        {
            throw new InvalidOperationException("PostgreSQL test database is not running.");
        }

        await _databaseGate.WaitAsync(cancellationToken);
        try
        {
            return await _postgres.CreateIsolatedDatabaseAsync(cancellationToken);
        }
        finally
        {
            _databaseGate.Release();
        }
    }

    public async Task DisposeAsync()
    {
        _databaseGate.Dispose();
        await _postgres.DisposeAsync();
    }
}
