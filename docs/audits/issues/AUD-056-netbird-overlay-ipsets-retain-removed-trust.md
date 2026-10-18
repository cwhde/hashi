# AUD-056: NetBird overlay sets retain removed trusted ranges

Priority: P2

Category: Security / stale firewall desired state

Spec reference: Base §§14.3,14.4; base rule8 idempotence.

## Finding

Each apply creates hashi_netbird/6 with -exist and adds the configured CIDRs, but never flushes or atomically replaces either set. The script flushes trusted/blocked sets only. Removing an overlay range from host configuration leaves that range in the actual NetBird allow set indefinitely. Disabling then reenabling NetBird also reuses the stale set.

## Evidence

- src/Hashi.Core/Firewall/FirewallScriptRenderer.cs:220-229 creates all sets but flushes only trusted/blocked.
- Lines369-382 only add configured NetBird CIDRs and trust matching traffic.

## Expected outcome

Reconcile the full NetBird set membership using a staged set and atomic swap, removing ranges no longer desired.

## Acceptance criteria

- Removing an overlay CIDR removes its live trust after apply.
- Disable/reenable does not restore previously removed ranges.
- Refresh does not create an empty trust window.

Verification: confirmed by manual source tracing; no live firewall/provider change was performed.
