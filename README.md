# Hashi V2

Single-user homelab edge orchestration platform — public and private routing, DNS, Traefik, firewall forwarding, monitoring, SSO, and more from one control plane.

## Current project state

The [October18 manual review and ordered backlog](docs/audits/TRIAGE-2026-10-18.md) records confirmed installation blockers, incomplete workflows and security/correctness defects. The project is not yet ready for ordinary deployment. Start repairs from that backlog; historical “completed” audit summaries do not establish current acceptance.

## Stack

- **Backend**: ASP.NET Core 10, PostgreSQL 18, EF Core
- **Frontend**: SvelteKit, Svelte 5, shadcn-svelte, Tailwind CSS v4
- **Agent**: Hashi Pulse (Go)
- **Deploy**: Docker Compose

## Development

The quick commands below are historical. See [AUD-094](docs/audits/issues/AUD-094-documented-development-and-backup-procedures-are-not-executable-as-written.md) for the known development/backup documentation gaps before treating them as a verified startup procedure.

```bash
# Backend
dotnet build
dotnet test

# Frontend
cd web && pnpm install && pnpm dev

# Full stack
docker compose -f deploy/compose/docker-compose.dev.yml up
```

## Project documentation

- [Implementation specifications](docs/implementation-spec/README.md) — current requirements and addenda.
- [Audit index](docs/audits/README.md) — audit history and issue-series navigation.
- [Current triage](docs/audits/TRIAGE-2026-10-18.md) — manual review results, priorities, and next steps.
- [Specification matrix](docs/audits/full-review/SPEC-MATRIX.md) — implementation evidence and gaps for all specifications.
- [Historical issue reconciliation](docs/audits/full-review/HISTORICAL-RECONCILIATION.md) — original fixes, remaining defects and false positives.

## License

BSD-3-Clause — see [LICENSE](LICENSE).

V1 source is archived under [`hashi.old/`](hashi.old/) for reference only.
