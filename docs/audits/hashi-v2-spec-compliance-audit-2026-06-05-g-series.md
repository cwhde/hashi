# Hashi v2 spec compliance audit - G-series - 2026-06-05

Scope: reread the full Hashi v2 implementation spec and addendum, reviewed prior audit issue formatting only, and audited the current implementation for new spec gaps not already captured by the existing A-F issue series.

Current checkout: `351e96d0bd7363790485af4b802147c9976c5ece`.

## Verification

- Read `docs/implementation-spec/hashi-v2-implementation-spec.md`.
- Read `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md`.
- Reviewed existing audit issues under `docs/audits/issues` for format and tone only.
- Inspected setup UI, security dashboard contracts/UI, security ingestion, background job registration, blocklist source management, connection target resolution, AdGuard target handling, Traefik/firewall setup, SSH connection handling, and persistence entities.
- Did not run the full application test suite because this audit only adds documentation issues.

## New Issues

- `issues/G01-blocklists-are-not-available-during-setup.md` - Recommended blocklists are not available during setup.
- `issues/G02-addendum-background-jobs-are-registered-but-not-run.md` - Addendum background jobs are registered but not run.
- `issues/G03-security-dashboard-omits-required-addendum-widgets.md` - Security dashboard omits required addendum widgets.
- `issues/G04-security-event-ingest-drops-request-correlation-fields.md` - Security event ingest drops request correlation fields.
- `issues/G05-agent-bound-targets-are-not-supported-for-ssh-backed-connections.md` - Agent-bound targets are not supported for SSH-backed connections.
- `issues/G06-blocklist-refresh-loses-entry-lifecycle-and-fetch-metadata.md` - Blocklist refresh loses entry lifecycle and fetch metadata.

## Notes

- I did not reopen old issue files as fix verification targets; they were used only to preserve the existing audit issue style.
- I intentionally did not create issues for areas where the implementation appears to be a reasonable equivalent or a stricter implementation than the spec.
