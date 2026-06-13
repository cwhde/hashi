using System.Security.Claims;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Microsoft.AspNetCore.Authentication.Cookies;

namespace Hashi.Api.Hosting;

public sealed class AdminSessionCookieEvents(
    AdminSessionService sessions,
    ForwardedClientContextResolver forwardedClientContext,
    VaultSessionState vaultSession) : CookieAuthenticationEvents
{
    public const string ValidationItemKey = "hashi.admin_session_validation";

    public override async Task ValidatePrincipal(CookieValidatePrincipalContext context)
    {
        var sessionId = context.Principal?.FindFirstValue(ClaimTypes.NameIdentifier)
            ?? context.Principal?.FindFirstValue(ClaimTypes.Sid);
        if (!forwardedClientContext.TryResolve(context.HttpContext, out var client))
        {
            if (!string.IsNullOrWhiteSpace(sessionId))
            {
                await sessions.RevokeAsync(
                    sessionId,
                    "client_ip_unavailable",
                    context.HttpContext.RequestAborted);
                vaultSession.LockForSession(sessionId);
            }

            RejectAndClearCookie(context);
            return;
        }

        var validation = await sessions.ValidateAsync(
            sessionId,
            client.ClientIp.ToString(),
            context.HttpContext.RequestAborted);

        if (!validation.IsValid || validation.Session is null)
        {
            if (!string.IsNullOrWhiteSpace(sessionId))
            {
                vaultSession.LockForSession(sessionId);
            }

            RejectAndClearCookie(context);
            return;
        }

        var authMethod = context.Principal?.FindFirstValue(AdminClaimTypes.AuthMethod);
        if (!string.Equals(authMethod, validation.Session.AuthMethod, StringComparison.Ordinal))
        {
            await sessions.RevokeAsync(validation.Session.Id, "claim_mismatch", context.HttpContext.RequestAborted);
            vaultSession.LockForSession(validation.Session.Id);
            RejectAndClearCookie(context);
            return;
        }

        context.HttpContext.Items[ValidationItemKey] = validation;
    }

    private static void RejectAndClearCookie(CookieValidatePrincipalContext context)
    {
        context.RejectPrincipal();
        context.HttpContext.Response.Cookies.Delete("hashi.session", new CookieOptions
        {
            HttpOnly = true,
            Secure = true,
            SameSite = SameSiteMode.Strict,
            Path = "/",
        });
    }
}
