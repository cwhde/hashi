using Hashi.Core.Hosting;
using Hashi.Infrastructure.Persistence.Entities;

namespace Hashi.Infrastructure.Platform;

public sealed class HashiInternalUrlResolver(HashiPortOptions ports)
{
    public string ResolveBaseUrl(AppSettingsEntity settings)
    {
        var configured = settings.InternalUrl?.Trim();
        return string.IsNullOrWhiteSpace(configured)
            ? $"http://127.0.0.1:{ports.Admin}"
            : configured.TrimEnd('/');
    }

    public string ResolveUrl(AppSettingsEntity settings, string path)
    {
        var baseUrl = ResolveBaseUrl(settings);
        return $"{baseUrl}/{path.TrimStart('/')}";
    }
}
