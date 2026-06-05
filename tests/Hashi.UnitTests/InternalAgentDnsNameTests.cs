using Hashi.Core.Dns;
using Xunit;

namespace Hashi.UnitTests;

public sealed class InternalAgentDnsNameTests
{
    [Theory]
    [InlineData("Kanae Node", "kanae-node")]
    [InlineData("  KANAE__Node!!  ", "kanae-node")]
    [InlineData("München Edge", "munchen-edge")]
    [InlineData("---edge---01---", "edge-01")]
    public void NormalizeLabel_creates_ascii_dns_slug(string input, string expected)
    {
        Assert.Equal(expected, InternalAgentDnsName.NormalizeLabel(input));
    }

    [Fact]
    public void NormalizeLabel_rejects_empty_slug()
    {
        var ex = Assert.Throws<InvalidOperationException>(() => InternalAgentDnsName.NormalizeLabel("!!!"));

        Assert.Contains("ASCII", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void NormalizeDomain_requires_lowercase_ascii_labels()
    {
        Assert.Equal("hashi.home.arpa", InternalAgentDnsName.NormalizeDomain(null));
        Assert.Equal("hashi.home.arpa", InternalAgentDnsName.NormalizeDomain("Hashi.Home.Arpa"));
        Assert.Throws<InvalidOperationException>(() => InternalAgentDnsName.NormalizeDomain("hashi_home.arpa"));
        Assert.Throws<InvalidOperationException>(() => InternalAgentDnsName.NormalizeDomain("home"));
    }
}
