# AUD-134: Live firewall sets and chains are flushed before replacements are ready

Priority: P2

Category: Security / firewall update atomicity

Status: open; confirmed by manual source trace.

## Finding

The script flushes the active trusted/blocked ipsets and chains and then repopulates them through sequential commands. Traffic continues traversing those names during the operation. Failures or interruption can leave empty/partially populated active policy, and concurrent executions can interleave. This violates the promised atomic set refresh even after CIDR/table bugs are repaired. Hash equality of the script file is not verification of installed live rules.

## Evidence

src/Hashi.Core/Firewall/FirewallScriptRenderer.cs:219-289; main specification14.3/14.5. AUD-034 covers cross-path locking; AUD-007/052 cover specific errors that can interrupt this sequence.

## Expected outcome and acceptance criteria

Build validated temporary sets and a complete rule transaction, atomically swap/restore live state under a host lock, and retain the previous state on failure. Interrupted updates must leave either the old or new complete enforcement policy.
