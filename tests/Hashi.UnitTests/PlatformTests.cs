using Hashi.Core.Resources;
using Hashi.Core.Traefik;
using Xunit;

namespace Hashi.UnitTests;

public sealed class TraefikConfigRendererTests
{
    [Fact]
    public void Render_produces_stable_hash_for_same_input()
    {
        IReadOnlyList<ResourceDefinition> resources =
        [
            new ResourceDefinition(Guid.NewGuid(), "Hashi", "hashi", ResourceKind.Https, true, true, "hashi.example.com", "http", "127.0.0.1", 8080),
        ];
        var first = TraefikConfigRenderer.Render(resources);
        var second = TraefikConfigRenderer.Render(resources);
        Assert.Equal(first.ContentHash, second.ContentHash);
        Assert.Contains("hashi.example.com", second.DynamicHttpYaml);
    }
}

public sealed class ResourceSlugTests
{
    [Fact]
    public void Normalize_replaces_invalid_characters()
    {
        Assert.Equal("my-app", ResourceSlug.Normalize("My App!"));
    }
}
