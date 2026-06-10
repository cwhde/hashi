# H-094: Firewall Apply No Skip If Unchanged

**Priority:** Medium
**Conflict Type:** wrong_implementation
**Spec Reference:** §3 Rule 9 (Hashi avoids hot reloads caused by rewriting identical files); §25 (Restart/reload services only if needed)

**Status:** In Progress
**Branch:** h/sync-engine
**Branch:** 

## Description

`Firewall ApplyForHostAsync` always applies the script regardless of whether it changed. Traefik has change-detection via hash comparison, but Firewall does not short-circuit on unchanged config. This means unnecessary SSH connections, script deployments, and service reloads.

## Evidence

- `ApplyForHostAsync` always writes and reloads, even if the script content is identical
- Traefik apply uses hash comparison to skip unchanged configs
- Firewall apply has no equivalent hash check

## Expected Outcome

No-op Apply when firewall config hash matches deployed hash. Unnecessary SSH connections should be avoided. Reconcile should report "no changes" when script is unchanged.

## Fix Guidance

1. Before applying, compare the rendered script hash against `host.LastAppliedScriptHash`.
2. Skip the apply if the hash matches.
3. Report "no changes" in the reconcile result when skipping.

## Acceptance Criteria

- [ ] No-op Apply when firewall config hash matches deployed hash
- [ ] Unnecessary SSH connections are avoided
- [ ] Reconcile reports "no changes" when script is unchanged
