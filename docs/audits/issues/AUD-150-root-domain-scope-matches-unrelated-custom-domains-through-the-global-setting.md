# AUD-150: Root domain scope matches unrelated custom domains through the global setting

Priority: P1

Category: Security / manual policy scope

Status: open; confirmed by manual source trace.

## Finding

MatchesScope treats a manual root-domain entry as matching whenever ScopeId equals the installation's configured RootDomain, regardless of the current request host. Thus an entry scoped to the normal root domain also affects custom external-domain resources in that installation. Conversely an exact apex match for a different scoped domain is not accepted by the suffix-only alternative. This affects both allow and block entries independently of the global subject-state leakage in AUD-077.

## Evidence

src/Hashi.Infrastructure/Platform/SecurityDecisionService.cs:678-697. Context.RootDomain is loaded from global AppSettings in BuildContextAsync, not derived from the current host.

## Expected outcome and acceptance criteria

Match the canonical request host to the specified apex or a dot-boundary subdomain, then apply resource/root-domain scope intentionally. A normal root-domain allow/block must not affect a separate custom domain.
