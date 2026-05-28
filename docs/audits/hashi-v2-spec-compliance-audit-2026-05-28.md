# Hashi v2 spec compliance audit - 2026-05-28

Scope: full reread of `docs/implementation-spec/hashi-v2-implementation-spec.md` and review of backend, frontend, deployment, CI, and Pulse agent implementation against the spec.

## Verification

- `dotnet build Hashi.slnx -c Release /p:SkipFrontendBuild=true` passed.
- `dotnet test Hashi.slnx /p:SkipFrontendBuild=true` passed before this write-up.
- `dotnet format Hashi.slnx --verify-no-changes` passed before this write-up.
- `corepack pnpm install --frozen-lockfile` passed.
- `corepack pnpm run check` passed.
- `corepack pnpm run lint` passed.
- `corepack pnpm run test` passed, with a Vitest warning about `.svelte-kit/tsconfig.json` during the parallel run.
- `corepack pnpm run build` passed.
- Pulse Go tests and Docker build could not be run locally because `go` and `docker` are not installed in this environment.

## New Issues

- `B01-setup-connection-types-do-not-match-backend-required-types.md` - setup creates `traefik`/`firewall` connection types that backend workflows do not recognize.
- `B02-pulse-agent-binary-source-is-missing.md` - Pulse packaging exists, but the Go agent command source is absent.
- `B03-runtime-and-packages-are-pinned-to-dotnet-preview.md` - Docker images and Microsoft packages are still preview builds.
- `B04-public-and-admin-ports-are-hard-coded.md` - admin/dashboard/status ports are not runtime configurable.
- `B05-anonymous-access-log-ingest-can-create-firewall-blocks.md` - unauthenticated access-log ingestion can manufacture blocklist entries.
- `B06-service-sync-secrets-omit-unattended-runtime-credentials.md` - several runtime secrets cannot decrypt through service-sync mode.
- `B07-acme-eab-secret-can-be-stored-plaintext-during-setup.md` - ACME EAB credentials can be persisted as plaintext setup state.
- `B08-dns-generation-ignores-resource-domain-and-can-publish-internal-host-ip.md` - resource DNS ignores `Domain`, and host DNS can publish internal Traefik IPs.
- `B09-firewall-sync-omits-http-https-forwarding.md` - firewall plans omit standard web DNAT for HTTP/HTTPS resources.
- `B10-monitoring-provisioning-does-not-cover-required-sources-or-check-types.md` - monitoring auto-provisioning covers only a subset of required sources/types.
- `B11-settings-surface-and-widget-persistence-are-incomplete.md` - settings categories and persisted widget preferences are incomplete.
- `B12-edge-sso-still-uses-custom-oidc-token-handling.md` - Edge SSO still uses hand-rolled OIDC token handling.
- `B13-resource-geoip-rules-can-be-enabled-without-geoip-data.md` - resource GeoIP rules can be enabled without GeoIP data.
- `B14-manual-dns-record-crud-is-missing.md` - manual DNS CRUD is missing and generated records can displace manual same-name records.

## Notes

The implementation is generally compile-clean and test-clean. The remaining issues are mostly spec-compliance and safety boundaries rather than syntax or build failures.
