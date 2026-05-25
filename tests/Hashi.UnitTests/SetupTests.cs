using Hashi.Core.Setup;
using Hashi.Infrastructure.Bootstrap;

using Xunit;

namespace Hashi.UnitTests;

public sealed class SetupStepTests
{
    [Theory]
    [InlineData(SetupStep.DnsProvider, "dns-provider")]
    [InlineData(SetupStep.PasskeyAndVault, "passkey-and-vault")]
    public void RoundTrips_step_slugs(SetupStep step, string slug)
    {
        Assert.Equal(slug, SetupStepNames.ToSlug(step));
        Assert.Equal(step, SetupStepNames.FromSlug(slug));
    }
}

public sealed class BootstrapNetworkPolicyTests
{
    [Theory]
    [InlineData("10.0.0.5", true)]
    [InlineData("192.168.1.20", true)]
    [InlineData("127.0.0.1", true)]
    [InlineData("8.8.8.8", false)]
    public void Allows_private_ranges_only(string ip, bool expected)
    {
        Assert.Equal(expected, BootstrapNetworkPolicy.IsAllowed(ip));
    }
}
