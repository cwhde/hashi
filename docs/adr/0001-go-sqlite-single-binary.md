# ADR 0001 — Go + SQLite single binary, SvelteKit SPA

Date: 2026-10-27 · Status: accepted

## Context

The V2 attempt used .NET 10 + EF Core + PostgreSQL 18 + SvelteKit + a Go agent: three languages, two containers, and a
database server to operate. It was never deployed; its first install blocker was the Postgres volume layout. Hashi is
single-user, homelab scale, and spends most of its time on SSH, HTTP APIs, file rendering and network logic.

## Decision

- Backend in Go (shared with the Pulse agent), net/http, spec-first OpenAPI with oapi-codegen, sqlc, goose.
- SQLite (pure-Go driver) in WAL mode as the only database, file under `/data`.
- SvelteKit 2 / Svelte 5 SPA (adapter-static) embedded into the binary.
- One container, one process, three listeners.

## Consequences

- Cross-compiling multi-arch images needs no emulation.
- No partitioning: telemetry uses minute buckets + batched retention; acceptable at expected volume (re-evaluate if
  security buckets exceed ~50 M rows).
- Single writer: write paths must be short; telemetry is aggregated in memory and flushed per minute.
- Backups are a file copy (`VACUUM INTO`).
