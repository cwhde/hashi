using Hashi.Core.Security;
using Xunit;

namespace Hashi.UnitTests;

public sealed class WafMiddlewareRendererTests
{
    [Fact]
    public void Render_includes_coraza_directives()
    {
        var yaml = WafMiddlewareRenderer.RenderCorazaMiddleware("app", WafMode.On);
        Assert.Contains("coraza", yaml);
        Assert.Contains("app-waf", yaml);
        Assert.Contains("SecRuleEngine On", yaml);
    }
}
