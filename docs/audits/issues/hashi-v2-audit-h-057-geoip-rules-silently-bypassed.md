# H-057: GeoIP Rules Silently Bypassed When GeoIP Is Unavailable — Fail-Open for Security Rules

**Priority:** Critical
**Conflict Type:** wrong_implementation
**Spec Reference:** Main Spec §6 (GeoIP rules become invalid with clear validation error and cannot be enabled when GeoIP unavailable)

## Description

When GeoIP is unavailable, Country/Region/ASN match types simply fail to match (return false) in `SecurityDecisionService.MatchesResourceRule()`. This means rules meant to deny access are silently bypassed. The spec explicitly states that GeoIP-dependent rules must become invalid with a clear validation error and cannot be enabled when the GeoIP database is unavailable. Current behavior is fail-open for security rules, which is dangerous.

## Evidence

SecurityDecisionService.MatchesResourceRule() — when GeoIP is unavailable, request.CountryCode/RegionCode/Asn are null, causing all GeoIP match rules to return false (no match = no action = traffic allowed). GeoIpLookupService.ValidateGeoMatchRules() only validates at rule-creation time, not at evaluation time.

## Expected Outcome

When GeoIP is unavailable, rules depending on Country/Region/ASN are flagged as "cannot evaluate" and the system fails closed (treat as deny) or explicitly marks them as invalidated, not silently skips them.

## Fix Guidance

In DecideForwardAuthAsync, before rule evaluation, check if GeoIP is available. If not, and a rule depends on GeoIP match types, treat the rule as matched-with-deny-action (fail-closed), or refuse to evaluate and return a service-unavailable response, or at minimum add a warning to the decision explanation.

## Acceptance Criteria

- [ ] When GeoIP is offline, a Country-deny rule still blocks access (fail-closed)
- [ ] GeoIP-dependent rules show validation errors in the UI when GeoIP is unavailable
- [ ] Rules cannot be enabled when GeoIP data is missing
