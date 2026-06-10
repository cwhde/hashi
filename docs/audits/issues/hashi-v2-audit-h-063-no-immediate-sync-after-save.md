# H-063: No Immediate Sync After Save — Changes Wait Up to an Hour to Propagate

**Priority:** High
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §25 (Immediate sync after save; Default periodic sync: every hour, immediate sync after save, manual sync button per subsystem and global)

**Status:** Fixed
**Branch:** h/security-2

## Description

Settings endpoints, resource CRUD, connection CRUD, and DNS record changes all save data but never trigger a sync. The SyncOrchestratorHostedService only runs on its hourly timer. There is no mechanism to trigger an immediate Plan+Apply after a configuration change. Users must wait up to an hour for changes to propagate.

## Evidence

No TriggerSync, QueueSync, or SyncAfterSave pattern exists anywhere. Settings endpoints (e.g., PUT /api/settings/general) save and audit but don't call SyncOrchestratorService. Resource/connection CRUD endpoints similarly don't trigger sync.

## Expected Outcome

After a configuration save that affects sync-eligible resources, an immediate Plan+Apply cycle should be triggered or queued.

## Fix Guidance

Add a QueueSyncRun() method to SyncOrchestratorService that signals the hosted service to run immediately. Call it from settings, resource, connection, and DNS record endpoints after successful saves.

## Acceptance Criteria

- [ ] Changing a resource, DNS record, or setting triggers a sync within seconds
- [ ] Manual sync button still works as before
- [ ] Immediate sync doesn't bypass destructive-change confirmation
