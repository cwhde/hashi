using Hashi.Core.Auth;
using Hashi.Infrastructure.Persistence.Entities;

namespace Hashi.Infrastructure.Auth;

public static class RuntimeSecretEligibility
{
    public static bool IsRuntimeSshConnectionType(string connectionType)
        => connectionType is ConnectionTypeNames.TraefikHost or ConnectionTypeNames.FirewallHost;

    public static bool IsRuntimePurpose(SecretPurpose purpose)
        => purpose is SecretPurpose.NotificationToken or SecretPurpose.OidcClientSecret or SecretPurpose.MaxMindLicenseKey;
}
