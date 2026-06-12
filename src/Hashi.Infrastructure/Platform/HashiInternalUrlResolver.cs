using Hashi.Core.Hosting;
using Hashi.Infrastructure.Persistence.Entities;

namespace Hashi.Infrastructure.Platform;

public sealed class HashiInternalUrlResolver(HashiPortOptions ports)
{
    public string ResolveBaseUrl(AppSettingsEntity settings)
    {
        var configured = settings.InternalUrl?.Trim();
        if (!string.IsNullOrWhiteSpace(configured))
        {
            return configured.TrimEnd('/');
        }

        var scheme = settings.InternalScheme?.Trim().ToLowerInvariant() ?? "http";
        return $"{scheme}://127.0.0.1:{ports.Admin}";
    }

    public string ResolveUrl(AppSettingsEntity settings, string path)
    {
        var baseUrl = ResolveBaseUrl(settings);
        return $"{baseUrl}/{path.TrimStart('/')}";
    }
}
