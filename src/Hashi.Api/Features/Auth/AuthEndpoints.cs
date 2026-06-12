using System.Security.Claims;
using Fido2NetLib;
using Hashi.Api.Hosting;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Bootstrap;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.AspNetCore.Authentication;
using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Antiforgery;
using Microsoft.AspNetCore.Http.HttpResults;
using Microsoft.AspNetCore.Mvc;

namespace Hashi.Api.Features.Auth;

public static class AuthEndpoints
{
    public static IEndpointRouteBuilder MapAuthEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/auth").WithTags("Auth");

        group.MapGet("/csrf", (IAntiforgery antiforgery, HttpContext httpContext) =>
        {
            var tokens = antiforgery.GetAndStoreTokens(httpContext);
            return TypedResults.Ok(new { token = tokens.RequestToken });
        });

        group.MapPost("/bootstrap/login", async Task<IResult> (
            BootstrapLoginRequest request,
            HttpContext httpContext,
            BootstrapAuthService bootstrapAuth,
            AdminSessionService sessions,
            ForwardedClientContextResolver forwardedClientContext,
            VaultService vault,
            CancellationToken ct) =>
        {
            var remoteIp = httpContext.Connection.RemoteIpAddress?.ToString();
            if (!BootstrapNetworkPolicy.IsAllowed(remoteIp))
            {
                return TypedResults.Forbid();
            }

            var result = await bootstrapAuth.ValidateAsync(request.Username, request.Password, ct);
            if (!result.Succeeded)
            {
                return TypedResults.Unauthorized();
            }

            var client = forwardedClientContext.Resolve(httpContext);
            var session = await sessions.CreateAsync(
                AdminAuthMethods.Bootstrap,
                client.ClientIp.ToString(),
                AdminSessionScopes.Bootstrap,
                userAgent: httpContext.Request.Headers.UserAgent.ToString(),
                cancellationToken: ct);
            await SignInAsync(httpContext, session, vaultUnlocked: false);
            return TypedResults.Ok(new BootstrapLoginResponse(true, null));
        });

        group.MapPost("/passkeys/register/begin", async Task<IResult> (
            HttpContext httpContext,
            PasskeyAuthService passkeys,
            WebAuthnChallengeStore challenges,
            [FromQuery] string nickname,
            [FromQuery] bool? afterSetup,
            CancellationToken ct) =>
        {
            if (!IsAuthenticatedDuringSetup(httpContext) && httpContext.User.Identity?.IsAuthenticated != true)
            {
                return TypedResults.Unauthorized();
            }

            var options = await passkeys.BeginRegistrationAsync(nickname, allowAfterSetupComplete: afterSetup == true, ct);
            var sessionId = Guid.NewGuid().ToString("N");
            challenges.StoreRegistration(sessionId, options);
            return TypedResults.Ok(new PasskeyRegistrationBeginResponse(options, sessionId));
        });

        group.MapGet("/passkeys", async (PasskeyAuthService passkeys, CancellationToken ct) =>
        {
            var items = await passkeys.ListAsync(ct);
            return TypedResults.Ok(items.Select(x => new PasskeySummaryResponse(x.Id, x.Nickname, x.PrfSupported, x.CreatedAtUtc)));
        });

        group.MapDelete("/passkeys/{credentialId:guid}", async Task<IResult> (
            Guid credentialId,
            PasskeyAuthService passkeys,
            CancellationToken ct) =>
        {
            try
            {
                var removed = await passkeys.DeleteAsync(credentialId, ct);
                return removed ? TypedResults.NoContent() : TypedResults.NotFound();
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });

        group.MapPost("/passkeys/register/complete", async Task<IResult> (
            PasskeyRegistrationCompleteRequest request,
            HttpContext httpContext,
            PasskeyAuthService passkeys,
            WebAuthnChallengeStore challenges,
            CancellationToken ct) =>
        {
            if (!IsAuthenticatedDuringSetup(httpContext))
            {
                return TypedResults.Unauthorized();
            }

            var options = challenges.GetRegistration(request.ChallengeSessionId);
            if (options is null)
            {
                return TypedResults.BadRequest(new ApiErrorResponse("Registration challenge expired or invalid."));
            }

            var attestation = System.Text.Json.JsonSerializer.Deserialize<AuthenticatorAttestationRawResponse>(
                System.Text.Json.JsonSerializer.Serialize(request.Attestation))
                ?? throw new InvalidOperationException("Invalid attestation payload.");

            var result = await passkeys.CompleteRegistrationAsync(
                attestation,
                options,
                request.Nickname,
                request.ClientReportsPrfSupported,
                ct);
            return TypedResults.Ok(new PasskeyRegistrationCompleteResponse(result.CredentialId, result.PrfSupported));
        });

        group.MapPost("/passkeys/login/begin", async Task<IResult> (
            PasskeyAuthService passkeys,
            WebAuthnChallengeStore challenges,
            CancellationToken ct) =>
        {
            var options = await passkeys.BeginLoginAsync(ct);
            var sessionId = Guid.NewGuid().ToString("N");
            challenges.StoreLogin(sessionId, options);
            return TypedResults.Ok(new PasskeyLoginBeginResponse(options, sessionId));
        });

        group.MapPost("/passkeys/login/complete", async Task<IResult> (
            PasskeyLoginCompleteRequest request,
            HttpContext httpContext,
            PasskeyAuthService passkeys,
            AdminSessionService sessions,
            ForwardedClientContextResolver forwardedClientContext,
            VaultService vault,
            WebAuthnChallengeStore challenges,
            CancellationToken ct) =>
        {
            var options = challenges.GetLogin(request.ChallengeSessionId);
            if (options is null)
            {
                return TypedResults.BadRequest(new ApiErrorResponse("Login challenge expired or invalid."));
            }

            var assertion = System.Text.Json.JsonSerializer.Deserialize<AuthenticatorAssertionRawResponse>(
                System.Text.Json.JsonSerializer.Serialize(request.Assertion))
                ?? throw new InvalidOperationException("Invalid assertion payload.");

            byte[]? prfOutput = string.IsNullOrWhiteSpace(request.PrfOutputBase64)
                ? null
                : Convert.FromBase64String(request.PrfOutputBase64);

            var result = await passkeys.CompleteLoginAsync(assertion, options, prfOutput, ct);
            var client = forwardedClientContext.Resolve(httpContext);
            var session = await sessions.CreateAsync(
                AdminAuthMethods.Passkey,
                client.ClientIp.ToString(),
                AdminSessionScopes.All,
                result.CredentialId,
                httpContext.Request.Headers.UserAgent.ToString(),
                ct);
            var vaultUnlocked = false;
            if (result.PrfOutput is { Length: >= 32 })
            {
                vaultUnlocked = await vault.UnlockWithPrfAsync(result.CredentialId, result.PrfOutput, session.Id, ct);
            }

            await SignInAsync(httpContext, session, vaultUnlocked);
            return TypedResults.Ok(new PasskeyLoginCompleteResponse(true, vaultUnlocked));
        });

        group.MapGet("/session", (HttpContext httpContext, VaultSessionState vaultSession) =>
        {
            var authMethod = httpContext.User.FindFirstValue(AdminClaimTypes.AuthMethod);
            var validation = httpContext.Items[AdminSessionCookieEvents.ValidationItemKey] as AdminSessionValidationResult;
            return TypedResults.Ok(new SessionStatusResponse(
                httpContext.User.Identity?.IsAuthenticated == true,
                authMethod,
                vaultSession.IsUnlocked,
                httpContext.User.HasClaim(c => c.Type == ClaimTypes.Name && c.Value == "setup-complete"),
                validation?.Scopes,
                validation?.Session?.BoundIp,
                validation?.Session?.IdleExpiresAtUtc,
                validation?.Session?.AbsoluteExpiresAtUtc,
                validation?.Session?.ReauthenticatedAtUtc));
        });

        group.MapGet("/sessions", async (
            HttpContext httpContext,
            AdminSessionService sessions,
            CancellationToken ct) =>
        {
            var currentSessionId = CurrentSessionId(httpContext);
            var items = await sessions.ListActiveAsync(ct);
            return TypedResults.Ok(items.Select(x => new AdminSessionSummaryResponse(
                AdminSessionService.GetCorrelationId(x),
                x.AuthMethod,
                x.BoundIp,
                AdminSessionService.GetScopes(x),
                x.CreatedAtUtc,
                x.LastSeenAtUtc,
                x.IdleExpiresAtUtc,
                x.AbsoluteExpiresAtUtc,
                x.ReauthenticatedAtUtc,
                x.Id == currentSessionId)));
        });

        group.MapDelete("/sessions/{sessionId}", async Task<IResult> (
            string sessionId,
            HttpContext httpContext,
            AdminSessionService sessions,
            VaultSessionState vaultSession,
            CancellationToken ct) =>
        {
            var currentSessionId = CurrentSessionId(httpContext);
            var revoked = await sessions.RevokeByCorrelationIdAsync(sessionId, "manual", ct);
            if (!revoked)
            {
                return TypedResults.NotFound();
            }

            var currentCorrelationId = AdminSessionService.GetCorrelationId(currentSessionId);
            if (string.Equals(sessionId, currentCorrelationId, StringComparison.OrdinalIgnoreCase))
            {
                vaultSession.LockForSession(currentSessionId);
                await httpContext.SignOutAsync(CookieAuthenticationDefaults.AuthenticationScheme);
            }

            return TypedResults.NoContent();
        });

        group.MapPost("/sessions/revoke-others", async (
            HttpContext httpContext,
            AdminSessionService sessions,
            CancellationToken ct) =>
        {
            var count = await sessions.RevokeOtherSessionsAsync(CurrentSessionId(httpContext), ct);
            return TypedResults.Ok(new RevokeOtherSessionsResponse(count));
        });

        group.MapPost("/reauthenticate", async Task<IResult> (
            HttpContext httpContext,
            PasskeyAuthService passkeys,
            WebAuthnChallengeStore challenges,
            CancellationToken ct) =>
        {
            if (httpContext.User.Identity?.IsAuthenticated != true)
            {
                return TypedResults.Unauthorized();
            }

            var options = await passkeys.BeginLoginAsync(ct);
            var sessionId = Guid.NewGuid().ToString("N");
            challenges.StoreLogin(sessionId, options);
            return TypedResults.Ok(new PasskeyLoginBeginResponse(options, sessionId));
        });

        group.MapPost("/reauthenticate/complete", async Task<IResult> (
            PasskeyLoginCompleteRequest request,
            HttpContext httpContext,
            PasskeyAuthService passkeys,
            AdminSessionService sessions,
            WebAuthnChallengeStore challenges,
            CancellationToken ct) =>
        {
            if (httpContext.User.Identity?.IsAuthenticated != true)
            {
                return TypedResults.Unauthorized();
            }

            var options = challenges.GetLogin(request.ChallengeSessionId);
            if (options is null)
            {
                return TypedResults.BadRequest(new ApiErrorResponse("Reauthentication challenge expired or invalid."));
            }

            var assertion = System.Text.Json.JsonSerializer.Deserialize<AuthenticatorAssertionRawResponse>(
                System.Text.Json.JsonSerializer.Serialize(request.Assertion))
                ?? throw new InvalidOperationException("Invalid assertion payload.");

            await passkeys.CompleteLoginAsync(assertion, options, null, ct);
            await sessions.MarkReauthenticatedAsync(CurrentSessionId(httpContext), ct);
            return TypedResults.Ok(new { reauthenticated = true });
        });

        group.MapPost("/logout", async Task<IResult> (
            HttpContext httpContext,
            AdminSessionService sessions,
            VaultService vault,
            CancellationToken ct) =>
        {
            await sessions.RevokeAsync(CurrentSessionId(httpContext), "logout", ct);
            await vault.LockAsync(ct);
            await httpContext.SignOutAsync(CookieAuthenticationDefaults.AuthenticationScheme);
            return TypedResults.Ok(new LogoutResponse(true));
        });

        return app;
    }

    private static bool IsAuthenticatedDuringSetup(HttpContext httpContext)
        => httpContext.User.Identity?.IsAuthenticated == true;

    internal static async Task SignInAsync(HttpContext httpContext, AdminSessionEntity session, bool vaultUnlocked)
    {
        var claims = new List<Claim>
        {
            new(ClaimTypes.NameIdentifier, session.Id),
            new(ClaimTypes.Sid, session.Id),
            new(ClaimTypes.Name, "admin"),
            new(AdminClaimTypes.AuthMethod, session.AuthMethod),
        };

        if (vaultUnlocked)
        {
            claims.Add(new Claim(AdminClaimTypes.VaultUnlocked, "true"));
        }

        var identity = new ClaimsIdentity(claims, CookieAuthenticationDefaults.AuthenticationScheme);
        await httpContext.SignInAsync(
            CookieAuthenticationDefaults.AuthenticationScheme,
            new ClaimsPrincipal(identity));
    }

    private static string CurrentSessionId(HttpContext httpContext)
        => httpContext.User.FindFirstValue(ClaimTypes.NameIdentifier)
            ?? httpContext.User.FindFirstValue(ClaimTypes.Sid)
            ?? throw new InvalidOperationException("Authenticated admin session identifier is missing.");
}
