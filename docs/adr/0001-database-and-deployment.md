# Hashi V2 — PostgreSQL desired state and Docker Compose deployment

## Status

Accepted

## Context

Hashi V2 replaces file-based V1 configuration with PostgreSQL as the single source of desired state. The product must run unattended background sync after restart without an active browser session, while keeping admin secrets protected behind passkey/recovery vault semantics.

## Decision

1. Use **PostgreSQL 18** as the primary database via **EF Core 10 + Npgsql**.
2. Deploy as a **single Hashi container** (ASP.NET Core API + built SvelteKit static assets) plus a **PostgreSQL container** via Docker Compose.
3. Store provider-specific settings in **jsonb** columns where needed, but keep operational fields relational.
4. Use **envelope encryption** with a service-sync vault key (Docker secret) for background jobs and passkey/recovery-wrapped keys for admin operations (Phase 2).

## Consequences

- Migrations ship with model changes in the same commit.
- Integration tests use PostgreSQL test containers.
- Raw SQL is reserved for partitioning, advisory locks, and high-volume security/monitor inserts.
