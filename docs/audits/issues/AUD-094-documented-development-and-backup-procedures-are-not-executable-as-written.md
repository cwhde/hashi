# AUD-094: Documented development and backup procedures are not executable as written

Priority: P2

Category: Documentation / installation

Status: open; confirmed by manual source tracing.

## Finding

README labels the development Compose command Full stack, but that file defines only PostgreSQL and exposes no database port for the native backend. Backup instructions assume pg_dump/pg_restore and directories on the application host without identifying the PostgreSQL container, credentials, backup directory creation or deployment key recovery. The Hashi runtime image does not include PostgreSQL tools. API endpoint and CI service documentation is also stale. README and all image labels claim MIT although LICENSE contains the BSD3-clause terms.

Documentation normalization in this audit corrected the root README license label and marked its quick commands as historical/unverified. Image metadata and the underlying development/restore procedure remain open; this small label correction is not a completed installation/backup repair.

## Evidence

README.md:12-24,32-36; deploy/compose/docker-compose.dev.yml; docs/operations/backup-restore.md:7-25; docs/operations/api-contract.md; docs/operations/hardening.md:13; LICENSE; .gitea/workflows/docker-build.yml:48 and other metadata

Preserves historical H026 backup verification requirements; no restore was attempted against user data.

## Expected outcome

Provide one exact supported development and production install procedure plus container-aware backup/restore commands, required key storage and recovery steps. Align license labels with actual license without changing license terms.

## Acceptance criteria

Follow the guide on a clean machine and recover a backup into an empty deployment; all command prerequisites, topology, keys and expected results are explicit.
