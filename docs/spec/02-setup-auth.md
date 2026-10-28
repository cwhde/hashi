# 02 — Setup, authentication, admin sessions, secrets vault

## 1. Setup state machine

Setup is a persisted, resumable state machine (`setup_state`). Each step records `status` (pending / in_progress /
done / failed), its validated inputs, and the last error. A step can only be marked done after its validation
**succeeded** — a failed SSH check or failed provider apply never advances the wizard.

| # | Step | Required | Completes when |
| --- | --- | --- | --- |
| 1 | Bootstrap login | yes | Valid bootstrap credentials from an allowed network |
| 2 | Base settings | yes | Root domain, admin domain, internal Hashi URL, sync interval, public page toggles, theme saved and validated |
| 3 | DNS provider | yes | Token validated read-only, zone resolved, import decisions saved |
| 4 | Certificate provider | yes | ACME email + EAB stored, ACME account registered, DNS provider bound |
| 5 | Traefik connection | yes | SSH validated + host key pinned, OS detected, paths confirmed, ownership confirmed |
| 6 | Linux firewall host | yes | SSH validated, tools detected, subnets valid, linked to a Traefik connection |
| 7 | Hashi system service | yes | DNS + Traefik + firewall plans applied and verified for the admin domain |
| 8 | HTTPS reachability | yes | Browser reaches Hashi via `https://<admin domain>`; verified server-side (below) |
| 9 | Passkey + recovery key | yes | Passkey registered, recovery key shown and confirmed by re-entry of a checksum segment, test unlock succeeded |
| 10 | Optional integrations | no | Any of: OIDC SSO, AdGuard, notifications, MaxMind, Cap CAPTCHA, blocklist feeds, internal agent DNS, dashboard widgets |
| 11 | Complete | — | Bootstrap credentials destroyed; bootstrap sessions revoked |

Steps are built by the slice that owns their subsystem (11-delivery.md §3): S1 builds steps 1, 2, 9 and 11; steps 3–8 and
10 arrive with S2–S5 and later. Until a step exists the wizard shows it as "arrives in a later slice" and cannot complete.
Passkeys need HTTPS (or `localhost`), which Hashi only has from S5, so S1 tests passkey registration and login with an
automated virtual authenticator; the first real passkey is registered during the full setup run in S5.

Step 8 verification: Hashi issues a one-time nonce (5 minutes) to the bootstrap session, and the browser opens
`https://<admin domain>/setup?https=<nonce>`. The SPA there posts the nonce to `/api/setup/https/proof`; the server
accepts only if the request arrived through a Hashi-managed Traefik (edge token, 01-platform.md §8) with the expected
`Host` and `X-Forwarded-Proto: https`. Acceptance marks the step done and hands the setup over to the HTTPS origin: a
new bootstrap session (cookie `Secure`) bound to the client IP seen there, because passkeys (step 9) need that origin.
Forwarded headers without the edge token are ignored.

### 1.1 Bootstrap access

On first boot with no completed setup:

1. Generate a random username (8 chars) and password (24 chars, base58).
2. Print them **once** to stdout (container logs) in a clearly delimited block.
3. Store only an argon2id hash.
4. Allow bootstrap login only from: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `127.0.0.0/8`, `::1/128`, `fc00::/7`
   (setting `bootstrap.allowed_cidrs`), resolved via the shared client-IP resolver (01-platform.md §8).
5. Bootstrap over plain HTTP on a private address is allowed: the bootstrap session cookie is issued without `Secure`
   only when the request is plain HTTP from an allowed range. After step 8 every cookie is `Secure`.
6. After restart before completion, the same credentials remain valid (hash persisted); a CLI command
   `hashi bootstrap reset` regenerates them.

Bootstrap sessions carry only `setup.*` scopes and are revoked when setup completes.

### 1.2 Base settings

Root domain, admin public domain, internal Hashi URL/IP + port (no hardcoded `8080`), default sync
interval (1 h), public dashboard enabled (on), public status enabled (on), theme, optional asset choices.
Domains are validated as FQDNs under the root domain; the internal URL must be an absolute `http(s)` URL.

### 1.3 DNS provider step

See 03-dns.md §2–3. Collect provider type (Hetzner first), API token, default TTL. Hashi manages exactly **one zone**:
the zone containing the root domain from step 2 is selected automatically; if the token sees several zones and none
matches, the user picks one from the list. Other zones the token can see are ignored. Validate read-only (list
zones, read RRSets). Write capability is only confirmed if the user explicitly approves a probe plan that creates and
deletes a `_hashi-test.<zone>` TXT probe RRSet (03-dns.md §2). Import table: see 03-dns.md §4.

### 1.4 Certificate provider step

Hashi itself is the ACME client (03-dns.md §5.3). Initial support: Google Trust Services ACME with External Account
Binding, DNS-01 via Hetzner. Collect ACME email, EAB
key ID, EAB HMAC (secret), DNS provider binding, propagation delay, resolver list. Secrets go to the vault. Let's
Encrypt is offered as a second ACME directory option (no EAB) because it costs nothing to support and is a useful fallback.

### 1.5 Traefik and firewall steps

See 04-traefik-services.md §5 and 05-firewall.md §2. Both collect SSH host, port, username, password or private key
(+ passphrase); both pin the SSH host key on first contact after showing the fingerprint to the user.

### 1.6 System service

Setup creates the Hashi system service: admin domain → internal Hashi URL, TLS on, forward auth with Stage A and default fallback `none` (06-edge-security.md §4.5), origin
`system`, delete forbidden, editable only via Settings → General.

## 2. Admin authentication

Hashi has exactly one admin user.

- After setup, login requires a passkey (WebAuthn, user verification `required`, resident key `preferred`).
- WebAuthn RP ID = the **root domain** (step 2), not the admin domain, so passkeys survive a change of the admin domain
  (e.g. a temporary name before the V1 cutover, then the final one). Hashi still accepts assertions only when the
  signed origin is exactly `https://<admin domain>`; assertions made on any other subdomain are rejected.
- Multiple passkeys may be registered; each has a name, created-at, last-used-at and AAGUID-derived label.
- Deleting the last passkey is forbidden. Deleting a passkey revokes sessions authenticated with it.
- Recovery key (§5.3) is for vault recovery; a separate **recovery login** is possible only from the console:
  `hashi admin recover` prints a one-time, 15-minute, private-network-only login link that lets the admin register a new passkey.
  The token sits in the URL fragment (never in server or proxy logs). The link opens a `recovery` session whose only scope is
  `recovery.passkey`; it ends as soon as one passkey is registered.
- The root domain is fixed after setup, because every passkey is bound to it. During setup, changing the root domain in
  step 2 removes passkeys registered for the old one and step 9 starts over; completion requires a passkey for the current RP ID.
- Adding another passkey (Settings → Account) requires reauthentication, like other privilege changes (§3.2).
- Recent reauthentication = a fresh passkey assertion within the current session; valid 5 minutes.

Reauthentication is required for: destructive operations, secret reveal/replace, script save/run, firewall/DNS prune,
permanent bans, changes to IP sets bound to firewall positions, global firewall allow/block bindings, Cap settings, internal agent DNS
domain change, trusted-proxy change, revoking other sessions, vault mode change.

## 3. Admin sessions

### 3.1 Model (`admin_sessions`)

- `id`: 256-bit random; the cookie carries it, the DB stores only `sha256(id)`.
- `auth_method`: `bootstrap` | `passkey` | `recovery`; `credential_id` for passkey sessions.
- `bound_ip`: canonical client IP at issuance.
- `scopes`: explicit list.
- `created_at`, `last_seen_at`, `idle_expires_at`, `absolute_expires_at`.
- `reauth_at` (nullable), `revoked_at` (nullable), `revocation_reason` (nullable).
- `user_agent_hash` (salted, for correlation only).

Cookie: `__Host-hashi.session` (`Secure`, `HttpOnly`, `SameSite=Strict`, `Path=/`), except the bootstrap-over-HTTP
case in §1.1 which uses `hashi.bootstrap`.

### 3.2 Rules

- Every login creates a new session. Logging in from another IP creates another, independently bound session.
- **Strict IP binding:** every request compares the canonical request IP with `bound_ip` by exact equality. Mismatch →
  reject as unauthenticated, revoke the session, clear the cookie, drop its in-memory material, audit stored and observed IPs (never the token).
- **Idle lifetime:** 4 h max (setting may lower it, minimum 5 min). Activity extends `idle_expires_at`, capped at `absolute_expires_at`.
  Activity is persisted every 5 minutes (every quarter of the idle time when that is shorter, so steady activity always
  lands before the deadline), and validation never grants more than the configured idle time.
- **Absolute lifetime:** 8 h max from issuance. Nothing extends it — not activity, not reauth, not vault unlock.
- **Reauth:** stored on the session row (survives restarts), valid 5 min, never changes `bound_ip` or `absolute_expires_at`,
  never revives a revoked/expired session.
- Expired, revoked, malformed, unknown → fail closed.
- Privilege changes rotate the session ID or update scopes only after successful reauth.

### 3.3 Scopes

| Scope | Grants |
| --- | --- |
| `admin.read` | read protected admin state |
| `admin.write` | ordinary mutations |
| `settings.manage` | application and security settings |
| `secrets.manage` | reveal, add, replace, remove secrets; vault state |
| `sync.apply` | apply sync plans |
| `firewall.apply` | apply or roll back firewall state |
| `scripts.manage` | create, modify, execute scripts |
| `security.manage` | IP sets, rule sets, dynamic entries (ban/challenge/remove), feeds, CAPTCHA, engine settings, session revocation |
| `setup.*` | bootstrap-only setup scopes (`setup.manage`) |
| `recovery.passkey` | register one replacement passkey (console recovery sessions only) |

Passkey sessions receive all non-setup scopes. Unknown scopes grant nothing. Scope and reauth checks are cumulative.

### 3.4 API

- `GET /api/auth/session` — current session status, scopes, expiries, reauth validity.
- `GET /api/auth/sessions` — active sessions: masked correlation ID, bound IP, created, last activity, absolute expiry, current marker.
- `DELETE /api/auth/sessions/{id}` — revoke one.
- `POST /api/auth/sessions/revoke-others` — requires reauth + `security.manage`.
- `POST /api/auth/reauth/options`, `POST /api/auth/reauth` — passkey reauth ceremony.
- `POST /api/auth/logout` — revokes server-side first, then clears the cookie.
- Passkey: `POST /api/auth/passkeys/register/options|verify`, `POST /api/auth/passkeys/login/options|verify`,
  `GET /api/auth/passkeys`, `PATCH|DELETE /api/auth/passkeys/{id}`.

### 3.5 Audit

Session issued, renewed (at the persisted cadence), idle-expired, absolute-expired, IP-mismatch rejection, logout,
manual revoke, bulk revoke, scope failure, reauth success/failure. Never log cookie values, raw session IDs, passkey
assertions or vault material; correlate with a salted hash prefix.

### 3.6 Cleanup

A job removes expired sessions and revoked sessions older than the audit-retention interval. Cleanup failure can never
make a session valid again (validity is computed from timestamps, not from row presence).

## 4. CSRF

Unsafe admin methods require header `X-Hashi-CSRF` equal to the session-bound CSRF token (delivered in
`GET /api/auth/session`). `SameSite=Strict` is defence in depth, not the mechanism. Edge, challenge, public and Pulse
endpoints are excluded and use their own protections.

## 5. Secrets vault

### 5.1 Secret classes

| Class | Examples | Decryptable by |
| --- | --- | --- |
| Service | SSH credentials, DNS tokens, AdGuard credentials, notification tokens, ACME EAB, Cap secret, OIDC client secrets, MaxMind key, script secret env | the service key (unattended) |
| Admin-only | none by default; reserved for secrets the user marks "never needed unattended" | the admin key (requires an unlocked admin session) |
| Operational | cookie/CSRF HMAC keys, Pulse token pepper | derived from the master key |

### 5.2 Envelope encryption

- Master key: 32 random bytes supplied by the deployment as a file (Docker secret `hashi_master_key`, path setting
  `HASHI_MASTER_KEY_FILE`). Hashi refuses to start in normal mode without it, and `hashi keygen` prints a fresh one.
- KEKs: `HKDF-SHA256(master, info="hashi/kek/<class>/v1")`.
- Each secret: random 256-bit DEK; payload sealed with AES-256-GCM (random 96-bit nonce, AAD = secret ID + class + version);
  DEK wrapped with the class KEK (AES-256-GCM). Ciphertext, wrapped DEK, nonces, key version stored in `secret_records`.
- Secrets are typed in Go (`secrets.Value`) — the type's `String`, `MarshalJSON` and `slog.LogValue` all render `[redacted]`,
  so accidental logging or serialization cannot leak them.
- API never returns secret values except `POST /api/vault/secrets/{id}/reveal` (reauth + `secrets.manage`, audited).
- Key rotation: `hashi vault rotate` re-wraps all DEKs under a new master key version.

### 5.3 Recovery key

At setup step 9 Hashi generates a 256-bit recovery key, shows it once as grouped base32 with a checksum, and requires
confirmation. It wraps a copy of the master key (stored in `recovery_key`). Use: after restoring a DB backup on a host
without the original master key, the admin runs `hashi vault recover` (console), enters the recovery key, and Hashi
writes the recovered master key to the configured secret path (or prints it for the operator to install).

### 5.4 Vault modes

| Mode | Behaviour |
| --- | --- |
| **Service mode** (default) | Master key from deployment secret; background sync runs after restart without anyone logged in. |
| **Strict mode** (optional) | Master key is not stored on disk; after each restart Hashi stays locked until the admin unlocks it with a passkey that supports the WebAuthn **PRF** extension or with the recovery key. Background jobs that need secrets pause while locked (critical health warning; public pages keep serving last known state). |

Setup explains the trade-off: strict mode protects secrets against a stolen disk/DB/backup but cannot reconcile unattended after a restart.

**Changed from V2:** V2 wrapped an "admin vault" with PRF output for every install while simultaneously requiring
unattended decryption via a Docker secret — so PRF protected nothing the service key couldn't already decrypt, and the
PRF path never worked. Now: one master key, one clear default, and the PRF/passkey-only
protection survives as the explicit opt-in strict mode (delivered in slice S12).

### 5.5 Locked behaviour

While secrets are unavailable: public pages render last known state; monitors needing no secrets continue; provider
syncs pause; the last applied firewall state stays active on hosts; the UI shows the lock state on every page header.

## 6. Session tests and acceptance (verbatim from the V2 session addendum)

### 6.1 Unit Tests

Add tests for:

* Canonical IPv4 and IPv6 comparison.
* IPv4-mapped IPv6 normalization.
* Idle expiry calculation.
* Absolute expiry enforcement.
* Activity extension capped by absolute expiry.
* Scope-to-endpoint mapping.
* Unknown and missing scope rejection.
* Five-minute reauthentication calculation.

### 6.2 Integration Tests

Add tests proving:

* A token works from its issuance IP.
* The same token is rejected and revoked from a different IP.
* Two logins from different IPs create two independently valid tokens.
* Activity extends idle expiry but never absolute expiry.
* A token is rejected after four hours of inactivity.
* A continuously active token is rejected after eight hours.
* Recent reauthentication does not reset absolute expiry.
* Revocation takes effect before cookie expiry.
* Missing scopes reject otherwise authenticated requests.
* Application restart does not lose session or recent-reauthentication state.

### 6.3 Security Tests

Add tests proving:

* Untrusted forwarding headers cannot select the bound IP.
* Trusted proxy forwarding produces the same canonical IP during issuance and validation.
* Raw tokens are absent from audit events and application logs.
* Expired, revoked, malformed, and unknown sessions fail closed.

### 6.4 Acceptance criteria

The feature is complete when:

1. Every newly issued admin token has an authoritative server-side session record.
2. Each token is strictly usable only from the exact canonical IP to which it was issued.
3. Logging in from another IP creates a separate token rather than moving an existing token.
4. Idle lifetime is no more than four hours.
5. Absolute lifetime is no more than eight hours from original token issuance.
6. Activity and recent reauthentication never extend the absolute deadline.
7. Every protected admin endpoint requires an appropriate explicit session scope.
8. High-risk endpoints still require five-minute recent passkey reauthentication.
9. Logout and manual revocation invalidate server-side state immediately.
10. Vault material is removed when its session is revoked, expires, or fails IP validation.
11. Trusted-proxy handling cannot be bypassed with arbitrary forwarding headers.
12. Session lifecycle events are auditable without exposing bearer credentials.
