# Cap Standalone contract (captured 2026-11-02)

Throwaway local containers: `tiago2/cap:3.1.15` with `valkey/valkey:9.0.0-alpine`; the widget `@cap.js/widget@0.1.58`
in a real browser solved one challenge. Site key, secret and token are replaced. Docs:
https://capjs.js.org/guide/standalone/. No Cap instance runs in the homelab yet.

| File | What |
| --- | --- |
| `siteverify.txt` | `POST <base>/<site key>/siteverify` answers, in the order sent: wrong secret, success, the same token again, another site key in the path, malformed token, no `response`, not JSON, unknown site key |

Findings:

1. Success is `200 {"success":true}`; every failure has `success:false` and an `error` string, with statuses 400, 403,
   404 and even 500 (a body without `response` crashes the handler). Hashi treats only `200` with `success:true` as
   solved, and any other answer as "not solved"; a connection error or timeout as "Cap unavailable".
2. A token is single use: the second check of the same token answers `404 "Token not found"`. A wrong secret is checked
   before the token is consumed, so a bad secret does not burn the visitor's token.
3. The token is `<site key>:<id>:<value>`; a token of another site key is refused when the site key is in the path.
4. The widget calls `POST <base>/<site key>/challenge` and `POST <base>/<site key>/redeem` (CORS preflight on redeem);
   redeem answers `{"success":true,"token":…,"expires":<ms>}`, about two hours ahead. The widget runs its work in
   `blob:` workers, so the challenge page's CSP needs `worker-src blob:` and `connect-src` for Cap's public base.
5. Site keys and secrets are created in Cap's dashboard (or `POST /server/keys` with a dashboard session); the secret
   starts with `sk-`. Protocol `hashwx` is the default for new keys; the capture used `sha256-pow` to solve quickly.

Second capture (same day, Cap 3.1.15 started from Hashi's own `deploy/compose/compose.yaml`, profile `cap`):

6. Settings read by the image (`src/*.js`): `ADMIN_KEY` (required, at least 12 characters, or the process exits),
   `REDIS_URL`/`VALKEY_URL`, `ENABLE_ASSETS_SERVER`, `WIDGET_VERSION`, `WASM_VERSION` (default `latest`), `CACHE_HOST`
   (default jsDelivr), `RATELIMIT_IP_HEADER`, `SERVER_PORT` (3000), `CORS_ORIGIN`. With the assets server on, Cap fetches
   the pinned widget and WASM from jsDelivr once (then hourly) and serves `/assets/widget.js` (200, 50634 bytes for
   widget 0.1.58), `/assets/cap_wasm_bg.wasm`, `/assets/cap_wasm.js`, `/assets/hashwx.wasm` (WASM 0.0.8). `/` is the
   dashboard (200).
7. Dashboard API: `POST /auth/login {"admin_key":…}` → `200 {"success":true,"session_token","hashed_token","expires"}`;
   later calls send `Authorization: Bearer base64({"token":session_token,"hash":hashed_token})` (or `Bot <id>_<token>`
   for API keys). `POST /server/keys {"name","protocol"}` → `200 {"siteKey":<10 hex>,"secretKey":"sk-…"}`; without
   authorization `401`.
8. Widget 0.1.58: `<cap-widget data-cap-api-endpoint="<base>/<site key>/">`, events `solve` (token in
   `event.detail.token`), `error`, `reset`, `progress`. It loads its WASM from jsDelivr unless the page sets
   `window.CAP_CUSTOM_WASM_URL` and `window.CAP_CUSTOM_HASHWX_URL` (Hashi points them at Cap's `/assets/`); pako comes
   from jsDelivr only for keys with instrumentation and a browser without `DecompressionStream`. `CAP_SCRIPT_NONCE`
   passes a CSP nonce to scripts it adds.
