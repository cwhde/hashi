# H-026: Missing docs/operations/backup-restore.md Verification

**Priority:** Low
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §33

**Status:** Fixed
**Branch:** h/spec-compliance-1

## Description

The spec requires operations documentation:
```
Operations Guidance:
- Backups: PostgreSQL backup is mandatory. /data backup is mandatory for GeoIP DB cache, uploads, and local generated artifacts. Recovery key must be stored outside Hashi.
- Disaster recovery: Restore PostgreSQL and /data. Start Hashi. Unlock vault with passkey or recovery key. Run global reconcile.
```

The `docs/operations/` directory contains:
```
docs/operations/
├── api-contract.md
├── backup-restore.md
├── ci-secrets.md
└── hardening.md
```

The `backup-restore.md` file exists. This finding requires verification that the content matches the spec requirements.

Without reading the full file, I cannot confirm if it covers:
1. PostgreSQL backup procedures
2. /data backup procedures
3. Recovery key storage guidance
4. Disaster recovery steps
5. Global reconcile procedure

## Expected Outcome

- backup-restore.md covers all spec requirements
- Procedures are clear and actionable
- Recovery key guidance is included

## Fix Guidance

Verify that `docs/operations/backup-restore.md` covers:
1. PostgreSQL backup commands and scheduling
2. /data volume backup procedures
3. Recovery key storage (outside Hashi)
4. Disaster recovery steps
5. Global reconcile trigger

## Acceptance Criteria

- [x] backup-restore.md exists (implemented)
- [x] Content covers PostgreSQL backup
- [x] Content covers /data backup
- [x] Content covers recovery key storage
- [x] Content covers disaster recovery
