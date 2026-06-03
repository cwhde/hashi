# E04 - Resource rule actions use inconsistent vocabularies

Priority: High

Spec conflicts: section 6 requires resource rules with priority ordering, actions for bypass auth, block access, pass to auth, and require adaptive challenge, and match types for IP, CIDR, path, country, region, and ASN.

## Problem

Resource rule actions are stored as arbitrary strings, and different parts of the implementation use different action vocabularies. Current tests create rules with actions such as `allow` and `block`, but forward-auth enforcement only recognizes `bypass_auth`, `block_access`, `require_adaptive_challenge`, and `pass_to_auth`.

When a matching rule has an unrecognized action, `EvaluateResourceRulesAsync` returns `null` and falls through to the normal resource/global auth path. That means an API client can store what looks like a valid block rule and have it silently not block anything.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:200-220` defines resource rule priority, actions, match types, and GeoIP validation.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:64-69` models resource rule `Action`, `MatchType`, and `MatchValue` as unconstrained strings.
- `src/Hashi.Infrastructure/Platform/PlatformServices.cs:386-405` deletes existing rules and stores the incoming `rule.Action`, `rule.MatchType`, and `rule.MatchValue` without normalizing or validating the action.
- `tests/Hashi.UnitTests/ResourceServiceRuleValidationTests.cs:18-19`, `:32-35`, and `:74` use `block` and `allow` as resource rule actions.
- `src/Hashi.Infrastructure/Platform/EdgeAuthService.cs:171-177` only enforces `bypass_auth`, `block_access`, `require_adaptive_challenge`, and `pass_to_auth`; all other matching actions return `null`.
- `src/Hashi.Infrastructure/Platform/EdgeAuthService.cs:265-281` does support the required match types, so the bug is specifically the action vocabulary and validation path.

## Expected outcome

The API, tests, UI, persistence, and forward-auth enforcement should share one canonical resource-rule action vocabulary, and unknown actions should be rejected before storage.

## Fix guidance

Introduce shared constants or an enum-like mapping for resource rule actions. Decide whether public API values should be the human/short names (`block`, `allow`, `challenge`) or the current enforcement names, then normalize consistently. Add validation for allowed actions and update tests to prove each action has the expected edge-auth effect.

## Acceptance criteria

- Creating or updating a resource rule with an unknown action fails with a clear validation error.
- A block-access rule that matches the request returns a deny/403 decision.
- A bypass-auth rule that matches the request returns an allow/204 decision.
- Pass-to-auth and require-adaptive-challenge rules exercise the expected SSO/challenge behavior.
- Resource rule action choices exposed by the UI use the same values that the API accepts and enforcement recognizes.
