# B02 - Pulse agent binary source is missing

Priority: High

Spec conflicts: sections 4, 17, 27, and Phase 12. Hashi Pulse must be a working Go dynamic endpoint agent with install and Docker paths.

## Problem

The repository contains Pulse packaging scaffolding, but the Go command the Makefile, installer, Dockerfile, and CI job build does not exist. There is no `agents/pulse/cmd/hashi-pulse` source tree, so the standalone agent cannot be built or installed.

The backend Pulse API and UI can create tokens and render install commands, but there is no actual agent implementation to run on endpoint hosts.

## Evidence

- `agents/pulse/Makefile:4` sets `CMD := ./cmd/hashi-pulse`.
- `agents/pulse/Makefile:8` runs `go build ... $(CMD)`.
- `agents/pulse/Dockerfile:4-5` copies `cmd/` and builds `./cmd/hashi-pulse`.
- `agents/pulse/install.sh:44-47` also builds `./cmd/hashi-pulse`.
- `.gitea/workflows/ci.yml:145` defines a `pulse-agent` job.
- `.gitea/workflows/ci.yml:164-166` runs `make build` and `docker build` for the agent.
- `rg --files agents/pulse` currently lists only `README.md`, `Makefile`, `install.sh`, `go.mod`, and `Dockerfile`; there is no `cmd` directory.

## Expected outcome

The Pulse agent must have buildable Go source that authenticates to Hashi, sends heartbeat payloads, reports IP changes, rotates or accepts tokens safely, and works through the documented Linux and Docker install paths.

## Fix guidance

Implement `agents/pulse/cmd/hashi-pulse` and any internal packages needed for configuration, token handling, IP discovery, heartbeat posting, retries, and logging. Keep the binary small enough for the spec target. Add Go tests and make the CI job run on the current source.

## Acceptance criteria

- `go test ./...` passes under `agents/pulse`.
- `make build` creates a `hashi-pulse` binary.
- `docker build -t hashi-pulse:ci .` succeeds under `agents/pulse`.
- The installer can install and run the built agent.
- A local or integration test proves the agent can post an authenticated heartbeat to the Hashi API.
