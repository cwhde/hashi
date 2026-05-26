namespace Hashi.Core.Resources;

public enum ForwardAuthPolicy
{
    Off,
    SsoRequired,
    Adaptive,
    Observe,
}

public static class ForwardAuthPolicyMapping
{
    public static ForwardAuthPolicy Parse(string? value) => value?.ToLowerInvariant() switch
    {
        "off" => ForwardAuthPolicy.Off,
        "sso_required" or "ssorequired" => ForwardAuthPolicy.SsoRequired,
        "observe" => ForwardAuthPolicy.Observe,
        _ => ForwardAuthPolicy.Adaptive,
    };

    public static string ToName(ForwardAuthPolicy policy) => policy switch
    {
        ForwardAuthPolicy.Off => "off",
        ForwardAuthPolicy.SsoRequired => "sso_required",
        ForwardAuthPolicy.Observe => "observe",
        _ => "adaptive",
    };
}
