using Hashi.Infrastructure.Services;

namespace Hashi.Infrastructure.Auth;

public sealed class BootstrapAuthService(SetupStateService setupState, AuditService audit)
{
    public async Task<BootstrapLoginResult> ValidateAsync(
        string username,
        string password,
        CancellationToken cancellationToken = default)
    {
        var state = await setupState.GetOrCreateAsync(cancellationToken);
        if (state.IsComplete)
        {
            return BootstrapLoginResult.Failed("Bootstrap login is disabled after setup completes.");
        }

        if (string.IsNullOrEmpty(state.BootstrapUsername) || string.IsNullOrEmpty(state.BootstrapPasswordHash))
        {
            return BootstrapLoginResult.Failed("Bootstrap credentials are not available.");
        }

        if (!string.Equals(username, state.BootstrapUsername, StringComparison.Ordinal))
        {
            await audit.WriteAsync("auth", "bootstrap_login_failed", outcome: "failure", cancellationToken: cancellationToken);
            return BootstrapLoginResult.Failed("Invalid credentials.");
        }

        if (!BCrypt.Net.BCrypt.Verify(password, state.BootstrapPasswordHash))
        {
            await audit.WriteAsync("auth", "bootstrap_login_failed", outcome: "failure", cancellationToken: cancellationToken);
            return BootstrapLoginResult.Failed("Invalid credentials.");
        }

        await audit.WriteAsync("auth", "bootstrap_login", subjectType: "admin", cancellationToken: cancellationToken);
        return BootstrapLoginResult.Success();
    }
}

public sealed record BootstrapLoginResult(bool Succeeded, string? Error)
{
    public static BootstrapLoginResult Success() => new(true, null);

    public static BootstrapLoginResult Failed(string error) => new(false, error);
}
