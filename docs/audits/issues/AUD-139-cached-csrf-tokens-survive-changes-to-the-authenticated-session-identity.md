# AUD-139: Cached CSRF tokens survive changes to the authenticated session identity

Priority: P2

Category: Frontend / authentication lifecycle

Status: open; confirmed by manual source trace.

## Finding

client.ts caches the first antiforgery request token for the entire module lifetime and never clears it after login, logout or session replacement. AuthEndpoints sets NameIdentifier to the newly generated session ID; ASP.NET antiforgery request tokens are identity-bound. Svelte navigation to login and back preserves the module. After reauthentication through a new login in the same page lifetime, later mutations can keep sending the old token and receive403 until a full reload. No CSRF bypass is claimed; this is a persistent usability failure.

## Evidence

web/src/lib/api/client.ts:11-44,209-235; web/src/lib/navigation.ts uses SvelteKit goto; src/Hashi.Api/Features/Auth/AuthEndpoints.cs:338-356.

## Expected outcome and acceptance criteria

Invalidate/refetch the request token on every identity transition and handle a genuine stale-token response with a bounded deliberate retry policy. Login/session expiry/relogin without a hard page reload must allow the next authorized mutation.
