# 04 — Resources and Traefik

## 1. Resource model

Common fields: name, slug (unique, DNS-safe), kind (`http` | `https` | `h2c` | `tcp` | `udp`), domain mode (root /
subdomain / full custom), enabled, origin, owning feature (for generated resources), detected firewall host, explicit
routing override, dashboard visible, monitoring visible, AdGuard rewrite enabled, extra middlewares (from the user
middleware file), security profile (06-edge-security.md), sync state + last applied hash.

Kind semantics: `http` = upstream speaks HTTP; `https` = upstream speaks TLS (with per-resource `insecureSkipVerify`
and `serverName` options); `h2c` = upstream speaks cleartext HTTP/2. All three are served publicly on `websecure` (443)
with Hashi-managed TLS and an HTTP→HTTPS redirect on `web` (80).

### 1.1 HTTP-family targets

- **Simple target:** scheme, host, port.
- **Advanced routes** (when enabled), each with: enabled, target list (load-balanced; weights optional), path match type
  (prefix / exact / regex), path value, priority, rewrite, route-specific middleware list.
- **Rewrite modes:** replace prefix, rewrite exact path, regex replacement with capture groups, strip prefix.
- TLS options (min version, client-auth off by default), forward-auth policy (06-edge-security.md §2), rule list.

Route identity is the route's ULID, never its priority.
Every middleware a route references is emitted in the same render.

### 1.2 Stream targets (TCP/UDP)

Protocol, public port, target host, target port, PROXY protocol option (TCP), monitoring protocol hint.
TCP with TLS may use `HostSNI`; raw TCP and all UDP are port-based, one entry point per public port.
TCP/UDP cannot use HTTP forward auth; abuse handling for streams is firewall-level (06-edge-security.md §8).

### 1.3 Resource rules

Ordered by priority (higher first). Actions: allow (bypass auth), deny, require SSO, require challenge, soft block,
firewall block. Match types: IP, CIDR, path, country, region, ASN. Country/region/ASN rules require a GeoIP database;
without one they are invalid and cannot be enabled (clear validation error). Full rule engine: 06-edge-security.md §3.

### 1.4 System resources

Origin `system`, created by owning features: Hashi admin (setup), CAPTCHA public challenge (when Cap enabled), optionally
the Traefik dashboard as a protected resource. Cannot be deleted while required; editable only via the owning workflow.

### 1.5 Validation

Hostnames are validated as FQDNs; target hosts as IP or hostname; ports 1–65535; regexes compiled (RE2) at save; paths
must start with `/`. Values are never string-interpolated into config — renderers build Go structs and marshal them. IPv6 upstreams are bracketed in URLs.

An invalid resource marks itself `invalid` with a reason and is excluded from rendering; it never stops other resources
from rendering.

## 2. Traefik installation

Supported OS: Debian, Ubuntu, Alpine. Install flow (over SSH, each step a plan item):

1. Detect existing Traefik (binary, service, container) and package manager.
2. Install Traefik if absent: official release binary for the pinned version + checksum verification (distro packages lag).
3. Create Hashi directories: `/etc/hashi/traefik`, `/etc/hashi/traefik/dynamic`, `/var/log/hashi/traefik`, `/var/lib/hashi/traefik` (ACME storage, mode 0600).
4. Discover existing configs in common paths (`/etc/traefik`, `/opt/traefik`, compose volumes); show files and summary.
5. If found: back up to `/var/lib/hashi/traefik/backup/<timestamp>/` and require explicit confirmation before Hashi takes ownership.
6. Write a systemd unit (Debian/Ubuntu) or OpenRC script (Alpine) running Traefik with Hashi's static config.

Pinned Traefik version lives in one place (`deploy/versions.env`) and is upgraded deliberately (ADR per major).

## 3. Static configuration (fully Hashi-managed)

- Entry points: `web` :80/tcp (redirect to websecure), `websecure` :443/tcp (+ HTTP/3 on 443/udp optional), and one
  entry point per public TCP/UDP resource port, named `tcp-<port>` / `udp-<port>`.
- Providers: file provider, directory `/etc/hashi/traefik/dynamic`, `watch: true`.
- Certificate resolvers: `hashi-gts` (Google Trust Services + EAB) and optionally `hashi-le`; DNS-01 via `httpreq` → Hashi (03-dns.md §5.3).
- Access log: JSON, `/var/log/hashi/traefik/access.log`, fields limited to what ingestion needs, `Authorization`/`Cookie`/`Set-Cookie` and
  any configured header dropped; rotation via logrotate (Hashi-installed config).
- General log: JSON, `/var/log/hashi/traefik/traefik.log`.
- `ping` enabled on an internal entry point; API/dashboard enabled on `127.0.0.1` only (used for Verify), never public unless exposed as a protected system resource.
- Plugins (Coraza WAF etc.) pinned by version; the pinned version must exist in the Traefik plugin catalog (checked by `make contract-capture PROVIDER=traefik-plugins`).
- `serversTransport` defaults; `forwardedHeaders.trustedIPs` = firewall-host internal IPs so client IPs survive DNAT.

Static changes require a Traefik restart and are high-risk.

## 4. Dynamic configuration files

| File | Content | Editable |
| --- | --- | --- |
| `00-hashi-core.yml` | Generated default middlewares | no |
| `10-hashi-http.yml` | HTTP/H2C/HTTPS routers and services | no |
| `20-hashi-streams.yml` | TCP/UDP routers and services | no |
| `30-user-middlewares.yml` | User-defined extra middlewares | **yes**, via UI editor |
| `40-hashi-security.yml` | WAF / rate-limit / security middlewares | no |
| `90-hashi-health.yml` | Health routers for monitoring | no |

The user middleware editor (CodeMirror YAML) parses strictly, extracts middleware names, and offers valid names as
toggles on resources. Parse/validation errors keep the previously applied file and show the error inline.

### 4.1 Default middlewares (per resource chain)

HTTP→HTTPS redirect, security headers, compression, Hashi forward auth (per policy), WAF (per mode), baseline rate limit,
optional error pages. The chain is generated per resource because SSO, WAF and adaptive modes differ per resource.

## 5. Traefik connection

Fields: name, SSH target (static host/IP or Pulse agent, 08-pulse.md §5), SSH port, username, password or private key
(+ passphrase), pinned host key, internal Traefik IP (used for DNAT and AdGuard), config paths (detected defaults,
overridable).

Validation: SSH login, host-key pin, OS + package manager detection, existing Traefik detection, config discovery,
write-permission check via a uniquely named temp file in a Hashi-owned path.

## 6. Apply and verify

Per Traefik host:

1. Render all files; compute hashes; compare with the host's actual file hashes (read over SFTP), not just DB state.
2. Write changed files atomically (unique temp + rename) — dynamic files trigger Traefik's watcher; unchanged files are never touched.
3. If static config changed: write it, restart the service, wait for `ping` (timeout 30 s). On failure restore the previous
   static file, restart again, mark the plan failed.
4. **Verify** through the local Traefik API over SSH (`curl -s 127.0.0.1:<api>/api/rawdata`): every expected router/service/
   middleware exists and none reports an error status. There is no `traefik check` command; do not invent one.
5. Store per-file `last_applied_hash`.

## 7. Sync rules

- Every Traefik connection receives every enabled resource; DNS decides which instance receives traffic, so routing can
  move by DNS without re-rendering resources.
- A new public port requires confirmation (high-risk; also opens the firewall, 05-firewall.md).
- Removing the last resource on a port removes its entry point and firewall opening after confirmation.
- Disabled resources stay in the DB but are absent from generated files.
- Reused ports pending removal are cancelled, not deleted, when a resource claims them again.

## 8. API

`/api/resources` CRUD + `POST /api/resources/{id}/enable|disable`, `GET /api/resources/{id}/effective` (rendered config
preview), `/api/traefik/connections/*` (CRUD, validate, install, discover, adopt), `GET|PUT /api/traefik/user-middlewares`,
`POST /api/traefik/plan`.

## 9. Tests that must exist

- Golden-file render tests for every resource kind, route option, rewrite mode and middleware combination.
- Rendered YAML parsed back and validated against the Traefik dynamic-config JSON schema (pinned copy in `testdata/`).
- Identical render → zero file writes (invariant 8).
- Integration: real Traefik binary (pinned version) in a test container loads the rendered files and its API reports no errors;
  a request through it reaches a stub upstream (HTTP, H2C, HTTPS upstream, TCP, UDP).
- Static-config restart failure → automatic restore.
