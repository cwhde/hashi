# AUD-102: GeoIP country fallback and capability checks allow unenforceable rules

Priority: P1

Category: Security / GeoIP availability

Status: open; confirmed by manual source trace.

## Finding

GeoIpLookupService loads GeoLite2-Country into the city reader but always calls DatabaseReader.City. MaxMind rejects a Country database for City lookups; the exception is swallowed and no country is returned. IsAvailable also returns true when only ASN data exists, so country/region rules pass validation and the runtime unavailable-data deny guard is skipped. A deny-country rule then fails to match and can fall through to allow. The reverse missing-ASN case has the same capability-check defect. This is conditional on incomplete database installation or an update/load failure, not a claim that complete healthy City+ASN installations always fail.

## Evidence

src/Hashi.Infrastructure/Platform/GeoIpLookupService.cs:39-105,128-151,211-220; PlatformServices.cs:529-540; SecurityDecisionService.cs:249-274. Main specification section6 requires unavailable geo rules to be invalid and impossible to enable. MaxMind source checks Metadata.DatabaseType.Contains(type) before lookup: https://github.com/maxmind/GeoIP2-dotnet/blob/main/MaxMind.GeoIP2/DatabaseReader.cs .

## Expected outcome and acceptance criteria

Track country/region/ASN availability independently, call Country for a Country database, and reject or fail closed for each required missing capability. Synchronize reload and lookup lifetimes so a disposed reader cannot silently remove enforcement.
