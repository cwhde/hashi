using Hashi.Contracts.Api;

namespace Hashi.Infrastructure.Platform;

public static class PulseInstallRenderer
{
    public static PulseInstallResponse Render(string apiBaseUrl, Guid agentId)
    {
        var api = apiBaseUrl.TrimEnd('/');
        var linux = $$"""
            export HASHI_PULSE_API='{{api}}'
            export HASHI_PULSE_AGENT_ID='{{agentId}}'
            export HASHI_PULSE_TOKEN='<PULSE_TOKEN>'
            curl -fsSL '{{api}}/api/pulse/install/linux.sh' | sudo bash
            """;
        var docker = $$"""
            docker run -d --name hashi-pulse --restart unless-stopped \
              -e HASHI_PULSE_API='{{api}}' \
              -e HASHI_PULSE_AGENT_ID='{{agentId}}' \
              -e HASHI_PULSE_TOKEN='<PULSE_TOKEN>' \
              -e HASHI_PULSE_DOCKER_IMAGE='git.juzo.io/juzo/hashi-pulse:latest' \
              -e HASHI_PULSE_DOCKER_NETWORK_MODE='bridge' \
              git.juzo.io/juzo/hashi-pulse:latest
            """;
        return new PulseInstallResponse(linux.Trim(), docker.Trim());
    }
}
