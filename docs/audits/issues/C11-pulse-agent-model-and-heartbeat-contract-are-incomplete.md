# C11 - Pulse agent model and heartbeat contract are incomplete

Priority: High

Spec conflicts: section 17 requires Pulse agents to track install type, allowed scopes, heartbeat interval, last private IP candidates, selected IP, version, and status. Heartbeat payloads must include private IPv4/IPv6 candidates, optional selected interface, timestamp, and optional Docker metadata.

## Problem

The Pulse implementation stores only a reduced agent model and accepts only a reduced heartbeat payload. The server stores one `LastPrivateIp`, not private IP candidates or selected IP/interface. There are no install type, allowed scope, or heartbeat interval fields on the agent model.

The Go agent sends private IPv4 candidates only. It does not report IPv6 candidates, selected interface, timestamp, or Docker metadata. The Linux installer also writes the token directly into the systemd unit environment instead of installing a root-owned config file as described by the spec.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:95-103` requires a Linux install with root-owned config and a systemd timer or cron fallback.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:900-915` lists the required Pulse agent model fields.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:926-940` lists the required heartbeat payload fields.
- `src/Hashi.Infrastructure/Persistence/Entities/PlatformEntities.cs:146-165` stores name, token hash, last seen, one public/private IP, hostname, version, DNS pending, and status only.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:439-449` exposes a reduced agent response and heartbeat request.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:665-669` accepts only token, version, hostname, and private IPv4 candidates for authenticated heartbeat.
- `src/Hashi.Infrastructure/Platform/PulseAgentService.cs:70-79` stores only the first private IPv4 candidate as `LastPrivateIp`.
- `agents/pulse/cmd/hashi-pulse/main.go:31-36` defines the agent heartbeat payload with only `privateIpv4Candidates`.
- `agents/pulse/cmd/hashi-pulse/main.go:102-107` sends only that reduced payload.
- `agents/pulse/install.sh:61-66` writes `HASHI_PULSE_TOKEN` directly into the systemd unit environment.

## Expected outcome

Pulse should store and report the full agent model needed for dynamic endpoint discovery, and heartbeats should carry the full network candidate set and metadata required by the spec.

## Fix guidance

Extend the Pulse agent schema/contracts with install type, allowed scopes, heartbeat interval, private IP candidates, selected IP/interface, and optional Docker metadata. Update the Go agent to collect IPv4 and IPv6 private candidates, interface names, timestamp, and Docker metadata when running in Docker. Change Linux install to write a root-owned config file and run via systemd timer or service plus timer according to the spec.

## Acceptance criteria

- Pulse agent records persist install type, allowed scopes, heartbeat interval, private IP candidates, selected IP/interface, version, and status.
- Heartbeat contracts and the Go agent include private IPv4/IPv6 candidates, selected interface, timestamp, and optional Docker metadata.
- The server validates heartbeat timestamps within an acceptable skew.
- Linux install stores agent config in a root-owned file rather than only in the unit environment.
- Tests cover heartbeat persistence, IPv6 candidates, selected IP/interface behavior, and stale timestamp rejection.
