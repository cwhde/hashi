using System.Diagnostics.CodeAnalysis;
using YamlDotNet.RepresentationModel;

namespace Hashi.Core.Traefik;

internal static class YamlNavigationHelpers
{
    public static bool TryGetMapping(YamlMappingNode node, string key, [NotNullWhen(true)] out YamlMappingNode? mapping)
    {
        if (TryGetNode(node, key, out var value) && value is YamlMappingNode child)
        {
            mapping = child;
            return true;
        }

        mapping = null!;
        return false;
    }

    public static bool TryGetScalar(YamlMappingNode node, string key, [NotNullWhen(true)] out string? value)
    {
        if (TryGetNode(node, key, out var yamlNode) && yamlNode is YamlScalarNode scalar && scalar.Value is not null)
        {
            value = scalar.Value;
            return true;
        }

        value = null;
        return false;
    }

    public static bool TryGetNode(YamlMappingNode node, string key, [NotNullWhen(true)] out YamlNode? value)
    {
        foreach (var (candidateKey, candidateValue) in node.Children)
        {
            if (candidateKey is YamlScalarNode scalar && string.Equals(scalar.Value, key, StringComparison.Ordinal))
            {
                value = candidateValue;
                return true;
            }
        }

        value = null!;
        return false;
    }
}
