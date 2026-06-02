# C09 - Manual DNS audit events store subjects in the outcome field

Priority: Medium

Spec conflicts: Hashi requires audited privileged actions and useful audit history for DNS changes.

## Problem

Manual DNS CRUD writes audit events with positional arguments in the wrong order. The third positional argument to `AuditService.WriteAsync` is `outcome`, but the DNS record service passes `"dns_record"` there and passes the record id as the fourth positional argument, which becomes `subjectType`.

As a result, successful manual DNS changes are stored with outcome `dns_record`, subject type equal to a GUID, and no subject id. The audit event exists, but it is not queryable or readable as a normal subject-based DNS audit trail.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:15` requires audited privileged actions.
- `src/Hashi.Infrastructure/Services/CoreServices.cs:72-79` defines `AuditService.WriteAsync(category, action, outcome, subjectType, subjectId, ...)`.
- `src/Hashi.Infrastructure/Dns/DnsRecordService.cs:65` writes manual create audit events with `"dns_record"` as the outcome argument.
- `src/Hashi.Infrastructure/Dns/DnsRecordService.cs:122` repeats the same issue for manual update.
- `src/Hashi.Infrastructure/Dns/DnsRecordService.cs:140` repeats the same issue for manual delete.
- `rg -n "audit.WriteAsync" src/Hashi.Infrastructure` shows most other services use named `subjectType:` and `subjectId:` arguments, so this is inconsistent with local style.

## Expected outcome

Manual DNS create/update/delete audit events should have `Outcome = "success"`, `SubjectType = "dns_record"`, and `SubjectId = <record id>`.

## Fix guidance

Change these calls to use named arguments. Add unit tests that inspect the resulting audit rows and verify category, action, outcome, subject type, and subject id.

## Acceptance criteria

- Manual DNS create/update/delete audit rows use the correct outcome, subject type, and subject id.
- Tests fail if `dns_record` appears in the audit outcome column for manual DNS actions.
- Existing audit rows from other services remain unchanged.
