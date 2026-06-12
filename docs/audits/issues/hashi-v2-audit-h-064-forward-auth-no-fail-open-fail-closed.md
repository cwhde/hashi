# H-064: Forward Auth Has No Fail-Open/Fail-Closed Policy — Errors Implicitly Deny Access

**Priority:** High
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §11 (For resources without SSO required, fail open only when Hashi is unreachable and no active challenge/block policy exists); Addendum §14 (Configurable forward-auth failure policy)

**Status:** Fixed
**Branch:** h/security-2

## Description

There is no configurable fail-open/fail-closed behavior for the forward-auth endpoint. When the decision service throws an exception, the endpoint returns a 500 error, implicitly denying access. The spec requires that for resources with SSO required, forward-auth must fail closed, but for resources without SSO, it should fail open only when Hashi is unreachable and no active challenge/block policy exists. Neither behavior is configurable.

## Evidence

No FailOpen, FailClosed, fail_open, or fail_closed setting found anywhere in the codebase. The edge-auth/forward endpoint has no try-catch that would implement a fail-open path.

## Expected Outcome

Administrator can configure whether forward-auth fails open (allow traffic on error) or fails closed (deny traffic on error), or the system should automatically fail-open for non-SSO resources and fail-closed for SSO-required resources.

## Fix Guidance

Add a ForwardAuthFailurePolicy setting (open/closed/auto). In the /api/edge-auth/forward endpoint, catch exceptions from the decision service and apply the policy: for "auto", fail-closed if the resource requires SSO, fail-open otherwise (unless active blocks exist).

## Acceptance Criteria

- [ ] Forward-auth endpoint has explicit error handling with fail-open/fail-closed logic
- [ ] SSO-required resources fail closed on decision service errors
- [ ] Non-SSO resources can be configured to fail open when Hashi is unreachable
