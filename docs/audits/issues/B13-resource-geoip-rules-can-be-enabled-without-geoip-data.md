# B13 - Resource GeoIP rules can be enabled without GeoIP data

Priority: High

Spec conflicts: section 6 Resource Rule Model and section 12. Country, region, and ASN rules require GeoIP data and must be invalid or disabled when GeoIP is unavailable.

## Problem

Hashi has GeoIP validation for Edge SSO rule JSON, but resource rules are stored directly without checking whether GeoIP data is present. A user can enable `country`, `region`, or `asn` resource rules even when there is no GeoIP database, which means the rules cannot evaluate as intended.

## Evidence

- `src/Hashi.Infrastructure/Platform/GeoIpLookupService.cs:80-92` can report that country, region, and ASN matches require GeoIP data.
- `src/Hashi.Infrastructure/Platform/OidcProviderAdminService.cs:208` uses that validation for Edge SSO rules.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:60-65` accepts resource rule requests with `Enabled`, `Action`, `MatchType`, and `MatchValue`.
- `src/Hashi.Infrastructure/Platform/PlatformServices.cs:324-343` writes resource rules directly from the request.
- `src/Hashi.Infrastructure/Platform/EdgeAuthService.cs:203-210` evaluates `country`, `region`, and `asn` resource rules, but no creation-time validation prevents enabling them without GeoIP data.
- `src/Hashi.Core/Validation/RequestValidators.cs:8-18` validates basic resource fields but does not validate resource rules.

## Expected outcome

Resource rules using country, region, or ASN must not be enabled unless GeoIP data is available and the requested match type/value is valid.

## Fix guidance

Add resource-rule validation in `ResourceService` or its validator. Reuse `GeoIpLookupService` for GeoIP-dependent rules and return a clear API error when the database is missing. Consider automatically disabling existing GeoIP-dependent resource rules and surfacing a health warning if GeoIP data disappears.

## Acceptance criteria

- Creating or updating an enabled `country`, `region`, or `asn` resource rule fails when GeoIP data is unavailable.
- Non-GeoIP resource rules still work without GeoIP data.
- Existing GeoIP-dependent rules are flagged or disabled if GeoIP data is removed.
- Tests cover create and update paths for GeoIP and non-GeoIP rule types.
