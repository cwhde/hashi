# AUD-009: Unknown edge-auth rule actions fail open

Priority: P1 — access control

Spec reference: implementation spec §11–13 and addendum §5.2 require deterministic edge-auth decisions and deny-by-default handling for invalid request/rule state.

## Finding

The edge-auth rule management service validates match JSON but does not validate `Action` when creating or updating a rule. In `SecurityDecisionService`, a matching rule maps `deny`/`block` to deny and `redirect`/`auth`/`require_sso` to SSO; every other action falls through to `Allow`. A typo or unsupported action can therefore turn a matching policy rule into an allow decision.

## Evidence

- `src/Hashi.Infrastructure/Platform/OidcProviderAdminService.cs`: `CreateRuleAsync` and `UpdateRuleAsync` assign `request.Action` without validation; `ValidateRule` only examines match JSON and GeoIP availability.
- `src/Hashi.Infrastructure/Platform/SecurityDecisionService.cs`: `EvaluateGlobalEdgeRulesAsync` returns `Allow(...)` from the switch default for any unrecognized action.
- `src/Hashi.Api/Features/EdgeSsoAdmin/EdgeSsoAdminEndpoints.cs`: rule create/update endpoints pass requests to this service.

## Expected outcome

Only documented rule actions can be stored. Unknown or corrupt stored actions must fail closed and produce an observable configuration error instead of allowing upstream access.

## Acceptance criteria

- Create and update reject unsupported action values before persistence.
- Evaluation of an unknown legacy/stored action denies access and records an actionable error.
- Tests cover misspellings and malformed stored action values.
