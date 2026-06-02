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

    [Fact]
    public void Render_multiple_middlewares_uses_one_http_map_and_includes_exclusions()
    {
        var yaml = WafMiddlewareRenderer.RenderCorazaMiddlewares(
            [
                new WafMiddlewareDefinition("app", WafMode.On, ["SecRuleRemoveById 941100"]),
                new WafMiddlewareDefinition("admin", WafMode.DetectOnly, ["SecRuleUpdateTargetById 942100 !ARGS:search"]),
            ]);

        Assert.Equal(1, yaml.Split('\n').Count(line => line == "http:"));
        Assert.Contains("app-waf:", yaml);
        Assert.Contains("admin-waf:", yaml);
        Assert.Contains("SecRuleRemoveById 941100", yaml);
        Assert.Contains("SecRuleUpdateTargetById 942100 !ARGS:search", yaml);
    }
}
