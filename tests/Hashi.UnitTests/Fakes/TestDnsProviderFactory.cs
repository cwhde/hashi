using Hashi.Core.Dns;
using Hashi.Infrastructure.Providers.Dns;

namespace Hashi.UnitTests.Fakes;

public sealed class TestDnsProviderFactory : IDnsProviderFactory
{
    public InMemoryDnsProvider Provider { get; } = new();

    public IDnsProvider Create(string providerType, string apiToken) => Provider;
}
