# AUD-037: Auto fail-open fallback misses active block and challenge state

Priority: P2 — an evaluation exception can bypass active security policy when `auto` fail-open is configured

Category: Security enforcement / fail-open policy

Spec reference: `docs/implementation-spec/hashi-v2-implementation-spec.md` §11 requires fail-open for non-SSO resources only when Hashi is unreachable and no active challenge/block policy exists; SSO-required resources fail closed.

## Finding

`ResolveFailureDecisionAsync` implements the `auto` fallback by checking only enabled, unexpired manual block and blocklist rows whose subject type is exactly `ip` and whose normalized value exactly equals the request IP. It does not evaluate CIDR, country, region, or ASN entries, active `SecuritySubjectState` challenge/soft-block/firewall-block state, or resource-scoped policy. It then allows when the resolved resource is present and its policy is `off` or `observe`.

If the primary decision path throws while the database remains queryable, configuring `Hashi:ForwardAuthFailurePolicy=auto` can therefore allow a request despite an active block/challenge policy that the normal path would enforce. The shipped `appsettings.json` default is `closed`, so exposure requires the optional `auto` setting and an exception in the main evaluator.

## Evidence

- `src/Hashi.Api/Features/EdgeAuth/EdgeAuthEndpoints.cs`: `ResolveFailureDecisionAsync` checks only exact IP matches in `ManualSecurityEntries` and `BlocklistEntries`; its auto allow condition is `!activeBlock && resource != null && policy is Off or Observe`.
- `src/Hashi.Infrastructure/Platform/SecurityDecisionService.cs`: normal evaluation separately handles active subject state, resource-scoped manual entries, firewall subjects, and blocklist matching.
- `src/Hashi.Api/appsettings.json`: shipped default is `closed`.

## Expected outcome

The fallback must implement the specification's “no active challenge/block policy” condition conservatively. If it cannot reliably determine whether any applicable block or challenge is active, it should fail closed. It must also never permit the fallback for SSO-required resources.

## Acceptance criteria

- `auto` never allows when an applicable manual/blocklist CIDR, geo/ASN rule, resource-scoped block, firewall block, soft block, or active challenge applies.
- Uncertainty while checking active policy results in denial.
- SSO-required resources remain closed when evaluation fails.
- The fallback's permitted cases are documented and match §11 behavior.
