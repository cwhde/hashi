# AUD-099: Ban policy settings and repeat-offender semantics are incomplete

Priority: P1

Category: Missing feature / security policy correctness

Status: open; confirmed by manual source and API/UI tracing.

## Finding

Security policy records and contracts exist, but there is no registered API or UI for reading/updating the ban policies. CAPTCHA escalation always evaluates both soft and firewall policies with offense count `1`, despite storing cumulative offense counters. `RepeatOffenderPolicyJson`, `CountWindowSeconds`, `ResetCountAfterSeconds` and `ChallengeIgnoredWindowSeconds` have no runtime consumers implementing their behavior. The stored repeat-offender and window configuration therefore cannot control enforcement.

Permanent results are also lost: duration helpers replace a null permanent duration with 10 minutes or one hour, and callers always assign a finite timestamp. Continued challenged hits increment offense/block counters every time the threshold remains exceeded, rather than only on a new escalation. This makes later use of those counters incorrect without a lifecycle repair.

This is distinct from AUD-066 arithmetic overflow and AUD-072 the separate lifetime abuse-score path. Repairing either of those alone does not implement configurable policy windows and repeat bans.

## Evidence

- `src/Hashi.Infrastructure/Platform/CaptchaChallengeService.cs:309-339,762-784`: threshold counters and constant count `1`; null-duration substitution.
- `src/Hashi.Infrastructure/Platform/BanDurationPolicyEvaluator.cs:7-16,29-58,62-76`: policy contract, permanent results, window fields carried but not evaluated.
- `src/Hashi.Infrastructure/Persistence/Entities/CoreEntities.cs:117-135` and `src/Hashi.Contracts/Api/PlatformContracts.cs:867-886`: persisted/contract policy scaffolding.
- Full API registration, OpenAPI and frontend reading found no policy administration path.
- Addendum §6 and §12 require configurable ban policies, independent offense history and window/reset semantics.

## Expected outcome

Expose validated policy configuration; count distinct offenses in the configured windows; implement reset and repeat-offender policies; preserve permanent bans explicitly and apply the same rules across escalation paths.

## Acceptance criteria

Configured constant, linear, exponential, capped and permanent-after-N policies change real enforcement. Repeated events within one active block do not create multiple new bans; expiry, reset windows and permanent state work. Configuration round-trips through the actual UI/API.
