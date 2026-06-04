# TASK-02: Security Decision Engine

## Goal

Replace the current scattered security decision logic with a deterministic, explainable engine that supports normalized subjects, manual entries, challenge state, soft/firewall block state, blocklist matches, resource rules, rate buckets, and ban policies.

## Spec Context

- Original spec sections: 11, 12, 13, 17, 18, 19, 25.
- Addendum sections: 5.1, 5.2, 6, 7.4, 7.5, 7.8, 13.2, 13.3, 13.5, 14, 15, 16.

## Current Code Anchors

- Current evaluator: `src/Hashi.Infrastructure/Platform/EdgeAuthService.cs`
- Current ingestion/scoring: `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs`
- Forward-auth endpoint: `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs`
- Rule names: `src/Hashi.Core/Resources/ResourceModels.cs`
- Forwarded client context: `src/Hashi.Api/Hosting/ForwardedClientContextResolver.cs`
- GeoIP lookup: `src/Hashi.Infrastructure/Platform/GeoIpLookupService.cs`
- Existing tests: `tests/Hashi.UnitTests/EdgeAuthServiceTests.cs`, `tests/Hashi.UnitTests/SecurityIngestionServiceTests.cs`, `tests/Hashi.UnitTests/ForwardedClientContextResolverTests.cs`

## Implementation Shape

Create a focused service layer, for example:

- `SecuritySubjectNormalizer`
- `SecuritySubjectService`
- `SecurityDecisionService`
- `BanDurationPolicyEvaluator`
- `SecurityEventWriter`
- `SecurityBucketService`

The forward-auth endpoint should become a thin adapter:

1. Parse trusted request metadata.
2. Resolve resource.
3. Resolve or create normalized subject.
4. Call the decision service.
5. Persist event/bucket updates.
6. Return allow, SSO, challenge, deny, redirect, or API challenge response.

## Effective Decision Order

Implement addendum order:

1. Invalid or untrusted request metadata.
2. Manual block.
3. Firewall block.
4. High-confidence blocklist with firewall enforcement.
5. Soft block.
6. Active challenge required.
7. Resource-specific rule.
8. Manual allow.
9. Default resource behavior.

Manual allow must prevent Hashi-controlled blocking/escalation, but must not bypass SSO or CAPTCHA by default.

## Ban and Escalation

Implement safe policy evaluation:

- Constant.
- Linear.
- Exponential.
- Capped exponential.
- Permanent after N offenses.

Track offense counts separately from active block state. CAPTCHA solve may reset or decay triggering buckets, but must not erase offense history.

## Resource Rule Updates

The current resource rule actions are:

- `bypass_auth`
- `block_access`
- `pass_to_auth`
- `require_adaptive_challenge`

Addendum rule actions are:

- Allow.
- Deny.
- Require SSO.
- Require challenge.
- Soft block.
- Firewall block.
- Bypass blocking where manual allow applies.

Add aliases carefully, but persist canonical action names. Do not silently fall through on unknown actions.

## Deliverables

- Decision result object with:
  - action
  - response mode
  - status code
  - redirect URL when applicable
  - effective decision explanation
  - matched manual entries/blocklists/rules/states
  - audit/security event details
- Forward-auth endpoint migrated to the decision service.
- Security ingestion migrated from IP score-only buckets to normalized subject/bucket updates.
- Challenge state counters added but CAPTCHA verification itself remains task 04.
- Existing dashboard reads kept working or adapted to the new tables.

## Tests

- IP, CIDR, ASN, country, region normalization.
- Manual block precedence.
- Firewall block precedence.
- Blocklist precedence.
- Manual allow semantics and default bypass flags.
- Resource rule action normalization and enforcement.
- Active challenge denies upstream service access.
- Requests while challenged counters.
- Ban duration calculations.
- Forward-auth browser vs API decision response shape.
- GeoIP missing data invalidates country/region/ASN rule enablement.

## Acceptance

- Decisions are deterministic and explainable from stored data.
- Existing OIDC SSO flows still pass.
- Existing resource rule tests still pass or are updated to canonical action names.
- No upstream resource receives requests for active challenged or blocked subjects.
