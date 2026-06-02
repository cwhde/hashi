# E05 - Abuse decision state machine collapses required states

Priority: High

Spec conflicts: section 13.3 requires explicit IP/security subject states: observed, warm, suspect, challenged, soft blocked, firewall blocked, manually allowed, and manually blocked. It also defines a progression from suspect, to adaptive SSO, to soft block, to firewall block.

## Problem

The current abuse state model is a single IP bucket with a score and one of three implementation states: `watch`, `challenge`, or `block`. It does not represent the required intermediate and manual states, and it promotes directly from challenge to a global firewall blocklist entry when the score reaches the block threshold.

Because soft block and firewall block are collapsed, Hashi cannot distinguish an edge-only block from a firewall-synced block. Because manual allow and manual block are not represented in the abuse state machine, operators do not get the state model the spec describes for security subjects.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:623-642` requires observed, warm, suspect, challenged, soft blocked, firewall blocked, manually allowed, and manually blocked states with staged escalation.
- `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:403-413` defines `AbuseBucketEntity` with only `ClientIp`, `Score`, `State`, and `UpdatedAtUtc`; the default state is `watch`.
- `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:31-36` maps the score directly to `watch`, `challenge`, or `block`.
- `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:115-145` creates and syncs a global IP blocklist entry as soon as the bucket reaches `block`.
- `src/Hashi.Infrastructure/Platform/EdgeAuthService.cs:122-141` only treats bucket states `block` and `challenge` specially at request time.

## Expected outcome

Hashi should model the spec's abuse/security subject states explicitly enough to preserve the difference between observation, suspicion, adaptive challenge, edge soft block, firewall block, manual allow, and manual block.

## Fix guidance

Add shared state constants and migrate the existing `watch`/`challenge`/`block` values into the spec vocabulary. Keep edge soft blocks separate from firewall-synced blocks. Decide how manual allow/block entries interact with automatic scoring and document that in code and tests. If the product intentionally keeps a smaller state machine, update the spec instead of leaving the implementation and spec in conflict.

## Acceptance criteria

- Abuse/security subject records can represent each state listed in section 13.3.
- High traffic first enters a suspect state before adaptive challenge.
- Continued anonymous or WAF/rate-threshold traffic can enter a soft-block state without immediately syncing to firewall.
- Firewall sync only marks a subject firewall blocked after the firewall apply path succeeds or records per-host failures.
- Manual allow and manual block states override or interact with automatic scoring according to a tested policy.
- Dashboard and audit views can surface the current state without inferring it from unrelated blocklist rows.
