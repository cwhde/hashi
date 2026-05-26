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
    public void SshConnectionValidator_rejects_invalid_port()
    {
        var validator = new CreateSshConnectionRequestValidator();
        var request = new CreateSshConnectionRequest(
            "prod-box",
            "ssh",
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
}