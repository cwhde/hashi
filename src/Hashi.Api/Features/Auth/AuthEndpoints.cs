using System.Security.Claims;
using Fido2NetLib;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Bootstrap;
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

            await SignInAsync(httpContext, AdminAuthMethods.Bootstrap, vaultUnlocked: false);
            return TypedResults.Ok(new BootstrapLoginResponse(true, null));
        });

        group.MapPost("/passkeys/register/begin", async Task<IResult> (
            HttpContext httpContext,
            PasskeyAuthService passkeys,
            WebAuthnChallengeStore challenges,
            [FromQuery] string nickname,
            CancellationToken ct) =>
        {
            if (!IsAuthenticatedDuringSetup(httpContext))
            {
                return TypedResults.Unauthorized();
            }

            var options = await passkeys.BeginRegistrationAsync(nickname, ct);
            var sessionId = Guid.NewGuid().ToString("N");
            challenges.StoreRegistration(sessionId, options);
            return TypedResults.Ok(new PasskeyRegistrationBeginResponse(options, sessionId));
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
            var vaultUnlocked = false;
            if (result.PrfOutput is { Length: >= 32 })
            {
                vaultUnlocked = await vault.UnlockWithPrfAsync(result.CredentialId, result.PrfOutput, ct);
            }

            await SignInAsync(httpContext, AdminAuthMethods.Passkey, vaultUnlocked);
            return TypedResults.Ok(new PasskeyLoginCompleteResponse(true, vaultUnlocked));
        });

        group.MapGet("/session", (HttpContext httpContext, VaultSessionState vaultSession) =>
        {
            var authMethod = httpContext.User.FindFirstValue(AdminClaimTypes.AuthMethod);
            return TypedResults.Ok(new SessionStatusResponse(
                httpContext.User.Identity?.IsAuthenticated == true,
                authMethod,
                vaultSession.IsUnlocked,
                httpContext.User.HasClaim(c => c.Type == ClaimTypes.Name && c.Value == "setup-complete")));
        });

        group.MapPost("/reauthenticate", async Task<IResult> (
            HttpContext httpContext,
            ReauthenticationState reauth,
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
            ReauthenticationState reauth,
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
            reauth.MarkRecent(httpContext);
            return TypedResults.Ok(new { reauthenticated = true });
        });

        group.MapPost("/logout", async Task<IResult> (
            HttpContext httpContext,
            VaultService vault,
            CancellationToken ct) =>
        {
            await vault.LockAsync(ct);
            await httpContext.SignOutAsync(CookieAuthenticationDefaults.AuthenticationScheme);
            return TypedResults.Ok(new LogoutResponse(true));
        });

        return app;
    }

    private static bool IsAuthenticatedDuringSetup(HttpContext httpContext)
        => httpContext.User.Identity?.IsAuthenticated == true;

    internal static async Task SignInAsync(HttpContext httpContext, string authMethod, bool vaultUnlocked)
    {
        var claims = new List<Claim>
        {
            new(ClaimTypes.Name, "admin"),
            new(AdminClaimTypes.AuthMethod, authMethod),
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
}
