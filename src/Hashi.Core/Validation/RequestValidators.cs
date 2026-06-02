using FluentValidation;
using Hashi.Contracts.Api;
using Hashi.Core.Resources;

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
        RuleFor(x => x.DomainMode)
            .Must(mode => string.IsNullOrWhiteSpace(mode) || ResourceDomainModeNames.IsValid(mode))
            .WithMessage($"Domain mode must be one of: {string.Join(", ", ResourceDomainModeNames.All)}.");
        RuleFor(x => x.PathRewriteMode)
            .Must(mode => string.IsNullOrWhiteSpace(mode) || ResourceRewriteModeNames.IsValid(mode))
            .WithMessage($"Path rewrite mode must be one of: {string.Join(", ", ResourceRewriteModeNames.All)}.");
        RuleFor(x => x.PathRewrite)
            .NotEmpty()
            .When(x => !string.IsNullOrWhiteSpace(x.PathRewriteMode));
        RuleForEach(x => x.Routes)
            .ChildRules(route =>
            {
                route.RuleFor(x => x.RewriteMode)
                    .Must(mode => string.IsNullOrWhiteSpace(mode) || ResourceRewriteModeNames.IsValid(mode))
                    .WithMessage($"Rewrite mode must be one of: {string.Join(", ", ResourceRewriteModeNames.All)}.");
                route.RuleFor(x => x.RewriteValue)
                    .NotEmpty()
                    .When(x => !string.IsNullOrWhiteSpace(x.RewriteMode));
            });
    }
}

public sealed class CreateSshConnectionRequestValidator : AbstractValidator<CreateSshConnectionRequest>
{
    public CreateSshConnectionRequestValidator()
    {
        RuleFor(x => x.Name).NotEmpty();
        RuleFor(x => x.ConnectionType)
            .NotEmpty()
            .Must(ConnectionTypeContractNames.IsSshConnectionType)
            .WithMessage($"Connection type must be one of: {string.Join(", ", ConnectionTypeContractNames.SshConnectionTypes)}.");
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
