namespace Hashi.Api.Hosting;

public static class HashiPorts
{
    public const int DefaultAdmin = 8080;
    public const int DefaultPublicDashboard = 8081;
    public const int DefaultPublicStatus = 8082;
}

public sealed class HashiPortOptions
{
    public const string SectionName = "Hashi:Ports";

    public int Admin { get; set; } = HashiPorts.DefaultAdmin;

    public int PublicDashboard { get; set; } = HashiPorts.DefaultPublicDashboard;

    public int PublicStatus { get; set; } = HashiPorts.DefaultPublicStatus;

    public static HashiPortOptions FromConfiguration(IConfiguration configuration)
    {
        var options = new HashiPortOptions();
        configuration.GetSection(SectionName).Bind(options);
        options.Validate();
        return options;
    }

    public void Validate()
    {
        ValidatePort(Admin, nameof(Admin));
        ValidatePort(PublicDashboard, nameof(PublicDashboard));
        ValidatePort(PublicStatus, nameof(PublicStatus));

        if (Admin == PublicDashboard || Admin == PublicStatus || PublicDashboard == PublicStatus)
        {
            throw new InvalidOperationException("Hashi ports must be distinct.");
        }
    }

    public bool IsPublicPort(int port) => port == PublicDashboard || port == PublicStatus;

    private static void ValidatePort(int port, string name)
    {
        if (port is < 1 or > 65535)
        {
            throw new InvalidOperationException($"Hashi port {name} must be between 1 and 65535.");
        }
    }
}
