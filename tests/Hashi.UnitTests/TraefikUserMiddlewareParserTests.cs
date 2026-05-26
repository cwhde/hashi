using Hashi.Core.Traefik;
using Xunit;

namespace Hashi.UnitTests;

public sealed class TraefikUserMiddlewareParserTests
{
    [Fact]
    public void Parse_accepts_empty_middleware_block()
    {
        var result = TraefikUserMiddlewareParser.Parse(TraefikUserMiddlewareParser.DefaultYaml);
        Assert.True(result.IsValid);
        Assert.Empty(result.MiddlewareNames);
    }

    [Fact]
    public void Parse_extracts_middleware_names()
    {
        var yaml = """
            http:
              middlewares:
                custom-headers:
                  headers:
                    customResponseHeaders:
                      X-Test: "1"
                ip-whitelist:
                  ipWhiteList:
                    sourceRange:
                      - 10.0.0.0/8
            """;

        var result = TraefikUserMiddlewareParser.Parse(yaml);

        Assert.True(result.IsValid);
        Assert.Equal(["custom-headers", "ip-whitelist"], result.MiddlewareNames);
    }

    [Fact]
    public void Parse_rejects_missing_http_section()
    {
        var result = TraefikUserMiddlewareParser.Parse("middlewares: {}");
        Assert.False(result.IsValid);
        Assert.Contains("http:", result.Error, StringComparison.Ordinal);
    }

    [Fact]
    public void Parse_rejects_duplicate_names()
    {
        var yaml = """
            http:
              middlewares:
                dup:
                  compress: {}
                dup:
                  compress: {}
            """;

        var result = TraefikUserMiddlewareParser.Parse(yaml);
        Assert.False(result.IsValid);
        Assert.Contains("Duplicate", result.Error, StringComparison.Ordinal);
    }
}

public sealed class TraefikConfigValidatorTests
{
    [Fact]
    public void ValidateRender_passes_for_default_render()
    {
        var render = TraefikConfigRenderer.Render([]);
        var result = TraefikConfigValidator.ValidateRender(render);
        Assert.True(result.IsValid, string.Join("; ", result.Errors));
    }
}
