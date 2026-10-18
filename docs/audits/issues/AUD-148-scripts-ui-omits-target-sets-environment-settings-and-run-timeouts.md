# AUD-148: Scripts UI omits target sets environment settings and run timeouts

Priority: P1

Category: Missing feature / scripts UI and target safety

Status: open; confirmed by manual source trace.

## Finding

The script editor exposes a singular connection selector but not the implemented multi-target/default-all model, secret/plain environment variables or configurable execution timeout. Creation sends ConnectionId while omitting Targets; the backend's specified default-all behavior then selects all enabled firewall hosts rather than limiting execution to that displayed connection. Default-all itself is required by the specification; the UI fails to make the actual target set clear and configurable. Editing plain environment values also has no usable read model because ScriptEnvironmentVariableResponse omits Value.

## Evidence

web/src/routes/(admin)/scripts/+page.svelte create/edit forms and request payloads; src/Hashi.Infrastructure/Platform/ScriptExecutionService.cs target selection,650-695,780-784; Contracts/Api/PlatformContracts.cs script records. Main specification23 requires target hosts, environment and timeout configuration. AUD-010/080/110/111 cover separate execution defects.

## Expected outcome and acceptance criteria

Expose and accurately preview explicit/default target sets, provide supported environment secret/value editing without disclosing secrets, and configure timeouts. A selected single host must not be presented as the execution scope when the operation will affect all hosts.
