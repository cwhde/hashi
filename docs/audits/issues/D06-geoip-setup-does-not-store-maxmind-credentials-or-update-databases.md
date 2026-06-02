# D06 - GeoIP setup does not store MaxMind credentials or update databases

Priority: Medium

Spec conflicts: section 7.9 includes MaxMind account ID and license key for GeoLite2 Country and ASN databases as optional setup fields. Section 24 requires GeoIP update settings under Security settings.

## Problem

GeoIP lookup support exists only as a passive local-file reader. The optional setup UI links to MaxMind signup and tells the operator to manually mount database files under `/data/geoip`, but it does not collect or store the MaxMind account ID/license key. There is no backend settings model, secret storage path, update job, or database metadata for downloading and refreshing GeoLite2 databases.

This leaves country/region/ASN rule evaluation dependent on manually managed files and does not implement the spec's setup/update path.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:376-382` lists MaxMind account ID and license key as an optional setup step.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:1261-1267` lists GeoIP update settings under Security settings.
- `src/Hashi.Infrastructure/Platform/GeoIpLookupService.cs:12-27` resolves only a local `/data/geoip` path and reports availability from local database readers.
- `src/Hashi.Infrastructure/Platform/GeoIpLookupService.cs:97-123` opens `GeoLite2-City.mmdb`, `GeoLite2-Country.mmdb`, and `GeoLite2-ASN.mmdb` from that local directory; it does not download or update them.
- `web/src/lib/components/setup/steps/OptionalStep.svelte:249-271` only toggles an informational GeoLite2 panel with manual download/mount instructions and a signup link.
- `rg -n "MaxMind|GeoLite2|GeoIP|license|AccountId|Updater" src web/src/lib/api tests` finds the MaxMind reader package and GeoIP validation/tests, but no credential model, update worker, or API surface for MaxMind account/license settings.

## Expected outcome

Optional setup and security settings should be able to store MaxMind account credentials securely and keep the GeoLite2 Country/City and ASN databases current according to configured update settings.

## Fix guidance

Add a typed GeoIP settings model/API with account ID, license-key secret id, enabled flag, update cadence, last update status, and database metadata. Store the license key in the vault or service-sync secret path as appropriate. Add a hosted updater that downloads databases into `/data/geoip` atomically and records success/failure state.

## Acceptance criteria

- Optional setup can collect MaxMind account ID and license key without storing the license key in plaintext.
- Security settings expose GeoIP update settings and last update status.
- A background updater downloads and atomically replaces GeoLite2 databases under `/data/geoip`.
- Missing or failed GeoIP updates produce clear health/settings feedback.
- Existing country/region/ASN rule validation continues to reject enabled rules when databases are unavailable.
- Tests cover credential storage, updater success/failure, and reader reload behavior after update.
