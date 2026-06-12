using System.Net;
using Hashi.Contracts.Api;

namespace Hashi.Infrastructure.Platform;

public sealed class EdgeAuthService
{
    private readonly SecurityDecisionService decisions;
    private readonly GeoIpLookupService geoIp;

    public EdgeAuthService(SecurityDecisionService decisions, GeoIpLookupService geoIp)
    {
        this.decisions = decisions;
        this.geoIp = geoIp;
    }

    public async Task<EdgeAuthForwardResponse> EvaluateForwardAsync(
        string host,
        string path,
        IPAddress clientIp,
        string? countryCode,
        string? regionCode,
        string? asn,
        string? edgeSessionKey = null,
        string? mode = null,
        bool trustedForwardedContext = true,
        string method = "GET",
        string? acceptHeader = null,
        CancellationToken cancellationToken = default)
    {
        var decision = await EvaluateForwardDecisionAsync(
            new SecurityDecisionRequest(
                host,
                path,
                clientIp,
                countryCode,
                regionCode,
                asn,
                edgeSessionKey,
                mode,
                trustedForwardedContext,
                method,
                acceptHeader),
            cancellationToken);
        return new EdgeAuthForwardResponse(decision.Decision, decision.RedirectUrl);
    }

    public Task<SecurityDecisionResult> EvaluateForwardDecisionAsync(
        SecurityDecisionRequest request,
        CancellationToken cancellationToken = default)
        => decisions.DecideForwardAuthAsync(request, cancellationToken);

    public IReadOnlyList<string> ValidateRuleMatchJson(string matchJson)
        => geoIp.ValidateGeoMatchRules(matchJson);
}
