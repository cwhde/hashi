# AUD-126: Resource soft and firewall block actions only deny the current request

Priority: P1

Category: Wrong implementation / resource rule actions

Status: open; confirmed by manual source trace.

## Finding

EvaluateResourceRulesAsync maps soft_block and firewall_block to Deny results without creating the state, expiry, offense event or desired firewall entry those actions imply. bypass_blocking returns null after prior global/manual/automatic block checks have already denied the request, so it cannot implement its advertised bypass behavior. These action names are accepted and normalized by the contracts/service, making them configured features with different runtime behavior.

## Evidence

src/Hashi.Infrastructure/Platform/SecurityDecisionService.cs:320-368 and preceding block decision ordering; SecurityResourceRuleActionNames:905 onward. AUD-033 covers missing firewall projection for actual existing adaptive/manual state; AUD-087 covers manual-allow flags.

## Expected outcome and acceptance criteria

Define and implement the persisted/scope-aware semantics of every accepted rule action, including deliberate precedence for bypass rules. Do not report a persistent firewall block unless it is represented in desired state and can be applied/expired correctly. Exercise subsequent requests and host plans, not only the immediate HTTP result.
