# AUD-033: Firewall desired state ignores enforcement mode and manual firewall blocks

Priority: P1 — observe and middleware-only feeds can block traffic at the host firewall

Category: Wrong security enforcement / unexpected traffic blocking

Spec reference: implementation addendum §§5.2 and 8.1 require blocklist enforcement to follow each source's configured mode and only firewall-enforce eligible, explicitly selected entries.

## Finding

`FirewallApplyService.BuildHostDefinitionAsync` selects every enabled, unexpired IP/CIDR blocklist entry, regardless of its enforcement mode, whether its source supports firewall enforcement, or whether it has been approved/synced for firewall use. The resulting values are passed into the generated firewall definition and installed as host-level blocks. An observe-only or middleware-only feed can therefore deny traffic at the host firewall the next time the firewall is applied, despite its configured mode.

The same desired-state builder does not include manual security block entries marked `FirewallEnforced`. `SecuritySubjectOperationsService` records that choice in `SecuritySubjectStateEntity`, but `BuildHostDefinitionAsync` reads blocklist entries and does not read manual security entries or security-subject state. As a result, an operator can request a firewall-enforced manual block and see a preview without the intended address in the generated firewall set; applying it does not install that manual block at the host firewall.

## Evidence

- `src/Hashi.Infrastructure/Platform/FirewallApplyService.cs`: `BuildHostDefinitionAsync` filters blocklist entries by enabled state, expiry, and IP/CIDR type only; it does not filter on `EnforcementMode`, `SyncedToFirewall`, or the source's `CanFirewallEnforce` flag.
- `src/Hashi.Infrastructure/Platform/BlocklistSourceManagementService.cs`: firewall sync recommendation and pending-entry queries explicitly require firewall mode; this contrasts with the renderer query.
- `src/Hashi.Infrastructure/Platform/BlocklistSourceManagementService.cs`: new/changed firewall-recommended entries start with `SyncedToFirewall == false`, and the service reports these as pending firewall sync.
- `src/Hashi.Infrastructure/Sync/SyncOrchestratorService.cs`: `ReconcileAsync` calls `ApplyForHostAsync` for every firewall host without checking pending firewall entries or requiring a firewall-specific plan approval; the current firewall definition builder includes those pending rows.
- `src/Hashi.Infrastructure/Platform/SecurityDecisionService.cs`: middleware enforcement excludes observe-mode entries and separately distinguishes firewall-enforced entries.
- `src/Hashi.Infrastructure/Platform/SecuritySubjectOperationsService.cs`: `CreateBlockAsync` and `UpdateBlockAsync` store firewall enforcement state through `ApplyBlockStateAsync` when the request asks for firewall enforcement.
- `FirewallApplyService.BuildHostDefinitionAsync` does not query `ManualSecurityEntries`, `SecuritySubjectStates`, or `FirewallBlockSubjects` when constructing its blocked IP list.
- `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md` §§5.2 and 8.1: the decision order and per-list enforcement-mode configuration distinguish observe, middleware, and firewall behavior.

## Expected outcome

Build firewall sets from exactly the active, eligible, explicitly approved firewall-enforced desired state, including manual blocks configured for firewall enforcement. Do not include observe/middleware-only entries. Preserve the review/confirmation path when those entries change.

## Acceptance criteria

- Observe-only and middleware-only entries never appear in generated firewall sets.
- A source that cannot firewall-enforce cannot contribute entries to host firewall rules even if stale rows claim firewall mode.
- Firewall-mode entries are included only after the application's defined approval/sync path marks them eligible.
- Active manual blocks marked firewall-enforced appear in the corresponding firewall preview and apply; soft blocks do not.
- Disabling a source or changing it away from firewall mode removes its entries from the next firewall plan and apply.
- Firewall plan output clearly shows additions and removals caused by blocklist enforcement-mode changes.

## Additional manual trace: adaptive firewall escalation

CaptchaChallengeService.CountProtectedHitWhileChallengedAsync writes FirewallBlockedUntilUtc and CurrentState=firewall_blocked, but does not create a host firewall subject/blocklist desired row or queue a firewall plan. FirewallApplyService does not read SecuritySubjectStates. An adaptive firewall block consequently remains HTTP decision state, even if a later ordinary firewall apply runs. Include this producer when unifying desired state and per-host application reporting.
