# H-069: Sync Plan Lacks Post-Render Validation

**Priority:** High
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §25

## Description

`SyncOrchestratorService.PlanGlobalAsync()` interleaves subsystem-specific plan calls but has no explicit normalization step or post-render validation step. Traefik rendering happens during Plan but validation (`TraefikConfigValidator`) is only exposed via a separate endpoint, not integrated into the Plan flow. The spec requires Plan to validate generated configs before returning the preview.

## Evidence

- `PlanGlobalAsync` calls subsystem plan/render methods but never calls `TraefikConfigValidator.ValidateRender` or equivalent DNS/firewall validation
- Validation endpoints exist but are separate from the plan pipeline

## Expected Outcome

Plan should render all configs in memory, validate all rendered configs, and include validation results in the plan response. No Apply should be possible with an invalid plan.

## Fix Guidance

1. After all subsystem plans/rendering, run validation (`TraefikConfigValidator.ValidateRender`, firewall script validation, DNS plan structural checks).
2. Include validation results in the plan response.
3. Reject Apply if any validation errors exist.

## Acceptance Criteria

- [ ] Plan returns validation errors when rendered configs are invalid
- [ ] Apply is rejected when plan has validation errors
- [ ] Validation covers Traefik YAML, firewall script, and DNS record integrity
