# AUD-166: Disabling all explicit script targets falls back to every host

Priority: P1

Category: Safety / privileged script targeting

Status: open; confirmed by manual source trace.

## Finding and evidence

GetEnabledTargetConnectionIdsAsync queries only enabled ScriptTargets, then falls back to all enabled firewall hosts whenever that result is empty. This conflates an absent target configuration with a present explicit configuration whose targets are all disabled. Disabling every target can unexpectedly execute the privileged script on every host. ToResponseAsync uses total configured target count for its default display, so the response can present disabled explicit targets while execution chooses all hosts. Default-all when no target rows exist is required by the specification and is not itself a defect.

Evidence: `src/Hashi.Infrastructure/Platform/ScriptExecutionService.cs:699-714,744-751`. AUD-148 separately covers the misleading singular target selector in the UI.

## Expected outcome and acceptance criteria

Distinguish default target mode from an explicit empty/disabled selection. An explicit configuration with no enabled targets must execute nowhere or reject clearly, never broaden to all hosts. Preview, manual runs and scheduled deployment must use the same effective target set.
