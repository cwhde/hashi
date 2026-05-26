using Hashi.Core.Connections;
using Hashi.Infrastructure.Ssh;
using Xunit;

namespace Hashi.UnitTests;

public sealed class OsReleaseParserTests
{
    [Theory]
    [InlineData("ID=debian", OsFamily.Debian, "apt")]
    [InlineData("ID=ubuntu", OsFamily.Ubuntu, "apt")]
    [InlineData("ID=alpine", OsFamily.Alpine, "apk")]
    public void Parses_os_release_id(string line, OsFamily expectedOs, string expectedPackageManager)
    {
        var (os, packageManager) = OsReleaseParser.Parse(line);
        Assert.Equal(expectedOs, os);
        Assert.Equal(expectedPackageManager, packageManager);
    }
}
