# AUD-085: Firewall rescue timer skips a freshly written non-executable rollback script

Priority: P1

Category: Firewall / lockout recovery

Status: open; confirmed by manual source tracing.

## Finding

The renderer rescue timer invokes rollback only if its file passes an executable-file check. FirewallApplyService writes rollback.sh through the SFTP atomic writer. That writer creates/uploads/moves files but never sets an executable mode; the only explicit rollback chmod is in the synchronous rollback helper. On a fresh host with a normal non-executable uploaded file, the timed rescue condition fails, so loss of SSH connectivity can leave the restrictive firewall active without the promised automatic rollback.

## Evidence

src/Hashi.Core/Firewall/FirewallScriptRenderer.cs: timer rollback executable check; src/Hashi.Infrastructure/Platform/FirewallApplyService.cs: rollback script write and RollbackInternalAsync; src/Hashi.Infrastructure/Ssh/SshRemoteExecutor.cs:135-206

## Expected outcome

Prepare and verify a root-owned executable rescue script before enabling restrictive rules, or invoke the script through an explicit interpreter; keep rescue armed through independent connectivity verification.

## Acceptance criteria

On a fresh host, intentional loss of the control connection triggers verified timed recovery without any later SSH command or prior rollback-file permissions.

No live credentials, host scripts or services were changed for this review.
