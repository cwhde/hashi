using System.Globalization;
using System.Text;

namespace Hashi.Core.Dns;

public static class InternalAgentDnsName
{
    public static string NormalizeLabel(string? value)
    {
        if (string.IsNullOrWhiteSpace(value))
        {
            throw new InvalidOperationException("Internal agent DNS name cannot be empty.");
        }

        var decomposed = value.Trim().Normalize(NormalizationForm.FormD);
        var builder = new StringBuilder(decomposed.Length);
        var previousHyphen = false;

        foreach (var ch in decomposed)
        {
            if (CharUnicodeInfo.GetUnicodeCategory(ch) == UnicodeCategory.NonSpacingMark)
            {
                continue;
            }

            var lower = char.ToLowerInvariant(ch);
            var valid = lower is >= 'a' and <= 'z' || lower is >= '0' and <= '9';
            if (valid)
            {
                builder.Append(lower);
                previousHyphen = false;
                continue;
            }

            if (!previousHyphen)
            {
                builder.Append('-');
                previousHyphen = true;
            }
        }

        var normalized = builder.ToString().Trim('-');
        if (normalized.Length == 0)
        {
            throw new InvalidOperationException("Internal agent DNS name must contain at least one ASCII letter or digit.");
        }

        return normalized.Length <= 63
            ? normalized
            : normalized[..63].TrimEnd('-');
    }

    public static string NormalizeDomain(string? value)
    {
        var domain = string.IsNullOrWhiteSpace(value)
            ? "hashi.home.arpa"
            : value.Trim().TrimEnd('.').ToLowerInvariant();
        if (string.IsNullOrWhiteSpace(domain) || domain.Contains("..", StringComparison.Ordinal))
        {
            throw new InvalidOperationException("Internal agent DNS domain is invalid.");
        }

        var labels = domain.Split('.', StringSplitOptions.RemoveEmptyEntries);
        if (labels.Length < 2)
        {
            throw new InvalidOperationException("Internal agent DNS domain must contain at least two labels.");
        }

        foreach (var label in labels)
        {
            if (NormalizeLabel(label) != label)
            {
                throw new InvalidOperationException("Internal agent DNS domain must use lowercase ASCII DNS labels.");
            }
        }

        return domain;
    }

    public static string BuildFqdn(string label, string domain)
        => $"{NormalizeLabel(label)}.{NormalizeDomain(domain)}";
}
