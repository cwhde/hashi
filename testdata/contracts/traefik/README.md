# Traefik contract (captured 2026-10-28)

- Remote software: Traefik v3.7.16 (official release binary, linux/amd64), Alpine 3.23 LXC, run in the foreground with
  `traefik --configFile=<static_capture.yaml>`. Disposable dev target, no real traffic.
- Docs: https://doc.traefik.io/traefik/reference/install-configuration/api-dashboard/ (API),
  https://doc.traefik.io/traefik/reference/routing-configuration/http/load-balancing/service/ (failover, health check),
  https://doc.traefik.io/traefik/reference/routing-configuration/http/middlewares/forwardauth/,
  https://doc.traefik.io/traefik/reference/routing-configuration/http/tls/tls-certificates/,
  https://github.com/traefik/traefik/releases (assets, checksums).
- Domains are `example.test`, upstreams and stubs listen on 127.0.0.1. Nothing real to sanitize.

| File | What |
| --- | --- |
| `checksums_v3.7.16.txt` | release checksum file: `<sha256>  traefik_<tag>_<os>_<arch>.tar.gz`; archive holds `traefik`, `LICENSE.md`, `CHANGELOG.md` |
| `static_capture.yaml` | static config Traefik accepted (API + ping on a loopback entry point, file provider, JSON logs, `hashi-fa` loopback entry point trusting loopback forwarded headers) |
| `dynamic_capture.yml` | dynamic config: forward auth through the local failover route, a direct (fail-closed) forward auth, a deliberately broken router |
| `certs_capture.yaml` | `tls.certificates` file list |
| `api_rawdata.json` | `GET /api/rawdata`: top-level `routers`/`middlewares`/`services`, keys `<name>@<provider>`, `status` `enabled`/`disabled`, `error` array on the broken router, `serverStatus` per URL |
| `api_rawdata_fa_down_ping.json` | the same with the forward-auth endpoint down: `serverStatus` `DOWN` on the health-checked primary |
| `api_router_not_found.txt` | `GET /api/http/routers/nope@file` → 404 `{"message":"router not found: nope@file"}` |
| `api_version.json`, `api_overview.json`, `api_http_routers.json` | other API reads |
| `traefik_log_errors.jsonl` | JSON log lines for the broken router and the failed health check |
| `access_log_tail.jsonl` | JSON access log lines |
| `summary.txt` | observed status codes and certificate fingerprints, in order |

Findings that shape the implementation (also in `docs/spec/environment.md`):

1. Replacing certificate files on disk while `hashi-certs.yaml` stays byte-identical is **not** picked up (same
   fingerprint after 6 s). Rewriting the YAML reloads them. So the certs file must change whenever a certificate changes.
2. `noop@internal` answers **418**; as a forward-auth fallback that denies. `ping@internal` answers **200 OK** on any path
   and works as the fail-open fallback (`failover` service: primary = Hashi with `healthCheck`, fallback = `ping@internal`).
3. The local forward-auth entry point must trust loopback forwarded headers (`forwardedHeaders.trustedIPs`), otherwise
   Traefik replaces `X-Forwarded-Host` with the loopback address and drops `X-Forwarded-Uri`/`-Method` on the second hop.
   With it, Hashi receives the original host, URI, method and cookies; `X-Forwarded-For` gets a second `127.0.0.1` appended.
4. Fail-closed forward auth (straight at Hashi) returns **500** while Hashi is down; the failover route returns to the
   primary within one health-check interval after Hashi is back.
5. Missing middleware → router `status: disabled` with `error: ["middleware \"x@file\" does not exist"]`, request 404.
6. `traefik healthcheck --configFile=…` exits 0 when ping answers. The web entry point redirect answers 301.

`schema_v3_file_provider.json`: Traefik v3 file-provider (dynamic config) JSON schema from SchemaStore
(https://json.schemastore.org/traefik-v3-file-provider.json), fetched 2026-10-28. Rendered dynamic files are validated
against it (04 §9); the real-binary integration test remains the authority when the two disagree.
