# AUD-133: Firewall script disarms its rescue timer before external connectivity verification

Priority: P1

Category: Security / firewall rescue

Status: open; confirmed by manual source trace.

## Finding

The generated script cancels its rollback timer after checking only that local iptables jump rules exist. External management connectivity is tested later by FirewallApplyService after the SSH execution returns. A rule set that is syntactically installed but blocks new SSH connections can therefore disarm its only independent rescue mechanism before the controller proves reconnection. Attempting controller-driven rollback over a now blocked management path cannot replace the timer. AUD-085 separately covers the non-executable rollback file that already breaks fresh-host rescue.

## Evidence

src/Hashi.Core/Firewall/FirewallScriptRenderer.cs:331-340; src/Hashi.Infrastructure/Platform/FirewallApplyService.cs:329-369. Main specification14.5 requires timed rollback and connectivity safety.

## Expected outcome and acceptance criteria

Keep an independent executable rescue armed through an external verified new connection and activation checks; acknowledge/disarm only from the successful controller transaction. A syntactically valid rule set that blocks reconnect must roll back autonomously.
