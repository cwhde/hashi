using Hashi.Contracts.Api;

namespace Hashi.Infrastructure.Platform;

public static class PulseInstallRenderer
{
    public static PulseInstallResponse Render(string apiBaseUrl, Guid agentId)
    {
        var api = apiBaseUrl.TrimEnd('/');
        var linux = $$"""
            curl -fsSL '{{api}}/api/pulse/install/linux.sh' | sudo env \
              HASHI_PULSE_API='{{api}}' \
              HASHI_PULSE_AGENT_ID='{{agentId}}' \
              HASHI_PULSE_TOKEN='<PULSE_TOKEN>' \
              bash
            """;
        var docker = $$"""
            services:
              hashi-pulse:
                image: git.juzo.io/juzo/hashi-pulse:latest
                restart: unless-stopped
                environment:
                  HASHI_PULSE_API: '{{api}}'
                  HASHI_PULSE_AGENT_ID: '{{agentId}}'
                  HASHI_PULSE_TOKEN: '<PULSE_TOKEN>'
                  HASHI_PULSE_DOCKER_IMAGE: git.juzo.io/juzo/hashi-pulse:latest
                  HASHI_PULSE_DOCKER_NETWORK_MODE: bridge
            """;
        return new PulseInstallResponse(linux.Trim(), docker.Trim());
    }
}
