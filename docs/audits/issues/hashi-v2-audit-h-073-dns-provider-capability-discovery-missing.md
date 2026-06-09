# H-073: DNS Provider Capability Discovery Missing

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §15.1

## Description

The DNS provider interface (`IDnsProvider`) supports list zones, resolve zone, list/create/update/delete records, and bulk plan/apply — but has no capability discovery method. The spec requires providers to advertise supported record types, features, batch operations, and rate limits. Without this, the UI cannot adapt to provider limitations.

## Evidence

- `IDnsProvider` interface has no `GetCapabilitiesAsync`, `SupportedRecordTypes`, or similar method
- The UI hardcodes the A/AAAA/CNAME/MX/TXT type list regardless of provider

## Expected Outcome

Each DNS provider advertises its capabilities (supported record types, batch operation support, rate limits, comment/metadata support). The UI uses these to show only valid options.

## Fix Guidance

1. Add a `GetCapabilitiesAsync()` method to `IDnsProvider` returning a `DnsProviderCapabilities` object (supported record types, batch support, max records per zone, etc.).
2. `HetznerDnsProvider` implements it with its specific limits.
3. `InMemoryDnsProvider` returns all capabilities.
4. Use this in the UI to filter available record types.

## Acceptance Criteria

- [ ] `IDnsProvider` has `GetCapabilitiesAsync` method
- [ ] UI shows only record types supported by the active provider
- [ ] Provider-specific limitations are surfaced to the user
