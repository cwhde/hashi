using Hashi.Contracts.Api;

namespace Hashi.Infrastructure.Platform;

public static class PulseInstallRenderer
{
    public static PulseInstallResponse Render(string apiBaseUrl, Guid agentId, string? token = null)
    {
        var api = apiBaseUrl.TrimEnd('/');
        var tokenPlaceholder = string.IsNullOrWhiteSpace(token) ? "<PULSE_TOKEN>" : token;
        var linux = $$"""
            export HASHI_PULSE_API='{{api}}'
            export HASHI_PULSE_AGENT_ID='{{agentId}}'
            export HASHI_PULSE_TOKEN='{{tokenPlaceholder}}'
            curl -fsSL '{{api}}/api/pulse/install/linux.sh' | sudo bash
            """;
        var docker = $$"""
            docker run -d --name hashi-pulse --restart unless-stopped \
              -e HASHI_PULSE_API='{{api}}' \
              -e HASHI_PULSE_AGENT_ID='{{agentId}}' \
              -e HASHI_PULSE_TOKEN='{{tokenPlaceholder}}' \
              git.juzo.io/juzo/hashi-pulse:latest
            """;
        return new PulseInstallResponse(linux.Trim(), docker.Trim());
    }
}
