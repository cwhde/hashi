using FluentValidation;
using Hashi.Contracts.Api;

namespace Hashi.Core.Validation;

public sealed class CreateResourceRequestValidator : AbstractValidator<CreateResourceRequest>
{
    public CreateResourceRequestValidator()
    {
        RuleFor(x => x.Name).NotEmpty();
        RuleFor(x => x.Kind).NotEmpty();
        RuleFor(x => x.TargetScheme).NotEmpty();
        RuleFor(x => x.TargetHost).NotEmpty();
        RuleFor(x => x.TargetPort).InclusiveBetween(1, 65535);
        RuleFor(x => x.PublicPort)
            .InclusiveBetween(1, 65535)
            .When(x => x.PublicPort.HasValue);
    }
}

public sealed class CreateSshConnectionRequestValidator : AbstractValidator<CreateSshConnectionRequest>
{
    public CreateSshConnectionRequestValidator()
    {
        RuleFor(x => x.Name).NotEmpty();
        RuleFor(x => x.ConnectionType).NotEmpty();
        RuleFor(x => x.Host).NotEmpty();
        RuleFor(x => x.Port).InclusiveBetween(1, 65535);
        RuleFor(x => x.Username).NotEmpty();
        RuleFor(x => x.AuthMode).NotEmpty();
    }
}

public sealed class RemoteWriteRequestValidator : AbstractValidator<RemoteWriteRequest>
{
    public RemoteWriteRequestValidator()
    {
        RuleFor(x => x.RemotePath).NotEmpty();
        RuleFor(x => x.ContentBase64).NotEmpty();
        RuleFor(x => x.Host).NotEmpty();
        RuleFor(x => x.Port).InclusiveBetween(1, 65535);
        RuleFor(x => x.Username).NotEmpty();
        RuleFor(x => x.AuthMode).NotEmpty();
    }
}