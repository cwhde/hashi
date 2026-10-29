# Progress

The single place that says where the project stands. Updated at the end of every working session.
Rules: docs/spec/11-delivery.md §2.

## Current slice

**S6 — IP sets + firewall + streams** · status: **in progress** (started 2026-10-29; entry gate passed the same day). PR A merged (#36). PR B in progress on `s6-firewall`.

Scope and gate: 11-delivery.md §3 row S6: 05a-ip-sets.md (`ip`, `cidr`, `host`, `dns`, set references, system sets,
compiler, IP sets pages, address page without dynamic state), 05-firewall.md (nftables table, folding, bindings, host
access, rescue; meters rendered but off), TCP/UDP services, DNS host and service records (03 §5.1–5.2), setup step 6.
Gate: first apply with the rescue timer on a real host; foreign + NetBird rules unchanged (diff of `nft list ruleset`
minus Hashi's table and of `iptables-legacy-save` before/after); NetBird connected after apply and after a NetBird
restart; a blocked address dropped and an allowed /32 inside it passing (real test sources); a public TCP/UDP port
reaching its upstream; Hashi's table surviving a `hashi-firewall` restart; a DNS record with an address-set value
applied; setup completed on the real install, sign-in with the S5 passkey, the S3 record path repeated with it; a
migrated service's generated CNAME applied.

Entry gate (05 §10) — before any renderer code:
- [x] Versions on the first real host (read-only): kernel 7.0.2-pve, nftables 1.1.3, `iptables` = legacy 1.8.11,
      NetBird 0.81.3 (kernel WireGuard, port 51820, connected), no nft tables, `nftables.service` disabled (its
      `/etc/nftables.conf` starts with `flush ruleset`). NetBird lives in iptables-legacy chains (`NETBIRD-*`) + ipsets.
- [x] Every §3.2 shape written as nft text and checked by the real binary: Debian 13 container (nft 1.1.3, kernel 6.8)
      and the Debian dev target (kernel 7.0.6-pve) with `nft -c`; the §3.3 transaction applied twice keeps `dyn` and
      `staging` elements (`testdata/contracts/nft/`).
- [x] NetBird backend after a restart with a foreign nft table: NetBird's own selection code (v0.81.3, same libraries)
      replayed in a netns → stays on iptables with `table inet hashi`; only a table named `filter` would switch it.
      **Coexistence holds; plan B (ADR 0002) not needed.** Replay kept in `testdata/contracts/netbird/backend_check/`.
- [x] `nft -c` (check mode, nothing committed) of the shapes, their JSON and the apply batch on the first real host
      (user OK 2026-10-29): all ok; `nft list ruleset` still empty, `iptables-legacy-save` + `ipset save` hashes equal
      before/after (a first attempt overlapped the old script's 15-minute run, which adds its duplicate rules: 559 now).
      **Entry gate passed.**

Found on the first real host (read-only), for the takeover plan (05 §3.4):
- The old script adds rules to built-in chains besides its `CUSTOM_*` chains: MSS clamp (mangle FORWARD), three
  MASQUERADE rules (nat POSTROUTING), two `wt0`↔bridge ACCEPTs (filter FORWARD), the `trusted_hosts` ipset ACCEPT.
  Its cleanup deletes the two ACCEPTs in a different form than it inserts them, so every 15-minute run adds another
  copy: 555 of each today. Harmless (same verdict) but the takeover must remove all copies.
- It re-installs root's crontab from `/scripts/crontab.txt` every 15 minutes (`sync_and_apply.sh`), so cron changes
  must go into that file too. The same crontab carries unrelated jobs (log shrink, rclone mount) that stay.
- It also does non-firewall work: pins `mesh.<root>` in `/etc/hosts` and sets cluster MTU routes. Removing the script
  stops those; decided with the user at the takeover plan.
- Its `CUSTOM_INPUT` ends in DROP: today all other inbound traffic is already blocked. Keeping that posture after the
  takeover means enabling Hashi's "block all other inbound" for that host (user decision at the takeover plan).

Plan (three PRs: S6 is the largest slice; each PR is one coherent layer, ~1,500 hand-written lines):

PR A — IP sets
- [x] Migration 00007: `ip_sets` (kind user/system, origin, locked, name, description) + `ip_set_elements` (typed rows
      `ip`/`cidr`/`host`/`dns`/`set` with CHECKs, note, expires_at); system sets `all`, `hashi-hosts`,
      `private-networks` seeded; sqlc.
- [x] `internal/ipsets`: sealed item variants, normalization, cycle check (DFS, path in the error), compiler
      (v4/v6 interval lists, merged; status ok/partial/empty + problems; expiry drop-out), address-set capability,
      DNS item resolver (TTL-bounded refresh), reverse dependency graph with debounced consumer notification.
      Tests incl. `TestInvariant_22_SetGraphsAcyclic` (IP sets part) and property tests of the merge.
- [x] OpenAPI + handlers: `/api/ip-sets` CRUD, elements, resolved, consumers, `GET /api/addresses/{address}`
      (containing sets + nesting path; firewall verdict once PR B lands).
- [x] UI from the mocks: IP sets list, set page (elements, resolved, consumers), address page (no dynamic state);
      shared IP set picker + item editor. E2E against the dev fakes.

Decisions made in PR A:
- Reactivity is "recompile every set, compare hashes, notify consumers of the changed ones" instead of a stored reverse
  dependency graph: same notifications, far less state at homelab size. Consumers subscribe with the changed set IDs.
- dns items use the Go resolver and refresh every 5 minutes (it exposes no TTLs; spec c58229d). Expired items are
  deleted by the `ipsets.refresh` job (every minute) with one `ip_set.element.expired` audit entry each.
- `host_id`/`pulse_agent_id` carry no foreign key (their tables arrive later); their owners refuse deletes while
  referenced. Host items are refused until a firewall host exists (PR B wires the host lookup).
- A cycle answers 400 with its path; other item errors 422; locked sets 409. The audit list takes a `target` filter
  (a set's History tab). The ⌘K address lookup stays a mock until the palette is built for real.
- Review of PR A: the set page now pages past 200 items ("Show more"); ⌘K lookup and a test of the 2 s debounce are
  not done (noted, not reproduced as faults).

PR B — Firewall hosts (backend + API)
- [x] Migration 00008: `firewall_hosts` (05 §1 fields, edge mode, NetBird mode, pinned host key, credential refs,
      detected facts, setup ruleset), bindings, host access; sync subsystem `firewall` (sync tables rebuilt).
- [x] Validation over SSH (05 §2): one captured probe (kernel, nft + JSON, iptables backend, NetBird from `status
      --json` + routed networks, a `filter` table, `nftables.service`, firewalld, WAN/public address, listeners),
      `$SSH_CONNECTION` as the rescue source, the full existing ruleset kept once.
- [x] Folding (05 §4.2) with the table-driven cases and the brute-force property test.
- [x] Renderer: Go structs → libnftables JSON matching `testdata/contracts/nft/`; golden files per host shape; every
      golden passes `nft -c -j -f` in the integration container; meters rendered but off; `TestInvariant_20` (kernel).
- [x] Apply + rescue (05 §3.3, §7): §4.4 risk, previous table + file saved, `rollback.sh`, `systemd-run` timer with
      `AccuracySec=1s`, verify by set coverage, rescue over a new SSH connection + NetBird status, boot unit, sysctl.
      Integration: foreign + NetBird-like rules byte-identical, DNAT to a Traefik stand-in, timer rollback.
- [x] Manager + engine subsystem, `hashi-hosts` maintained, host items resolve, bindings/host access re-plan, the
      lock-out guard (active admin sessions + rescue source).
- [x] OpenAPI + handlers (05 §9 without kernel-sets/rollback, which come with the host link in S10), setup step 6 API.

PR C — DNS records, address-set targets, streams, takeover
- [ ] DNS host records (`<host>`, `via.`, `on.`, `ovr`) and service records (03 §5.1–5.2).
- [ ] Address sets as targets: service upstreams and DNS A/AAAA values (`TestInvariant_21` for both).
- [ ] TCP/UDP services: model, `20-hashi-stream-services.yml` + `tcp-<port>`/`udp-<port>` entry points (static
      change, high risk), DNAT to the entry-point port; golden + Traefik integration with stub TCP/UDP upstreams.
- [ ] Takeover of an existing firewall script (05 §3.4) as explicit plan items: exact rule specs incl. all
      duplicates, the ipset, cron lines in the live crontab and `crontab.txt`; rollback restores them.
      User decisions 2026-10-29: the script's non-firewall duties (`/etc/hosts` pin, cluster MTU routes) are dropped
      with it; the first real host gets "block all other inbound" (today's `CUSTOM_INPUT` ends in DROP and nothing else
      keeps Proxmox :8006 or sshd closed to the public); its trusted addresses move into a host access set.
      Hashi's SSH public key is added to root on the first real host by Claude (user OK 2026-10-29), at the gate.

PR D — UI + deploy
- [ ] Firewall pages from the mocks (hosts, host page with observed ruleset, rendered table, bindings, host access),
      setup step 6 page (step becomes built), stream fields on the service page; dev fakes (`firewalltest`); E2E.
- [ ] Prod-proof first on a dev target (Debian LXC on host B, snapshot `clean`), then deploy (user runs `/deploy`) and
      the gate on the first real host with the user's OK for the first apply.

Decisions made in PR B:
- NetBird's routing-peer rules already accept and masquerade overlay traffic (capture), so Hashi renders no NetBird
  NAT; only the MSS clamp. Hairpin DNAT matches the public address from non-WAN interfaces (not `fib daddr local`,
  which would also catch the bridge address).
- Risk is computed from the rendered file on the host vs the new one; Verify compares set coverage (nft may list
  merged ranges). A failed `sysctl -p` is accepted when the wanted value is already in effect (containers, LXCs).
- Firewall hosts log in like Traefik hosts (Hashi's key, a private key or a password); SSH and DNAT targets are
  literal addresses until connection targets become address sets with Pulse (S8). nft must already be installed
  (Proxmox has it; validation says so otherwise); Alpine/OpenRC firewall hosts (no systemd-run) come later.
- Step 6 completes at "confirm" (validated, linked to a Traefik connection), like step 5; the first apply is planned
  and run from the step but is not required to finish it. An external_nat host completes it without SSH.
- The step's page, dev fakes and E2E come with PR D, so the step stays unbuilt in dev builds until then.
- Review of PR B (reproduced, fixed with regression tests): folding used merged set prefixes (a /32 inside a /16 of the
  same allow set lost to a /24 block); a carved /16 block counted as low risk; the last host was deletable after setup
  (invariant 11); bindings, host access and edits of firewall-bound sets lacked the passkey confirmation (02 §2).
  Added: hourly `firewall.reconcile`, service changes re-plan firewall hosts.

## Previous slice

**S5 — HTTP services** · status: **done** 2026-10-29, deployed `3c4504e` (#34, #35 + gate fixes), prod-proof passed. Next: S6.

Scope and gate: 11-delivery.md §3 row S5 (amended 77ca773 with the user, 2026-10-28): 04 §1, §7–8 for the HTTP kinds,
system service (setup steps 7–8), deeper wildcards (03 §5.3). Gate: Hashi admin reachable at its HTTPS domain through
Hashi-managed Traefik (step 8 proof on the real install); the first real passkey registered from a real browser on the
temporary admin domain; one real service served by the new Traefik and reachable through it. Setup completion,
passkey sign-in, the S3 record repeat and generated host/service records move to S6 (they need a firewall host).

User decisions 2026-10-28: temporary admin domain `dev.hashi.<root>` (needs the deeper wildcard `*.hashi.<root>`);
first real service: SearXNG (its public name stays on the old setup until the S6 traffic move). The temporary name
resolves through a manual DNS entry (S3) pointing at the edge's internal address, so it opens from the home network and
NetBird only.

External systems and contracts:
- Traefik v3.7.16 dynamic config for services (routers with Host/Path rules and priorities, `stripPrefix`,
  `replacePath`, `replacePathRegex`, `headers.customRequestHeaders`, `serversTransports` with `insecureSkipVerify` /
  `serverName`, `h2c://` servers, weighted services): proven against the real binary in the integration test (loads,
  every object `enabled`, requests reach stub HTTP/H2C/HTTPS upstreams with the expected path) plus the pinned schema.
- Edge → Hashi source address (captured 2026-10-28): the remote edge's connection arrives from the gateway address,
  the same address routed laptops get, hence the edge token (spec 77ca773).
- Hetzner, ACME, sshd: already captured.

Scope cut: TCP/UDP services, address-set targets (literal IPs until S6), DNS service records, rule sets and WAF
(the service pages show them as arriving later), user middlewares file (`30-user-middlewares.yml`) and adding Traefik
hosts after setup (both before the V1 cutover, with S6's edge move).

PR A — backend
- [x] Migration 00006: `services` (name, slug, kind http/https/h2c, domain mode + host name, enabled, origin, owning
      feature, dashboard/monitoring/AdGuard flags, target scheme/address/port, `https` options insecureSkipVerify +
      serverName, TLS min version, advanced-routes flag) and `service_routes` (ULID, enabled,
      targets with weights, path match prefix/exact/regex, value, priority, rewrite mode + params); sqlc.
- [x] `internal/services`: model, validation (04 §1.5: FQDN inside the managed zone, reserved labels, ports, RE2 at
      save, paths start with `/`, IPv6 bracketed), invalid services excluded from rendering, the Hashi system service
      kept in step with General settings (admin domain → internal URL; delete forbidden, invariant 10).
- [x] Render `10-hashi-http-services.yml`: router per service and route (`Host` + path rule, websecure, TLS, priority),
      chain headers → compress → forward auth (fail-open while no rule set can deny) → route rewrites; services with
      weighted servers, `serversTransports` for `https`, `h2c://`; edge token header on the system service and on
      forward auth. Golden files for every kind, path match and rewrite mode; schema check; integration with the real
      Traefik and stub upstreams.
- [x] Certificates: desired names = root + `*.root` + one wildcard per deeper parent of an enabled service's host name;
      a changed name set re-issues (existing "names changed" rule), dropped wildcards wait for renewal.
- [x] Edge token (vault secret, rendered on every edge), client-IP resolver trusts requests that carry it; saving a
      service plans every Traefik host (low risk) and the certificate when names change.
- [x] OpenAPI + handlers: `/api/services` CRUD, `enable|disable`, `GET /api/services/{id}/effective` (rendered
      routers/services for it), `POST /api/traefik/plan`; setup step 7 (system service → certificate + Traefik plans
      run; done once every host verified the admin router and serves a certificate covering the admin domain) and
      step 8 (nonce → `https://<admin>/setup?https=…` → proof through the edge token, `Host`, `X-Forwarded-Proto` →
      done + `Secure` bootstrap session on that origin). Tests incl. invariants 7, 8, 10.

Decisions made in PR A:
- Targets are a single IP or host name until S6's address sets; a host name goes to Traefik as is.
- Values with `{{` or `}}` are refused: Traefik's file provider runs dynamic files through Go templates.
- The edge token is set by the `hashi-fa` router (forward-auth calls) and on the Hashi admin service only, so
  upstream services never see it; `40-hashi-security.yml` carries it and is owner-only (0600).
- The step 8 nonce travels in the URL fragment, so it never reaches an access log; the proof also requires a
  bootstrap network, like the bootstrap login.
- A service change starts the certificate job at once when a name is not covered (`jobs.Runner.Soon`); setup step 7
  issues synchronously, like step 4's "Issue now".
- Deleting a service needs a fresh passkey; the resulting Traefik plan is low risk (dynamic files only).
- A service's invalid reason is computed on every read (against the current root and zone), not stored; its sync state
  is the hosts' (per-file `last_applied_hash` in `traefik_files`), since every host receives every service.
- Review #34 (reproduced, regression tests added): a failed admin-domain change (name taken) no longer stays saved
  while the system service stays behind (`TestAdminDomainChangeIsAllOrNothing`); routes are validated while advanced
  routes are off (they are stored either way). New scope `services.manage`, also on bootstrap sessions (spec
  0e502cc), so the gate's service can be added before setup completes; `TestServicesAPI` covers the API over HTTP.
- CI found that Traefik rejects a dynamic file with only `http: {}` and then drops every file of the provider; the
  services file is comments only while no service renders (contract finding 7).

PR B — UI + deploy
- [x] `/services` list and service page from the mocks (target, routes, rewrites, TLS, flags, rendered config;
      Access/WAF/monitoring marked as later slices); add/edit drawer; plans through `/changes`.
- [x] Setup steps 7 and 8 pages (step 8 opens the HTTPS origin and continues there), steps 7–8 built.
- [x] E2E against the dev fakes: system service applied on the fake edge, a user service with a route and rewrite,
      step 8 proof with a simulated edge request.
- [x] Deploy (user runs `/deploy`) and prod-proof: manual DNS entry `dev.hashi` → edge internal address, steps 7–8 on
      the real install, real browser opens `https://dev.hashi.<root>` with the issued wildcard, first passkey
      registered by the user, SearXNG served through `hashi-edge-test` (request with the real host name), V1 untouched.


Decisions made in PR B:
- A setup session reaches the services pages (and nothing else of the admin app) while setup is open, so the gate's
  service can be added from the browser before passkey sign-in exists (scope `services.manage`, spec 0e502cc).
- Step 8's Continue is not tied to the HTTPS address; the passkey ceremony on step 9 fails clearly anywhere else.
  Dev builds with fixtures trust a fixed edge token (`dev-edge-token`) so the E2E can send the proof; release builds
  never contain it.
- Upstreams may be single-label names (`localhost`, a container name).
- Review of PR B (reproduced, fixed): setup session could not open /services; delete needed the passkey retry;
  hidden TLS options blocked a switch away from https. The E2E covers the setup session on /services and delete.
  Not covered by E2E: the proof sent on arrival at the HTTPS address (the gate does it for real), the https kind's
  options, regex rewrites and weights (golden files and the Traefik integration test cover their rendering).
- A flaky S3 E2E check had a UI cause: `/dns` said "Decisions saved." before its follow-up plans finished, and a
  reload on that text cancelled the records plan. Fixed in the page.
## Side step: security model (branch `security-model`)

Status: **done** (signed off 2026-10-28, mocks merged). S2 is next.

The user replaced the security design with a model built on two shared primitives: IP sets (the one address primitive,
also for DNS values, upstreams and connection targets, so anything can be "set via Pulse") and rule sets (global,
reusable and service-root rule sets are one type), a two-stage edge engine (Stage A abuse engine before Stage B rules),
one ban function, and nftables with folded allow/block sets and kernel meters. Original input archived in
`docs/archive/` (not for implementation); decisions, corrections and rejected alternatives in ADR 0002.

Done on the branch:
- [x] ADR 0002; spec rewritten: 05-firewall, new 05a-ip-sets, 06-edge-security, 06a (object model §0: what is shared,
      what is compiled per layer, deliberate deviations); updated 01, 02, 03, 04 (renamed `04-traefik-services.md`),
      07, 08, 09, 10, 10b, 11, README (invariants 7, 10, 12 amended; 20–22 added), environment, CLAUDE.md.
- [x] Read-only survey of the first firewall host: kernel 7.0, nft 1.1.3, iptables **legacy**; NetBird uses legacy
      chains + ipsets; old script chains still present; Proxmox firewall disabled; no nftables tables;
      `nftables.service` disabled with the stock `flush ruleset` config.
- [x] Counters view (ban + challenge counter, filters, edit/reset).
- [x] Review round 2026-10-28: step decay for both counters; an SSO session satisfies `pass_captcha`; observe/detect
      modes stay out; host link stays SSH (ADR 0002 "Host link vs host agent"); Pulse footprint targets (08 §1).

After sign-off (phase 2):
- [x] Merge the spec branch (docs → `main`).
- [x] Redo the affected S0b mocks (Services, Security incl. Counters, address page, IP sets, rule sets, Firewall) —
      branch `s0b-security-mocks`.
- [x] Redone mocks approved by the user (2026-10-28), plus host and per-service routing (03-dns.md §5.1 `ovr` records).
- [x] Mocks PR #27 merged.
- [x] Built code checked: only setup step 7's key/title and the palette mock said "resource"/subjects; renamed.
- [ ] Then continue with S2.

## Finished slices (detail)


**S4 — SSH + Traefik** · status: **done** 2026-10-28, deployed `2868d0e` (#32, #33), prod-proof passed. Next: S5.

Scope and gate: 11-delivery.md §3 row S4: 04-traefik-services.md §2–6, sshx with host-key pinning, Hashi-issued
certificates (03 §5.3), setup steps 4 (certificate provider) and 5 (Traefik connection). Gate: Hashi-installed Traefik
running on a fresh edge LXC; rendered files applied; Verify reads routers from the real Traefik API; Hashi issues the root +
wildcard certificate via DNS-01, distributes it, and the edge serves it (fingerprint checked); forward-auth fallback
observed with Hashi stopped, using a stub forward-auth endpoint (06 §2.1).

Dev targets: three temporary LXCs on host B, one per OS family Hashi installs on: `hashi-edge-test` (Alpine 3.23,
OpenRC, sshd without forwarding), `hashi-edge-debian` (Debian 13, systemd) and `hashi-edge-fedora` (Fedora 44, systemd,
dnf). Same size and settings as the first (see Log 2026-10-28); not the production edge.

External systems and contracts:
- Traefik v3.7.16 release, static/dynamic config, `/api/rawdata`, cert reload, forward-auth fallback: captured
  2026-10-28 (`testdata/contracts/traefik/`). Spec corrected from it (3e0bef4).
- OS probe over SSH (Alpine 3.23, Debian 12): captured (`testdata/contracts/os/`).
- sshd: real `sshd` in the integration container (x/crypto/ssh + pkg/sftp, 01 §2), no fixture needed.
- Hetzner `add_records`/`remove_records` for `_acme-challenge`: already captured (`testdata/contracts/hetzner/write/`).
- ACME: Pebble (the ACME test server, with EAB required) in the integration container; Google Trust Services in the gate.
  User decisions 2026-10-28: CA = **Google Trust Services** (EAB; the user creates a fresh EAB key for the gate, the one
  in the old Traefik config is not reused); Hashi may write and remove `_acme-challenge` TXT on the real zone, in
  development and on the server (nothing else beyond `_hashi-test`).

Scope cut: services (S5) are not rendered yet, so `10-`/`20-hashi-*` files stay absent; the gate's route is a Hashi health
router on the edge. Deeper wildcards come with S5 services. The Traefik install is proven on all three dev targets
(Alpine/OpenRC, Debian/systemd, Fedora/systemd + dnf); Ubuntu shares Debian's path. Forward auth is a stub that allows everything (the engine is S9/S10).

PR A — backend
- [x] `internal/sshx`: x/crypto/ssh client, mandatory host-key pin (scan → user confirms the SHA256 fingerprint → stored),
      key or password auth, Hashi's own ed25519 key (public half as an authorized_keys line), `Run` with capped output,
      SFTP read/hash, atomic write (same-dir temp + chmod + posix-rename, skipped when the hash matches), `Fetch` of
      loopback URLs through the SSH connection or the host's curl/wget. `make test-integration` + CI job with a real
      OpenSSH container (Alpine 3.23): pin, both logins, changed host key refused, atomic writes, both fetch paths.
- [x] Migration 00005: certificate provider (ACME email, EAB, account key ref), `traefik_connections` (target, port, user,
      credential refs, pinned host key, internal IP, OS, paths, state), `traefik_files` (per-host `last_applied_hash`),
      `certificates` (+ per-host distribution state); sync subsystems `traefik` and `certificates` in the CHECK + OpenAPI enum.
- [x] `internal/traefik`: OS probe parser (all four fixtures; family from tools, never versions), pinned release
      (embedded SHA-256, asset per arch, binary extracted only after the checksum matches), Hashi's directories,
      `hashi-traefik` OpenRC script (shape of Alpine's own, captured) / systemd unit, static config and dynamic files as
      Go structs → YAML (`00-hashi-core`, `40-hashi-security` with the fail-open/closed construct, `90-hashi-health`,
      `hashi-certs.yaml` with certificate hashes), golden tests. Rendered files loaded into a real Traefik v3.7.16
      container: every router/middleware/service `enabled`, no errors, health `/open` 200 and `/closed` 500 with Hashi absent.
- [x] Apply + Verify (04 §6): `Observe` reads the host fresh (binary version, directories, service file, file hashes)
      → plan items with paths and hashes only (first install, binary, service and static file high risk; dynamic and
      certificate files low); `Apply` writes only changed files (certificates before `hashi-certs.yaml`, static last),
      enables/restarts, waits for ping, restores the previous static file + service definition on failure and waits for
      Traefik to come back; existing configs backed up to `/var/lib/hashi/traefik/backup/<ts>/` once ownership is
      confirmed; `Verify` = every rendered router/middleware/service `enabled` in `/api/rawdata`. `traefik` sync subsystem
      (`Manager`): builds the desired state from the connection, internal URL and issued certificates, logs in with
      Hashi's key or the stored credential against the pinned host key, records `last_applied_hash` (never for keys),
      installed version and health. Tests: fake host (`TestInvariant_8_OnlyChangedFilesAreWritten`, restore, backup,
      engine end to end), schema check against the pinned SchemaStore schema, integration with the real Traefik image
      over real sshd (apply, verify, no rewrite, fail-open 200 / fail-closed 500 with the upstream stopped, restore after
      a static change Traefik cannot start with). Not wired into `internal/app` yet: comes with the handlers.
- [x] Certificates (03 §5.3): `internal/certs` on `golang.org/x/crypto/acme` (already a dependency; lego not needed):
      account with EAB (key created and registered on first use, vault), order root + `*.root`, DNS-01 through
      `dns.Challenger` (single TXT values via Hetzner `add_records`/`remove_records`, captured; `_acme-challenge` names
      reserved: no import, prune or entry), wait until the value is visible on the zone's nameservers (TCP) or the
      configured resolvers, then the propagation delay; certificate key in the vault, old key deleted on renewal.
      `certificates` subsystem (target = account: issue missing / renew inside 30 days / names changed; target =
      certificate: "Renew now"), `certificates.renew` job (auto plan, then `traefik` plans for hosts whose distribution
      lags, expiry levels 21/7 days in the summary and log). `traefik` apply checks each certificate by TLS handshake
      to the edge's :443 and stores per-host state (`verified`/`failed`). Tests: plan rules, levels, Hetzner value
      replay, challenger leaves other values, TLS check with a local listener, engine end to end with distribution;
      integration with Pebble 2.10.1 (EAB required) + challtestsrv (issue, nothing due, renew now) and the real
      Traefik serving a renewed certificate without restart. Job and handlers are wired with the next item.
- [x] Stub `GET /api/edge-auth/forward` (204) on the admin listener; health router on the edge uses it fail-open.
- [x] OpenAPI + handlers: setup step 4 (`PUT /api/setup/certificates`: validate, seal EAB, register the account; step
      done only when the CA accepted it; `POST /api/setup/certificates/issue`), step 5 (`/api/setup/traefik/identity`
      (Hashi's key), `/scan` (host key shown, nothing stored), `/connection` (save with exactly the confirmed key, then
      validate: OS, root, existing Traefik, write check in `/var/lib/hashi/traefik`), `/confirm` (ownership, step done),
      `/plan` + `/plan/{id}/run`), admin `GET /api/traefik/connections`, `POST /api/traefik/connections/{id}/plan`,
      `GET /api/certificates`, `POST /api/certificates/{id}/renew` (03 §7 paths). Managers and the daily
      `certificates.renew` job wired in `internal/app`. Tests: `TestInvariant_1_ChallengeValuesLeaveOtherWritersAlone`,
      `TestInvariant_7_TraefikChangesOnlyThroughApprovedPlans`, `TestInvariant_13_TraefikApplyIsVerified`, invariant 5
      (EAB key and SSH password never returned, logged or stored in plain text), setup API over HTTP; Pebble test goes
      through `Configure` (unknown EAB key ID refused by the CA). Steps 4–5 count as "built" once their wizard pages
      exist (PR B), so the current wizard and E2E keep working. Adding further Traefik hosts after setup comes with the
      `/traefik` page (S5 needs it, not the S4 gate).

Review of PR A (#32), fixed with regression tests (`internal/traefik/review_regress_test.go`, each failed before the
fix): existing config is backed up once (`discovery.backed_up`), not in every plan; `installed_version` is recorded
after every verified apply, so hosts that already ran v3.7.16 get certificates; a binary upgrade keeps
`traefik.previous` and restores it when the new binary does not start (04 §2.1).

PR B — UI + deploy
- [x] Access-log rotation (04 §3): captured on the dev targets (Alpine and Debian ship logrotate with a daily run;
      Fedora's container image lacks it, its package enables but does not start the timer; `pkill` everywhere,
      `testdata/contracts/os/README.md`). Hashi renders `/etc/logrotate.d/hashi-traefik` (daily or 100 MB, keep 14,
      postrotate `pkill -USR1` matched on Hashi's exact Traefik command line) and installs logrotate when missing
      (high-risk plan item). Integration: the real Traefik writes to a fresh `access.log` after `logrotate -f`.
- [x] Certificate retry: `certificates.renew` runs hourly (nothing to do while certificates are current); after a failed
      attempt it waits 4 hours, "Renew now" always tries (spec c84e87a).
- [x] Setup steps 4 and 5 (`CertificateStep`, `TraefikStep`): CA choice with EAB, account registration, issue now; host
      form from the mock, Hashi's key to copy, host key shown and pinned only after "It matches", checks, explicit
      takeover, install plan approved and run. Steps 4–5 count as built. Plan items for Traefik and certificates show a
      text summary (path, mode, short hash, version, names, expiry).
- [x] `/traefik` page: hosts (health, version, last check, host key, applied files with hashes, "Check and sync") and
      Certificates (names, issuer, days left by level, hosts serving the current fingerprint, failures, "Renew now").
- [x] E2E against dev fakes: `certstest` (local CA that still publishes DNS-01 values through the fake zone) and
      `traefiktest` (in-memory Debian 13 edge with TLS), both only behind `HASHI_DEV_DNS_FIXTURES` in `-tags dev`
      builds. Setup walks steps 4–5 (including "It doesn't match"), then `/traefik`: files, served certificate, renew,
      sync. A root domain outside the managed zone is refused with a plain message (spec c84e87a).
Review of PR B (#33), fixed: "Issue now"/"Try again" in setup after a failed first issue targets that certificate, so
the job's 4-hour wait does not swallow the click (`TestIssueNowRetriesAfterFailedFirstIssue`); Debian/Ubuntu run
`apt-get update` before installing logrotate (captured on `debian:13` and `ubuntu:24.04`, which lack it).

- [x] Deploy (user runs `/deploy`) and prod-proof on `hashi-edge-test` (full gate; also: `logrotate -f` on each target
      rotates and the OpenRC/systemd-started Traefik reopens its access log, i.e. the `pkill` pattern matches) plus the install plan, files and Verify
      on `hashi-edge-debian` and `hashi-edge-fedora`: host key pinned, install plan applied (Traefik under OpenRC/systemd), files applied, Verify from the real API, real certificate issued + distributed + fingerprint matches, Hashi
      stopped → fail-open route 200 / fail-closed 500 on the edge, Hashi back → normal; live Traefik instances untouched.


**S3 — DNS write** · status: **done** 2026-10-28, deployed `94d60ad` (#30, #31), prod-proof passed.

Scope and gate: 11-delivery.md §3 row S3: 03-dns.md §5–7, sync engine 01-platform.md §5, approval UI. Gate: a probe
TXT is created and deleted through an approved plan; a Hashi-owned test record is applied and verified; NS/SOA are untouched.

External systems: Hetzner Cloud API writes. The contract is already captured (`testdata/contracts/hetzner/write/`,
2026-10-28), so nothing new to capture.

Scope cut: §5 host and service records need firewall hosts (S6) and services (S5), and A/AAAA address-set values need
IP sets (05a). Those arrive with their slices on top of this engine. S3 delivers manual entries with literal values,
imports, prunes and the probe, all through plan → approve → apply → verify.

PR A — backend
- [x] Migration 00004: `dns_records` (desired manual entries: name, type, TTL or zone default, values, display name,
      dashboard/monitoring flags), `sync_plans`, `sync_plan_items`, `sync_runs`, `sync_locks`, observed `owner`; sqlc queries.
- [x] Hetzner write adapter from the write fixtures: create, `set_records`, `change_ttl`, delete, owner label (`PUT`),
      Action polling with timeout, TXT quoting/escaping/255-byte split; replay tests for every write fixture used.
- [x] `internal/dns` planner: desired (owned rows) vs observed → create/update/label/delete/conflict items; NS/SOA and
      unowned RRSets never planned (conflict instead); CNAME exclusivity and apex checks; deletes are high risk.
- [x] `internal/sync` engine: plan + items + hash, low-risk auto-approve, high-risk approval, apply under the lock with
      re-plan (a new high-risk item goes back to approval), verify by read-back, `last_applied_hash`, run row, audit.
      Background reconcile in the `dns.refresh` job.
- [x] Probe plan (create `_hashi-test` TXT → verify → delete → verify, high risk); import → desired entry `imported`
      (+ owner label as a low-risk item); prune → one high-risk plan listing every RRSet.
- [x] OpenAPI + handlers: `POST /api/dns/records`, `PATCH|DELETE /api/dns/records/{id}`, `POST /api/dns/plan|prune|probe`,
      `GET /api/sync/plans`, `GET /api/sync/plans/{id}`, `POST /api/sync/plans/{id}/approve|apply|discard`,
      `POST /api/setup/dns/probe`, `POST /api/setup/dns/probe/{id}/run`. Tests `TestInvariant_1/2/3/4/7_*` for the write side.

Decisions made in PR A:
- A user entry counts as Hashi's at the provider only after Hashi created it (`last_applied_hash` set). An RRSet that
  appears in the console between saving and planning becomes a conflict item, never an overwrite (invariant 1).
- Conflict-only drafts create no plan (the background reconcile would otherwise make one every hour); saving an entry
  over an unowned RRSet is refused up front with "import it instead".
- Owner marker = label `hashi.io/owner=<instance id>` (new setting `dns.instance_id`, a lowercase ULID); imported
  RRSets get it as a low-risk "label" item, other labels are kept.
- Updates use `set_records` and `change_ttl` only (`add_records`/`remove_records` are captured but not needed).
- Low-risk plans apply in the saving request (no 2 s debounce yet; one save = one plan). `GET /api/sync/runs` dropped:
  recent plans carry the result. The probe in setup step 3 is approved by the click on the shown plan (there is no
  passkey yet in setup); after setup it needs `sync.apply` + reauth like every high-risk plan.
- `make dev`/E2E use a stateful Hetzner fake (`hetznertest.Sim`) seeded from the recorded zone, so plans apply locally.
- PR A is ~4,000 hand-written lines (about half tests): one coherent backend, kept whole.
- Review #30 (all four reproduced, regression tests added): S2-era imports get their entry before planning (else
  they became deletes); entry deletes are their own subsystem `dns-record-deletes` so a waiting delete never holds
  back safe changes (invariant 16); after a failed apply/read-back the zone is re-read and what took effect stays
  Hashi's; the write test deletes its probe even after a failed create. Prod has no S2 imports (all 58 were "leave").

PR B — UI + deploy
- [x] `/changes`: pending plans with exact before/after (PlanReview), passkey approval, apply, discard, recent runs;
      top bar counts plans awaiting approval.
- [x] `/dns` Records: add/edit/delete entries inline (editor from the mock), "Save · creates a plan", pending tag;
      Import: saving creates the prune plan; "Test write access" on `/dns`; optional write test in setup step 3.
- [x] E2E against the fake: write test in setup, entry created → applied and verified, delete → Pending changes →
      passkey approval → gone; NS/SOA have no edit button.
- [x] Record gate against the real zone (spec row S3, as amended): `TestRealZoneRecordPath` (`-tags 'dev realzone'`),
      2026-10-28 on `s3-dns-ui`: 58 RRSets before; `_hashi-test` A 192.0.2.10 TTL 300 saved → plan applied "1 created,
      verified", direct read shows it with the owner label; delete → high-risk plan pending (still at the provider),
      approve refused without reauth, approved after a passkey assertion → "1 deleted, verified"; 58 RRSets after,
      NS hash and SOA (without the provider's serial) unchanged.
- [x] Deploy (user OK, `/deploy 94d60ad`) and prod-proof on the deployed install: write test through setup step 3's
      approved plan (probe TXT created + deleted); NS/SOA unchanged.

Decisions made in PR B:
- RRSets Hashi created (origin user/system/managed) are left off the import table: there is nothing to import.
- The setup write test shows its plan and runs on "Approve and run" (no passkey exists in setup yet).
- Saving import decisions (setup or `/dns`) plans the removals on the server, so "Remove" chosen in setup waits on
  Pending changes (review #31). The pending-changes counter is always in the top bar, amber when something waits.
- Review #31 fixed (reproduced): a save reported another entry's waiting delete as its own; a TTL typo became "zone
  default" without a preview; setup's removals never became a plan.

**S2 — DNS read + import** · status: **done** 2026-10-28, deployed `04ff8c7` (#28, #29), prod-proof passed.

Scope and gate: 11-delivery.md §3 row S2 — 03-dns.md §1–4 (read side), setup step 3 (02 §1.3). Gate: the real zone is
listed; the import table shows the real RRSets; import decisions persist; sanitized fixtures committed.

External systems: Hetzner Cloud API (DNS zones and RRSets). No contract yet → captured first (read-only).

Contract
- [x] `make contract-capture PROVIDER=hetzner`: list zones (paginated), get zone, list RRSets (paginated), bad token
      (401), unknown zone (404); raw into `.contracts-raw/`, sanitized into `testdata/contracts/hetzner/` with
      `captured_at` + docs URL; denylist check. Token: V1's project token copied read-only into the keychain
      (`hashi-hetzner-token`) on the user's request. Record names, external hosts and TXT contents are sanitized too.

PR A — backend
- [x] Migration 00003: DNS provider connection (type, default TTL, token as vault secret), `dns_zones`,
      `dns_observed_rrsets`, `dns_ownership`, `dns_import_decisions`; sqlc queries.
- [x] `internal/dns`: provider interface (read part: Capabilities, ListZones, ResolveZone, ListRRSets), RRSet model and
      normalization (TXT, TTL default); `internal/dns/hetzner` read adapter from the fixtures (pagination, RateLimit
      backoff, legacy-token error); replay tests; `TestInvariant_2_*` (NS/SOA never selectable for prune) on the import side.
- [x] Zone refresh job (observed snapshot) + OpenAPI: `GET /api/dns/zones`, `POST …/refresh`, `GET …/rrsets`,
      `GET /api/dns/import/{zoneId}`, `PUT …/decisions`; setup step 3 endpoints; `make generate`.

Decisions made in PR A:
- Setup step 3 has its own endpoints (`PUT /api/setup/dns/provider`, `GET|PUT /api/setup/dns/import`, scope
  `setup.manage`), because bootstrap sessions carry no admin scopes; they share the service with `/api/dns/*`.
- Step 3 stays `built: false` until PR B adds its UI and the E2E run covers it (flipping it now would break the E2E
  setup flow, which cannot reach step 3 without a page). `make dev`/E2E then need a fixture-backed fake provider.
- The observed snapshot stores TXT values decoded (chunks joined, RFC 1035 escapes resolved); re-chunking for writes is
  S3's job, from the write capture. `ttl: null` stays "zone default".
- NS/SOA and change-protected RRSets allow only "leave" on the import table; the schema backs this with CHECKs.
- `dns.refresh` job interval = the sync interval at startup (a changed interval applies after a restart).
- No 429 response was captured; the adapter acts on the status code and the captured `RateLimit-*` headers only.
- SRV and CAA are managed types (user's request, spec 03 §3). Hetzner supports 16 types; the others are leave-only.
- Write contract captured early on the user's OK (2026-10-28): `tools/contract-capture/hetzner_write.py` creates,
  changes, labels and deletes probe RRSets named `_hashi-test` / `*._hashi-test` of every managed type, plus the error
  cases; fixtures in `testdata/contracts/hetzner/write/`, nothing left in the zone. S3 builds the write side from these.
- PR A review (#28) fixed: a token that cannot see the stored zone no longer replaces the stored token (no zone chosen
  → nothing stored); types outside the provider's managed set are leave-only on the import table.

PR B — UI + deploy
- [x] Setup step 3 (provider, token, default TTL, zone auto-pick or choice, read-only validation) and the import
      table (shared component); step 3 built; E2E covers it against the fixtures.
- [x] `/dns`: Records (observed, origin, read-only in S2) and Import tab with decisions, from the approved mock.
- [x] Contract drift job (03 §2 point 4): `.gitea/workflows/contract-drift.yml` (manual dispatch + Mondays) and
      `make contract-drift`; compares JSON paths/types only. Repo secret `HETZNER_TOKEN` set (the existing project token, user's choice).
- [x] Deploy (user OK) and prod-proof: real zone listed, import table matches the zone, decisions survive a restart.

Decisions made in PR B:
- `-tags dev` builds serve recorded fixtures in-process when `HASHI_DEV_DNS_FIXTURES` is set (`make dev`, E2E; token
  `replay-token`, zone `example.com`); release builds contain no fixture code (`devDNSProviders` stub).
- Saving the import table sends a decision for every row (new rows as `leave`), so a saved table has no "new" rows.
- The default TTL field starts at 3600 s (no spec value).
- The import table hides "Remove" on RRSets Hashi owns other than imports (the server rejects it too).
- The drift diff merges pages per endpoint and does not look inside `labels`, so a grown zone or new labels are no
  drift; a path that appears or disappears, or a value type the fixtures never had, is (review #29: labels would
  otherwise have failed the job from S3 on).
- E2E uses a root domain outside the fixture zones, so it always walks the zone choice; auto-pick stays covered by the
  Go integration tests. On a re-import the table starts with "New only" on (03 §4).

**S1 — Setup, auth, vault** · done, signed off by the user 2026-10-27, deployed `8e5c1b5`.

Scope and gate: 11-delivery.md §3 row S1. Pages built on the approved mocks (`/setup`, `/login`, `/settings`, `/activity`)
following `docs/spec/10b-ui-system.md`.

External systems: none (WebAuthn via `go-webauthn`, tested with a Playwright virtual authenticator), so no contract capture.

PR A — backend (#25, merged)
- [x] Migration 00002: setup state, audit, admin user, bootstrap credentials, passkeys (with RP ID), sessions, secrets,
      recovery key, recovery links; sqlc queries; S0 noop job removed.
- [x] Vault (service mode): master key file required (`HASHI_MASTER_KEY_FILE`, `hashi keygen`), HKDF KEKs, per-secret DEK,
      key-check canary; recovery key (grouped base32 + checksum) wraps the master key; `hashi vault recover`.
- [x] Client-IP resolver (01 §8), audit log, settings framework (General, Security, Appearance), live-log ring.
- [x] Sessions (02 §3): sha256-only storage, strict IP binding, 4 h idle / 8 h absolute, reauth on the row, CSRF, cleanup
      job; scope table generated from `x-hashi-scopes/-session/-reauth`; 02 §6 tests over real HTTP.
- [x] Bootstrap (argon2id, printed once, private networks, `hashi bootstrap reset`); passkeys (RP ID = root domain, origin
      = admin domain); `hashi admin recover` link; setup steps 1, 2, 9, 11 with invariant 14.

PR B — UI + deploy (#26)
- [x] Live AppShell (session, sign-out, built pages only), CSRF in the client, reauth-and-retry, 401 → sign-in.
- [x] `/setup`, `/login`, `/login/recover`, `/settings` (Account, General, Security, Appearance), `/activity`.
- [x] `make test-e2e` + CI job: full setup with a virtual authenticator → passkey sign-in → account → reauth → sign-out →
      sign-in again → deleting the session's passkey ends it; console recovery link.
- [x] Compose: master key as a Docker secret; deploy.sh refuses to start without it; `make dev` uses a dev build + dev key.
- [x] Master key created on the host by the user; deployed `8e5c1b5` (user OK); prod-proof below.

Decisions made in S1:
- `-tags dev` builds (never in images) allow a `localhost` passkey origin and count later-slice setup steps as done, so
  `make dev` and E2E can run the whole flow before S5 brings HTTPS.
- Setup cannot finish in production builds until S2–S6 add steps 3–8; the deployed S1 only reaches step 9's prompt (no
  HTTPS for passkeys yet). That is expected.
- Deferred: `hashi vault rotate` (02 §5.2) → S12 with the backup drill; AAGUID → authenticator name (needs a metadata
  source) → later; settings without a consumer yet (sync interval → S2, internal URL → S5, theme already applied by the UI).

Spec changes before S1 (2026-10-27, from an external review, agreed with the user): S1 tests passkeys with a virtual
authenticator only (no HTTPS before S5; first real passkey in S5, RP ID = root domain); setup steps arrive with their
slices; Hashi issues and distributes certificates itself (03 §5.3, no `httpreq`); generated DNS is IPv4-only; forward-auth
fallback guarantees stated and proven in S4; decision-order table with overlap rule (06 §3.1); firewall rollback warns
on reverted foreign rules; security telemetry bounded by volume (01 §4.3, 06 §5.3–5.4).

**S0b — UI design for all features** · done, approved by the user 2026-10-27 (not deployed; the user reviewed screenshots
and the local preview). `docs/spec/10b-ui-system.md` + every page as a mock under `/design` (#24), four themes, contrast
tests, phone layout. Feature slices build the real page at the same path without `/design`.

**S0 — Walking skeleton** · done 2026-10-27, deployed `e7c0a30` to the Docker LXC (`hashi-v3`, ports 3340–3342).
PRs #17–#23: Go module + tools, `make check`, CI, redacting logs and `secrets.Value`, SQLite + goose migrations with
pre-migration backup, job runner, three listeners + `/healthz` `/readyz`, OpenAPI 3.1 → oapi-codegen strict server + TS
client, embedded SvelteKit SPA, multi-arch distroless image, release workflow, `make deploy/rollback/smoke`. Review fixed a
rollback bookkeeping bug and added missing invariant 6 / readyz tests.

Decisions made in S0:
- SvelteKit **2** kept as specified; Kit 3.0 (2026-10-01) skipped for now — the component ecosystem still targets Kit 2.
- shadcn-svelte init moved to S0b, where the design tokens are defined.
- Deploy targets live in gitignored `deploy/targets/<env>.env` (example in `deploy/targets.example/`).
- `make dev`, `make image`, `make deploy` exist now; `make test-integration`, `make test-e2e`, `make contract-capture` arrive with the slices that need them.

## Slices

| Slice | Status | Prod-proof (date · version · evidence) |
| --- | --- | --- |
| S0 Walking skeleton | **done** | 2026-10-27 · `e7c0a30` · `make deploy ENV=prod TAG=e7c0a30` on the Docker LXC next to V1: smoke from the LAN → `/readyz` `{"checks":{"database":"ok"},"status":"ok"}`, `/api/version` `{"commit":"e7c0a30","version":"e7c0a30"}`, SPA 200, public apps 200, public status 200; container `healthy`, uid 65532, read-only rootfs; restart keeps the DB (no re-migration); V1 and Gatus still up. CI green on the Gitea runners (#17–#23 and main); release published `e7c0a30` = `latest`, amd64 + arm64. |
| S0b UI design for all features | **done** | 2026-10-27 · `646888e` · approved by the user from screenshots and the local preview; not deployed (not part of this gate) · #24 |
| S1 Setup, auth, vault | **done** | 2026-10-27 · `8e5c1b5` · `make deploy ENV=prod TAG=8e5c1b5`: readyz ok, version `8e5c1b5`, SPA/public 200. Bootstrap login from the PVE host (LAN) → 200, `hashi.bootstrap` cookie (HttpOnly, SameSite=Strict, no Secure over HTTP), session bound to `10.0.4.1`, scopes `setup.manage` only. `docker restart hashi-v3` → same cookie still authenticated, bootstrap block not printed again. Cookie replayed from inside the Docker LXC (`10.0.4.106`) → `authenticated:false` + cookie cleared; afterwards rejected from `10.0.4.1` too; audit `session.revoked` `{bound_ip:10.0.4.1, reason:ip_mismatch}` observed ip `10.0.4.106`. DB + WAL copy: bootstrap password, master key (base64 and raw) absent; `secret_records` holds only AES-GCM ciphertext; session ids stored as 32-byte sha256; bootstrap hash argon2id. Passkeys: CI E2E with virtual authenticator green on `8e5c1b5` (no HTTPS before S5). Note: from the laptop, 10.0.4.0/24 is reached through kanae (NetBird route, SNAT), so the laptop appears as `10.0.4.1`. |
| S2 DNS read + import | **done** | 2026-10-28 · `04ff8c7` · `make deploy ENV=prod TAG=04ff8c7`: readyz ok, version `04ff8c7`, SPA/public 200. `hashi bootstrap reset` in the container, bootstrap login from the laptop (seen as `10.0.4.1`, scopes `setup.manage`); base settings with the real root domain. Step 3 with the real project token: the zone containing the root domain picked automatically (token sees 2 zones, the other ignored), provider `healthy`, 58 RRSets read; no write requests. Import table vs a direct read-only Hetzner read: 58 = 58 RRSets, 0 differences in name/type/TTL/values/protection; only NS and SOA leave-only. All 58 saved as `leave` (no takeover before the V1 cutover) → step 3 `done`; `docker restart hashi-v3` → 58 decisions still `leave`, 0 new, step 3 still done; V1 container untouched. Screenshot of the setup page shown to the user. Drift check: no drift against the live API. |
| S3 DNS write | **done** | 2026-10-28 · `94d60ad` · `make deploy ENV=prod TAG=94d60ad`: migration 3 → 4 with pre-migration backup, readyz ok, version `94d60ad`, SPA/public 200, container healthy, V1 untouched. Direct Hetzner read before: 58 RRSets, NS hash `d604b402d218`, SOA without serial `131e884f3662`, no `_hashi-test`. `hashi bootstrap reset`, setup login from the laptop (scope `setup.manage`); `POST /api/setup/dns/probe` → plan pending, high risk, 2 items (create + delete `_hashi-test` TXT), zone unchanged while pending; `POST …/probe/{id}/run` → applied in 14.6 s: "write access confirmed: _hashi-test TXT created, verified and deleted". Direct read after: 58 RRSets, NS and SOA (without serial) unchanged, no `_hashi-test`. Record path (spec row S3 as amended): `TestRealZoneRecordPath` against the real zone, same code (see PR B checklist): create applied + verified with owner label, delete pending until passkey reauth, then deleted and verified; zone unchanged. First background reconcile on the server (12:41 UTC): `dns.refresh` succeeded, "juzo.io: in sync" (no plan). Bootstrap credentials file deleted after use. |
| S4 SSH + Traefik | **done** | 2026-10-28 · `2868d0e` · `make deploy ENV=prod TAG=2868d0e`: readyz ok, version `2868d0e`, SPA/public 200. Dev targets rolled back to `clean` first. Setup over the API with a fresh bootstrap login (CSRF header): step 4 with a fresh Google Trust Services EAB key → account registered (health healthy); "issue now" → plan applied, "issued <root> (2 names, valid until 2027-01-26)", `_acme-challenge.<root>` TXT empty again at the authoritative server. Step 5 on `hashi-edge-test` (Alpine 3.23): host key read through Hashi (ECDSA SHA256:WAIE…aXc) equal to `ssh-keygen -lf` on the host, pinned; login with Hashi's own key; validation found Alpine/apk/OpenRC/amd64 and the existing binary; ownership confirmed; install plan (10 items, high risk, no download: v3.7.16 already there) approved and run: "10 change(s) applied … verified through the Traefik API; 1 certificate(s) served with the expected fingerprint; Traefik restarted". On the host: `hashi-traefik` started under OpenRC, in the default runlevel, process `/usr/local/bin/traefik --configFile=/etc/hashi/traefik/traefik.yaml`. Served on the edge's :443 for `<root>` and `app.<root>`: issuer Google Trust Services WR1, SAN <root> + *.<root>, sha256 AB:16:E5:8B…1B:9C = the file on disk. Health routes with Hashi up: open 200, closed 200 (the edge reaches Hashi's internal URL across sites). `docker stop hashi-v3` 18:53:39 → open 200 / closed 500 three times; started 18:54:23 → open 200 / closed 200. `logrotate -f` with Hashi's config: `access.log-20261028` created, the next request written to a fresh `access.log`, same Traefik PID (no restart). Install path on `hashi-edge-debian` (Debian 13, systemd, download + checksum, 11 items) and `hashi-edge-fedora` (Fedora 44, systemd, dnf, 12 items incl. logrotate install): applied and verified with the served fingerprint; services active + enabled, v3.7.16, `logrotate.timer` active, same certificate served, rotation reopens the access log. Live Traefik on both hosts and V1 untouched (up 2 days / still started / up 5 days). Found and fixed on main: the host-key hint named the ed25519 file while hosts offer their ECDSA key. |
| S5 HTTP services | **done** | 2026-10-29 · `3c4504e` · Deploys `24bef01` → `6f7d938` → `3c4504e` (smoke ok each; two gate findings fixed in between, see Log). Manual record `dev.hashi` A → the Alpine edge's internal address (user OK). Setup connection on `hashi-edge-test` (host key = `ssh-keygen -lf` ECDSA, pinned); its plan applied + verified. Step 7: certificate re-issued by Google Trust Services with `<root>`, `*.<root>`, `*.hashi.<root>`; admin route verified; `https://dev.hashi.<root>` from the laptop → 200, valid chain (WR1), `/api/version` = deployed build. SearXNG added with the setup session: a request with its real name to the edge → 200 (page + search), access log: router `svc-searxng@file` → internal upstream, forward auth → Hashi 204; public DNS unchanged. Step 8 + 9 by the user in a real browser (Firefox/Floorp, home LAN): audit 23:38:13 UTC `setup.https.proof` success from the user's address, 23:38:26 `auth.passkey.registered` (RP ID = root domain), 23:38:57 `vault.recovery_key.confirmed`; no session revoked. Setup state: steps 1–5, 7–9 done, 6 pending (S6). V1 untouched. |
| S6 Firewall + streams | — | |
| S7 Monitoring + public pages | — | |
| V1 cutover | — | |
| S8 AdGuard, Pulse, agent DNS | — | |
| S9 Edge SSO + rules | — | |
| S10 Security engine, WAF, CAPTCHA, blocklists | — | |
| S11 Scripts, themes, settings | — | |
| S12 Hardening + 1.0 | — | |

## Blocked

(Problems that survived two fix attempts: hypothesis, evidence, question for the human.)

## Unverified

(Review findings that were not reproduced. Not worked on unless promoted.)

- S6 PR B review (#37), acceptance points not built yet: public IP cross-check with Hashi's view (05 §2); per-host
  verdicts on the address page and per-set counts / "allow wider than /24" warning in the plan preview (05 §4.3);
  reconnect after a rescue window to report the rollback (05 §7 step 6); listening ports vs published ports (05 §8,
  matters for the takeover in PR C); `TestInvariant_20_ManualAllowBeatsDynamic` as a kernel test (meters are off).

- S4 PR B review (#33): on an Alpine host where Hashi has to install logrotate, rotation depends on `crond` running
  (`/etc/periodic/daily`); the plan neither checks nor starts it. The Alpine dev target runs `crond` (default runlevel).

- S4 PR A review (#32): the probe finds a Traefik binary on PATH and `/etc/traefik`, `/opt/traefik` only; a Traefik that
  runs as a service or container (ports 80/443 taken) is neither detected nor stopped after takeover, so
  `hashi-traefik` would fail to bind or run beside it (04 §2 steps 1, 4–6). Not tried on a real host; the gate's dev
  targets have no running Traefik (Alpine's binary is not started).

- S3 PR B review (#31): discarding a waiting entry-delete plan on `/changes` probably does not last: the entry is
  already gone from Hashi, so the next save or hourly sync proposes the same delete again. Not demonstrated; the spec
  does not say what Discard means there (undo the entry removal, or just "not now").

- S2 PR B review (#29): `/dns` "show generated" hides only `system` RRSets; the mock also counts `managed` as
  generated. No S2 row has either origin, so it cannot be observed yet — recheck when S3 creates records.

- S2 PR A review (#28): the zones pagination replay uses captures taken with `per_page=1` while the adapter sends
  `per_page=25` (the replay ignores `per_page`); `zone_get_by_name.json` is never replayed because the adapter does not
  call that endpoint. Test-shape gaps, no wrong behaviour observed.

- S1 review (#26): Activity → Live log keeps its last sequence number across a server restart (sequence restarts at 0),
  so an open tab could skip the startup lines after a restart. Read from code, not observed.
- S0 review: for a `v1.2.3` git tag the image tag is `1.2.3` (metadata-action `{{version}}`) but the binary reports
  `v1.2.3`, so `make deploy TAG=1.2.3` would fail its smoke version match (`deploy/deploy.sh` smoke). Not observed;
  only short-SHA deploys exist so far.

## Log

- 2026-10-29 — S6 PR A merged (#36; CI needed the E2E journey's timeout raised to 120 s: it outgrew Playwright's 30 s
  default on the runners). Host A pool was 85.6 % after the CI runs: `lxc-fstrim.service` run → 75 %. PR B: firewall
  contracts captured, folding/renderer/apply/rescue built and tested in containers.

- 2026-10-29 — S6 started with its entry gate. nft/NetBird coexistence holds (NetBird's own backend choice replayed:
  Hashi's table never switches it); plan B not needed. All rule shapes pass the real `nft` 1.1.3 in check mode on a
  container and the Debian dev target; the check on the first real host passed with the user's OK (nothing committed). Plan written (3 PRs).

- 2026-10-29 — S5 gate, step 8 with the user (Firefox/Floorp on the home LAN): the first link was refused ("nonce",
  probably altered on the way to the browser); with a fresh copied link the proof succeeded through the edge
  (`setup.https.proof` success from the user's LAN address), and so did a bootstrap login over HTTPS, but each session
  was revoked within the same second (`ip_mismatch`, observed `127.0.0.1`): the admin route's forward-auth call came
  through the edge's loopback hop with the visitor's cookie, and Hashi took the appended `127.0.0.1` for the client.
  Reproduced in `TestSetupStep8HTTPSProof`, fixed in `ba9ad85`/`3c4504e` (edge loopback hop skipped; forward auth never
  touches admin sessions). Server side of step 8 proven meanwhile by a proof sent from the edge (`127.0.0.1`).

- 2026-10-29 — Deploy `6f7d938` to prod: smoke ok. S5 gate so far: setup connection switched back to `hashi-edge-test`
  (Alpine; host key read through Hashi = `ssh-keygen -lf` ECDSA on the host, pinned; ownership confirmed); its plan
  (6 low-risk files: renewed certificate, services file with the admin route, security file with the edge token,
  logrotate) applied and verified. Step 7 done: `https://dev.hashi.<root>` from the laptop → 200, `/api/version`
  `6f7d938`, certificate Google Trust Services WR1, SAN `<root>`, `*.<root>`, `*.hashi.<root>`. SearXNG added as a
  service with the setup session (services.manage): applied + verified on the edge; a request with its real name sent
  to the edge → 200 (page and a search), edge access log: router `svc-searxng@file` → the internal upstream, forward
  auth `hashi-fa@file` → Hashi 204; its public DNS unchanged (old setup). Pocket ID's health check fixed by the user.

- 2026-10-29 — Deploy `24bef01` (S5: #34 + #35) to prod: smoke ok (readyz, version, SPA, public 200). S5 gate started:
  step 7 issued the certificate with `*.hashi.<root>` (Google Trust Services, 3 names) but its Traefik plan failed
  Verify on the Fedora dev edge, which Traefik had loaded 2 s later; with a second finding (general log not reopened
  after rotation) fixed in `6f7d938` (reproduced in the integration tests first). Gate continues after that deploy.

- 2026-10-28 — With the user's OK: host A got a daily `lxc-fstrim.timer` (runs `pct fstrim` for every running LXC;
  CI leaves ~15 GB of deleted blocks allocated per run, which caused the incident below), and the real zone got the
  manual record `dev.hashi` A → the Alpine edge's internal address (S5 gate admin name; outside Hashi's ownership,
  left on the import table; replaced at the S6 traffic move). S5 merged: #34, #35 (`24bef01`), CI and release green.

- 2026-10-28 — Incident (not caused by Hashi's code): the host A thin pool (local-lvm, 127 GB, about 232 GB promised
  to its volumes) filled at 23:00 CET while CI built S5 PR A on the Gitea runners in the Docker LXC (build cache 17 GB).
  The Docker LXC's filesystem went emergency read-only (Gitea, V1, hashi-v3, Vaultwarden, NetBird server and the other
  apps could not write); the Pangolin LXC lost writes to its Traefik access/stream logs only. Fixed with the user's OK:
  fstrim of the other LXCs, clean shutdown + fsck (repaired, then clean) + start of the Docker LXC, `docker builder prune
  -af` (17 GB), fstrim of the Docker LXC (42 GB returned): pool 100 % → 72 %, every container back (both Hashis
  healthy). The CI runs of that moment failed and were re-run. Open: the pool stays overcommitted and CI build caches
  grow without a limit.

- 2026-10-28 — Deploy `2868d0e` (S4: #32 + #33) to prod: smoke ok. S4 prod-proof passed (Slices table). The GTS EAB key
  created for the gate is now bound to Hashi's account; the ACME contact address is the one the old Traefik already uses
  with Google Trust Services.

- 2026-10-28 — S4: Proxmox snapshot `clean` of all three dev targets: `hashi-edge-debian` and `hashi-edge-fedora` (fresh OS + sshd, before any
  Hashi install); roll back to it before each install test and before the prod-proof. `hashi-edge-test` was tidied first
  (capture Traefik and stubs stopped, capture files removed) and snapshotted `clean` too; its Traefik v3.7.16 binary at
  `/usr/local/bin/traefik` stays on purpose so the existing-Traefik path is exercised.
- 2026-10-28 — S4 `internal/traefik`: Traefik runs as root under service name `hashi-traefik` (never collides with a
  distribution's `traefik` service; a dedicated user is a later hardening step). Health routers answer
  `/.well-known/hashi/health/{open,closed}` on every host name (priority 100000) with `ping@internal`. Traefik v3.7.16
  warns at start about `aliasHeadersStrategy` (entry points), `maxResponseBodySize` (forward auth) and encoded-character
  defaults: capture and decide in S5 with the service routers. Fail-closed forward auth to an unreachable (not refusing)
  Hashi host waits for the TCP timeout before the 500.
- 2026-10-28 — S4: two more temporary dev targets on host B, `hashi-edge-debian` (Debian 13) and `hashi-edge-fedora`
  (Fedora 44, template downloaded for it), same size, no autostart, sshd with the laptop key, reachable from the Hashi
  container. Fedora (dnf + systemd) added to the supported host OS list at the user's request.

- 2026-10-28 — S4 prep: the live Traefik instances on both hosts stay untouched (replaced, not adopted, 11-delivery.md §7).
  A temporary dev target exists for S4: a fresh unprivileged Alpine 3.23 LXC `hashi-edge-test` on host B (1 CPU,
  512 MB, 4 GB, no autostart, sshd with the laptop key, reachable from the Hashi container). It is not the production
  edge: base image, name, address and size of the real edge LXCs are decided with the user at the V1 cutover, then this one
  is deleted.

- 2026-10-28 — S3 done: #30 (backend) and #31 (UI) merged, deployed `94d60ad`, prod-proof passed (see the S3 row).
  README bar 5/15. Next: S4 (SSH + Traefik). Admin pages on the server need S5's HTTPS for passkeys.

- 2026-10-28 — S2 done: PR B (#29) merged, deployed `04ff8c7`, prod-proof passed (see the S2 row). README bar 4/15.
  Next: S3 (DNS write). Which existing records Hashi takes over is the user's decision at the V1 cutover.

- 2026-10-28 — S2 PR A (backend) done on `s2-dns-read`: migration 00003, `internal/dns` + Hetzner read adapter from the
  fixtures, refresh job, DNS and setup step 3 API. PR B (UI, deploy, prod-proof) next.

- 2026-10-28 — Security model side step done: mocks merged (#27), built code renamed (setup step 7), S2 next.

- 2026-10-28 — Security model signed off by the user (ADR 0002 accepted). Branch merged to `main`; mocks next.

- 2026-10-28 — Security model review round with the user: step decay, SSO satisfies CAPTCHA, no detect mode, SSH
  host link confirmed in the ADR, Pulse footprint targets. Still awaiting final sign-off.

- 2026-10-27 — Security model side step: the user's new model turned into spec on branch `security-model` (ADR 0002,
  05a new, 05/06/06a rewritten, Resource → Service). First firewall host survey: everything on iptables-legacy, NetBird too → S6 starts
  with an nft/NetBird coexistence gate, iptables+ipset as plan B. Awaiting sign-off.

- 2026-10-27 — S1 signed off by the user. README bar 3/15. S2 not started on the user's request.

- 2026-10-27 — Deployed `8e5c1b5` (S1) to the Docker LXC after the user created `master.key` on the host; smoke ok;
  S1 prod-proof checks passed (see the S1 row). Old S0 image can't run on the migrated DB (pre-migration backup kept).

- 2026-10-27 — S1 code: #25 (backend) merged, #26 (UI, E2E, deploy) open. Reviews reproduced and fixed: short idle
  limits expired active sessions; changing the root domain during setup stranded passkeys; admin pages stayed on a dead
  error after their session ended. The secret guard caught a real inventory IP used as a test fixture before any push;
  history was rewritten locally (nothing published). Lesson: fixtures use RFC 5737 / neutral private ranges only.

- 2026-10-27 — S0b approved by the user. Next: S1.
- 2026-10-27 — S0b mocks merged (#24): `docs/spec/10b-ui-system.md` + every page under `/design`. One PR of ~5,500
  hand-written lines (mostly page mocks reviewed by clicking), kept whole on purpose. Found: the shadcn-svelte nova
  components need the CLI's `tailwind.css` variants (`data-open` → `data-state="open"` …) that `init` normally writes;
  vendored as `web/src/lib/styles/shadcn-svelte.css`.
- 2026-10-27 — S0 gate passed (`e7c0a30` on the Docker LXC). PR sizing rule changed from ~500 lines to per-slice
  reviewable PRs (spec 11-delivery §2.7). V1 image workflow restored for `legacy/v1` (V1 code changes only); CI and
  release skip docs/V1-only changes. README progress bar added. Repo moved out of iCloud-synced `~/Documents`.
- 2026-10-27 — S0 code done (PRs #17–#23). Found along the way: `.gitea/workflows/v1-docker-build.yml` on main has
  only a trigger, no jobs (cut off in the restart commit), so V1 images can't be rebuilt — left alone because a rebuild
  would push `:old`, which production runs. The repo sits in iCloud-synced `~/Documents`; iCloud once renamed a
  build dir to `dist 2` mid-build, which got committed by mistake and reverted.
- 2026-10-27 — Restart. V2 attempt removed from history (backup bundle kept off-repo). New spec in `docs/spec/`.
