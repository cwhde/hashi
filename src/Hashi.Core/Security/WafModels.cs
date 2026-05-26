namespace Hashi.Core.Security;

public enum WafMode
{
    Off,
    DetectOnly,
    On,
}

public static class WafMiddlewareRenderer
{
    public static string RenderCorazaMiddleware(string slug, WafMode mode)
    {
        var modeDirective = mode switch
        {
            WafMode.Off => "SecRuleEngine Off",
            WafMode.DetectOnly => "SecRuleEngine DetectionOnly",
            _ => "SecRuleEngine On",
        };

        return $$"""
            http:
              middlewares:
                {{slug}}-waf:
                  plugin:
                    coraza:
                      directives:
                        - "{{modeDirective}}"
                        - "Include @owasp_crs/REQUEST-900-EXCLUSION-RULES-BEFORE-CRS.conf"
                        - "Include @owasp_crs/REQUEST-941-APPLICATION-ATTACK-XSS.conf"
                        - "Include @owasp_crs/RESPONSE-959-BLOCKING-EVALUATION.conf"
            """;
    }
}
