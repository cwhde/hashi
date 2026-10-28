# 04 — Services and Traefik

## 1. Service model

A **service** is something Hashi exposes: HTTP-family kinds are L7 services (forward auth, rule set, WAF), TCP/UDP
kinds are L4 services (protected at the firewall). Formerly called "resource".

Common fields: name, slug (unique, DNS-safe), kind (`http` | `https` | `h2c` | `tcp` | `udp`), domain mode (root /
subdomain / full custom), enabled, origin, owning feature (for generated services), detected firewall host, explicit
routing override (entry host, 03-dns.md §5.1), dashboard visible, monitoring visible, AdGuard rewrite enabled, extra middlewares (from the user
middleware file), root rule set + default fallback and WAF modes (HTTP kinds, 06-edge-security.md §4, §9), sync state + last applied hash.

Kind semantics: `http` = upstream speaks HTTP; `https` = upstream speaks TLS (with per-service `insecureSkipVerify`
and `serverName` options); `h2c` = upstream speaks cleartext HTTP/2. All three are served publicly on `websecure` (443)
with Hashi-managed TLS and an HTTP→HTTPS redirect on `web` (80).

### 1.1 HTTP-family targets

- **Simple target:** scheme, address (an address set, [05a-ip-sets.md](05a-ip-sets.md) §5.2 — a single IP, a Pulse agent, a
  host or a DNS name; several resolved addresses are load-balanced), port.
- **Advanced routes** (when enabled), each with: enabled, target list (address sets, load-balanced; weights optional), path match type
  (prefix / exact / regex), path value, priority, rewrite, route-specific middleware list.
- **Rewrite modes:** replace prefix, rewrite exact path, regex replacement with capture groups, strip prefix.
- TLS options (min version, client-auth off by default). Access control is the service's rule set (06-edge-security.md §4),
  not a per-service policy mode.

Route identity is the route's ULID, never its priority.
Every middleware a route references is emitted in the same render.

### 1.2 Stream targets (TCP/UDP)

Protocol, public port, target address (address set), target port, PROXY protocol option (TCP), monitoring protocol hint.
TCP with TLS may use `HostSNI`; raw TCP and all UDP are port-based, one entry point per public port.
TCP/UDP cannot use HTTP forward auth; they are protected by the firewall's folded sets and kernel meters (05-firewall.md §3.2).

### 1.3 Access rules

Every HTTP-family service owns a root rule set ending in its default fallback; reusable rule sets attach to it at a
priority. Rule items reference IP sets (`all`, user, Pulse-backed, `geo` …). Model, flattening and evaluation:
06-edge-security.md §4. `geo` sets need a GeoIP database; without one the rule set shows a warning and the set resolves
to nothing.

### 1.4 System services

Origin `system`, created by owning features: Hashi admin (setup), CAPTCHA challenge service (when Cap enabled, 06 §4.5), optionally
the Traefik dashboard as a protected service. Cannot be deleted while required; editable only via the owning workflow.

### 1.5 Validation

Hostnames are validated as FQDNs; targets must be address sets (05a §5.2); ports 1–65535; regexes compiled (RE2) at save; paths
must start with `/`. Values are never string-interpolated into config — renderers build Go structs and marshal them. IPv6 upstreams are bracketed in URLs.

An invalid service marks itself `invalid` with a reason and is excluded from rendering; it never stops other services
from rendering.

## 2. Traefik installation

Supported OS: Debian, Ubuntu, Fedora (and other dnf + systemd distributions), Alpine. Hashi detects the family from
`/etc/os-release` and the tools present (package manager, systemd or OpenRC), never from a version number. Install flow (over SSH, each step a plan item):

1. Detect existing Traefik (binary, service, container) and package manager.
2. Install Traefik if absent: official release binary for the pinned version + checksum verification (distro packages lag).
3. Create Hashi directories: `/etc/hashi/traefik`, `/etc/hashi/traefik/dynamic`, `/var/log/hashi/traefik`, `/etc/hashi/traefik/certs` (mode 0700, certificate files from Hashi, 03-dns.md §5.3), `/var/lib/hashi/traefik`.
4. Discover existing configs in common paths (`/etc/traefik`, `/opt/traefik`, compose volumes); show files and summary.
5. If found: back up to `/var/lib/hashi/traefik/backup/<timestamp>/` and require explicit confirmation before Hashi takes ownership.
6. Write a systemd unit (Debian/Ubuntu/Fedora) or OpenRC script (Alpine) running Traefik with Hashi's static config.

### 2.1 Traefik updates (Hashi-managed)

Because Traefik is not installed through the OS package manager, Hashi owns its updates:

- A daily job checks Traefik's official GitHub releases (stable only) and shows "update available" per Traefik host,
  with the release notes link. Patch releases of the installed minor version can be set to auto-apply (setting, default off);
  minor/major upgrades are always manual.
- An upgrade is a high-risk sync plan per host: download the release archive, verify its checksum, keep the current binary
  as `traefik.previous`, swap the binary atomically, restart, wait for `ping`, verify routers through the Traefik API.
  Any failure restores `traefik.previous` and restarts.
- Hosts are upgraded one at a time; the next host starts only after the previous one verified.
- Before a major upgrade, Hashi validates its rendered static and dynamic config against the target version's config
  schema and blocks the upgrade if anything Hashi generates is no longer valid.
- Pinned plugin versions (e.g. Coraza) are checked the same way against the plugin catalog.

## 3. Static configuration (fully Hashi-managed)

- Entry points: `web` :80/tcp (redirect to websecure), `websecure` :443/tcp (+ HTTP/3 on 443/udp optional), and one
  entry point per public TCP/UDP service port, named `tcp-<port>` / `udp-<port>`.
- Providers: file provider, directory `/etc/hashi/traefik/dynamic`, `watch: true`.
- No certificate resolvers: certificates are files issued and distributed by Hashi (03-dns.md §5.3), loaded via the file provider's `tls.certificates`.
- Access log: JSON, `/var/log/hashi/traefik/access.log`, fields limited to what the continuous tail needs (incl. the WAF
  plugin's match fields, 06 §6.2), `Authorization`/`Cookie`/`Set-Cookie` and
  any configured header dropped; rotation via logrotate (Hashi-installed config).
- General log: JSON, `/var/log/hashi/traefik/traefik.log`.
- `ping` enabled on an internal entry point; API/dashboard enabled on `127.0.0.1` only (used for Verify), never public unless exposed as a protected system service.
- Plugins (Coraza WAF etc.) pinned by version; the pinned version must exist in the Traefik plugin catalog (checked by `make contract-capture PROVIDER=traefik-plugins`).
- `serversTransport` defaults; `forwardedHeaders.trustedIPs` = firewall-host internal IPs so client IPs survive DNAT.

Static changes require a Traefik restart and are high-risk.

## 4. Dynamic configuration files

| File | Content | Editable |
| --- | --- | --- |
| `00-hashi-core.yml` | Generated default middlewares | no |
| `10-hashi-http-services.yml` | HTTP/H2C/HTTPS routers and services | no |
| `20-hashi-stream-services.yml` | TCP/UDP routers and services | no |
| `30-user-middlewares.yml` | User-defined extra middlewares | **yes**, via UI editor |
| `40-hashi-security.yml` | Forward-auth and WAF middlewares (one WAF instance per mode/exclusion set) | no |
| `90-hashi-health.yml` | Health routers for monitoring | no |

The user middleware editor (CodeMirror YAML) parses strictly, extracts middleware names, and offers valid names as
toggles on services. Parse/validation errors keep the previously applied file and show the error inline.

### 4.1 Default middlewares (per service chain)

HTTP→HTTPS redirect, security headers, compression, Hashi forward auth (always; fail-open fallback only for services
whose rules cannot deny or require authentication, 06 §2.1), WAF (per mode; per-path modes become extra routers with a
higher priority, 06 §9), optional error pages. No Traefik rate-limit middleware: request rates are Stage A's job (06 §3.2).

## 5. Traefik connection

Fields: name, SSH target (address set, 05a §5.2: static IP, host, DNS name or Pulse agent), SSH port, username, password or private key
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
4. **Verify** through the local Traefik API (`GET 127.0.0.1:<api>/api/rawdata`): every expected router/service/middleware
   exists and none reports `status: disabled` or an `error`. Hashi reaches the loopback port through the SSH connection;
   when sshd refuses forwarding (Alpine ships `AllowTcpForwarding no`) it runs the host's `curl` or `wget` instead, and
   validation fails with a clear message when neither works. There is no `traefik check` command; do not invent one.
5. Store per-file `last_applied_hash`.

## 7. Sync rules

- Every Traefik connection receives every enabled service; DNS decides which instance receives traffic, so routing can
  move by DNS without re-rendering services.
- A new public port requires confirmation (high-risk; also opens the firewall, 05-firewall.md).
- Removing the last service on a port removes its entry point and firewall opening after confirmation.
- Disabled services stay in the DB but are absent from generated files.
- Reused ports pending removal are cancelled, not deleted, when a service claims them again.

## 8. API

`/api/services` CRUD + `POST /api/services/{id}/enable|disable`, `GET /api/services/{id}/effective`; access and WAF:
06 §14; (rendered config
preview), `/api/traefik/connections/*` (CRUD, validate, install, discover, adopt), `GET|PUT /api/traefik/user-middlewares`,
`POST /api/traefik/plan`.

## 9. Tests that must exist

- Golden-file render tests for every service kind, route option, rewrite mode and middleware combination.
- Rendered YAML parsed back and validated against the Traefik dynamic-config JSON schema (pinned copy in `testdata/`).
- Identical render → zero file writes (invariant 8).
- Integration: real Traefik binary (pinned version) in a test container loads the rendered files and its API reports no errors;
  a request through it reaches a stub upstream (HTTP, H2C, HTTPS upstream, TCP, UDP).
- Static-config restart failure → automatic restore.
