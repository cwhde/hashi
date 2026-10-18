# AUD-075: Traefik validation invokes a nonexistent check command

Priority: P1

Category: Traefik / installation and apply

Status: open; confirmed by manual source tracing.

## Finding

Every non-noop apply stages configuration and executes traefik check --configFile. Official Traefik CLI registers healthcheck and version, with no check subcommand. A standard supported Traefik binary therefore rejects this command, and ApplyAsync returns failure before installing live config. The current tests using mocked SSH cannot establish CLI compatibility.

## Evidence

Paths below are relative to src/Hashi.Infrastructure/:

Platform/TraefikSyncService.cs:148-153,490-509,577-587

## Expected outcome

Use a validation strategy supported by the explicitly pinned Traefik version and validate both static and dynamic configuration without production side effects.

## Acceptance criteria

A stock pinned supported Traefik binary accepts the chosen validation flow; valid first deployment passes and invalid dynamic config is rejected.

No live service configuration or firewall was changed to establish this finding.

Primary reference: [Upstream contract/source](https://github.com/traefik/traefik/blob/master/cmd/traefik/traefik.go).
