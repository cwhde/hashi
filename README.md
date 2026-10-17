# Hashi V2

Single-user homelab edge orchestration platform — public and private routing, DNS, Traefik, firewall forwarding, monitoring, SSO, and more from one control plane.

## Stack

- **Backend**: ASP.NET Core 10, PostgreSQL 18, EF Core
- **Frontend**: SvelteKit 5, shadcn-svelte, Tailwind CSS v4
- **Agent**: Hashi Pulse (Go)
- **Deploy**: Docker Compose

## Development

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
- [Current triage](docs/audits/TRIAGE-2026-10-17.md) — verified issue status, priorities, and next steps.

## License

MIT — see [LICENSE](LICENSE).

V1 source is archived under [`hashi.old/`](hashi.old/) for reference only.
