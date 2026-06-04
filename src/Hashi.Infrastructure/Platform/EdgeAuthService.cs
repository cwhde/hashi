using System.Net;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;

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

    public EdgeAuthService(HashiDbContext db, GeoIpLookupService geoIp, OidcEdgeAuthService oidc)
        : this(new SecurityDecisionService(db, oidc), geoIp)
    {
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
                mode),
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
