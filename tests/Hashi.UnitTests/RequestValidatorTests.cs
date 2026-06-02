using Hashi.Contracts.Api;
using Hashi.Core.Validation;
using Xunit;

namespace Hashi.UnitTests;

public sealed class RequestValidatorTests
{
    [Fact]
    public void CreateResourceValidator_rejects_empty_name()
    {
        var validator = new CreateResourceRequestValidator();
        var request = new CreateResourceRequest(
            "",
            "http",
            "app.example.com",
            "http",
            "localhost",
            8080,
            DashboardEnabled: true,
            StatusEnabled: true);

        var result = validator.Validate(request);

        Assert.False(result.IsValid);
        Assert.Contains(result.Errors, x => x.PropertyName == nameof(CreateResourceRequest.Name));
    }

    [Fact]
    public void CreateResourceValidator_rejects_invalid_domain_and_rewrite_modes()
    {
        var validator = new CreateResourceRequestValidator();
        var request = new CreateResourceRequest(
            "App",
            "http",
            "app.example.com",
            "http",
            "localhost",
            8080,
            DashboardEnabled: true,
            StatusEnabled: true,
            DomainMode: "wildcard",
            PathRewriteMode: "prefix_magic");

        var result = validator.Validate(request);

        Assert.False(result.IsValid);
        Assert.Contains(result.Errors, x => x.PropertyName == nameof(CreateResourceRequest.DomainMode));
        Assert.Contains(result.Errors, x => x.PropertyName == nameof(CreateResourceRequest.PathRewriteMode));
    }

    [Fact]
    public void CreateResourceValidator_rejects_invalid_route_rewrite_mode()
    {
        var validator = new CreateResourceRequestValidator();
        var request = new CreateResourceRequest(
            "App",
            "http",
            "app.example.com",
            "http",
            "localhost",
            8080,
            DashboardEnabled: true,
            StatusEnabled: true,
            Routes:
            [
                new ResourceRouteRequest(
                    true,
                    100,
                    "prefix",
                    "/",
                    "http",
                    "localhost",
                    8080,
                    RewriteMode: "prefix_magic",
                    RewriteValue: "/")
            ]);

        var result = validator.Validate(request);

        Assert.False(result.IsValid);
        Assert.Contains(result.Errors, x => x.PropertyName.Contains(nameof(ResourceRouteRequest.RewriteMode), StringComparison.Ordinal));
    }

    [Fact]
    public void SshConnectionValidator_rejects_invalid_port()
    {
        var validator = new CreateSshConnectionRequestValidator();
        var request = new CreateSshConnectionRequest(
            "prod-box",
            ConnectionTypeContractNames.TraefikHost,
            "ssh.example.com",
            70000,
            "root",
            "password",
            "secret",
            null,
            null);

        var result = validator.Validate(request);

        Assert.False(result.IsValid);
        Assert.Contains(result.Errors, x => x.PropertyName == nameof(CreateSshConnectionRequest.Port));
    }

    [Theory]
    [InlineData(ConnectionTypeContractNames.TraefikHost)]
    [InlineData(ConnectionTypeContractNames.FirewallHost)]
    public void SshConnectionValidator_accepts_canonical_ssh_connection_types(string connectionType)
    {
        var validator = new CreateSshConnectionRequestValidator();
        var request = ValidSshConnectionRequest(connectionType);

        var result = validator.Validate(request);

        Assert.True(result.IsValid);
    }

    [Theory]
    [InlineData("traefik")]
    [InlineData("firewall")]
    [InlineData("ssh")]
    [InlineData("dns_provider")]
    public void SshConnectionValidator_rejects_unknown_or_non_ssh_connection_types(string connectionType)
    {
        var validator = new CreateSshConnectionRequestValidator();
        var request = ValidSshConnectionRequest(connectionType);

        var result = validator.Validate(request);

        Assert.False(result.IsValid);
        Assert.Contains(result.Errors, x => x.PropertyName == nameof(CreateSshConnectionRequest.ConnectionType));
    }

    private static CreateSshConnectionRequest ValidSshConnectionRequest(string connectionType)
        => new(
            "prod-box",
            connectionType,
            "ssh.example.com",
            22,
            "root",
            "password",
            "secret",
            null,
            null);
}
