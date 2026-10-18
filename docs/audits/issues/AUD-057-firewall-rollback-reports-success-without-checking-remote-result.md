# AUD-057: Firewall rollback reports success without checking the remote result

Priority: P1

Category: Recovery reliability / firewall rollback

Spec reference: Base §14.5; extended addendum §15.

## Finding

RollbackInternalAsync ignores both the rollback-file write result and the remote command result. RollbackAsync then clears LastAppliedScriptHash/At and returns Succeeded=true even when writing or executing the rollback failed. Automatic apply failure paths also invoke the same unchecked helper. An operator can be told recovery succeeded while the failed firewall state remains active.

## Evidence

- src/Hashi.Infrastructure/Platform/FirewallApplyService.cs:504-517 unconditionally returns success after helper invocation.
- Lines606-618 discard WriteScriptAsync and RunCommandAsync outcomes.

## Expected outcome

Propagate rollback write/execution/verification failures and retain a truthful partial or unknown live-state record.

## Acceptance criteria

- Failed rollback writes or commands return failure with actionable errors.
- Host applied-state fields are changed only after verified recovery.
- Automatic rollback failure is retained in sync/audit results.

Verification: confirmed by manual source tracing; no live firewall/provider change was performed.
