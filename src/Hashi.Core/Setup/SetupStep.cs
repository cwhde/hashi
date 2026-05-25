namespace Hashi.Core.Setup;

public enum SetupStep
{
    BootstrapAccess = 0,
    BaseSettings = 1,
    DnsProvider = 2,
    CertificateProvider = 3,
    TraefikConnection = 4,
    FirewallHost = 5,
    SystemResource = 6,
    PasskeyAndVault = 7,
    Optional = 8,
    Complete = 9,
}

public static class SetupStepNames
{
    public static string ToSlug(SetupStep step) => step switch
    {
        SetupStep.BootstrapAccess => "bootstrap-access",
        SetupStep.BaseSettings => "base-settings",
        SetupStep.DnsProvider => "dns-provider",
        SetupStep.CertificateProvider => "certificate-provider",
        SetupStep.TraefikConnection => "traefik-connection",
        SetupStep.FirewallHost => "firewall-host",
        SetupStep.SystemResource => "system-resource",
        SetupStep.PasskeyAndVault => "passkey-and-vault",
        SetupStep.Optional => "optional",
        SetupStep.Complete => "complete",
        _ => "unknown",
    };

    public static SetupStep FromSlug(string slug) => slug switch
    {
        "bootstrap-access" => SetupStep.BootstrapAccess,
        "base-settings" => SetupStep.BaseSettings,
        "dns-provider" => SetupStep.DnsProvider,
        "certificate-provider" => SetupStep.CertificateProvider,
        "traefik-connection" => SetupStep.TraefikConnection,
        "firewall-host" => SetupStep.FirewallHost,
        "system-resource" => SetupStep.SystemResource,
        "passkey-and-vault" => SetupStep.PasskeyAndVault,
        "optional" => SetupStep.Optional,
        "complete" => SetupStep.Complete,
        _ => SetupStep.BootstrapAccess,
    };
}
