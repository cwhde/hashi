# AUD-091: Resource UI cannot edit core routing and access settings after creation

Priority: P1

Category: Missing features / resource administration

Status: open; confirmed by manual source tracing.

## Finding

The resource page exposes creation and a handful of per-row toggles/editors, but no general edit flow. After creation users cannot change name/domain mode/domain, target host/scheme/port/public port, forward-auth policy, WAF mode, dashboard/status membership or base path rewrite through the UI. The form offers SSO required but never offers OIDC provider selection. Existing bindings must be managed with manual API calls or resource recreation, which is especially problematic for protected system resources.

## Evidence

web/src/routes/(admin)/resources/+page.svelte:109-161,179-478,714-961

Base resource-management requirements and addendum UI workflows; backend correctness issues remain separate.

## Expected outcome

Implement complete resource editing using the existing patch contract, include provider/default selection, preserve system invariants and unsaved drafts, and show meaningful validation errors.

## Acceptance criteria

Every supported mutable resource setting can be changed, saved, reloaded and reflected in generated config from the UI; protected system fields remain correctly constrained.
