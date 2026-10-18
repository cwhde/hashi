# AUD-119: Unknown match fields silently turn edge allow rules into catch all rules

Priority: P1

Category: Security / edge rule validation

Status: open; confirmed by manual source trace.

## Finding

Enabled edge rules validate JSON parsing and GeoIP availability only. SecurityDecisionService.Matches checks a fixed set of known properties and ignores every other property before returning true. A typo such as {"countries":"US"} on an allow rule therefore matches every request instead of being rejected. Wrong JSON value kinds can also throw during live decisions. The host condition uses arbitrary substring Contains, so an allow pattern for example.com also matches otherexample.com or example.com.attacker.invalid when such a host reaches the gateway. This is separate from AUD-009 unknown action fallback.

## Evidence

src/Hashi.Infrastructure/Platform/OidcProviderAdminService.cs:229-240; SecurityDecisionService.cs:815-855,388-399. Validation needs a schema and explicit documented host matching semantics.

## Expected outcome and acceptance criteria

Reject unsupported/empty/malformed match shapes and type errors on create/update; fail closed for malformed stored rules. Use exact or explicit suffix/glob host semantics with DNS label boundaries. Typos must never broaden an allow rule silently.
