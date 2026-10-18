# SEC-002: Pulse release builds use an unsupported Go toolchain

Priority: P1

Category: Security / release toolchain

Status: open; manually traced build configuration and independently built/scanned the Linux release source with Go1.22.12.

## Finding

Pulse Docker builds pin `golang:1.22-alpine`. Native artifact workflows read `go 1.22` from go.mod. Go1.22 is outside the supported release window and lacks later standard-library HTTP/TLS/X.509 security fixes. The agent performs authenticated HTTP(S) heartbeats using the standard client, so its compiler/runtime security updates matter independently of having no third-party modules.

## Evidence

- `agents/pulse/Dockerfile:3`, `agents/pulse/go.mod`, `.gitea/workflows/docker-build-pulse.yml` setup-go configuration.
- Independent CGO-disabled Linux/amd64 build of the unchanged agent source with the last1.22 patch, Go1.22.12, succeeded. `govulncheck -mode binary -json` produced63 distinct advisory identifiers /884 finding records. These include package/symbol enumeration, impossible newer-API/platform cases and server code linked by net/http; **63 is not a count of exploitable Pulse vulnerabilities**.
- Actual HTTP client/TLS/certificate use establishes an update requirement; advisory-specific reachability must be interpreted separately. The modern local1.26.8 scanner build is not the release build and was not used to clear Go1.22.
- [Official Go release/support policy](https://go.dev/doc/devel/release#policy) supports a major release until two newer major releases exist. It lists subsequent HTTP/TLS/X.509 security patch releases.

## Expected outcome and acceptance criteria

Use a supported patched Go toolchain consistently for native and image releases, make the selected compiler version explicit, scan the actual Linux architectures/artifacts and retain advisory applicability decisions. Updating go.mod's language minimum alone does not prove the compiler image changed. Rebuild distributed binaries/images and confirm heartbeat interoperability; no production agent was changed during this audit.
