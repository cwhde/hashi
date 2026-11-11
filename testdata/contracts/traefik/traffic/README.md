# Traefik traffic options and user middlewares (captured 2026-11-11)

- Remote software: Traefik v3.7.16 (official image `traefik:v3.7.16`), throwaway local container, no homelab host
  touched. Upstream: a stub on 127.0.0.1:9000 that answers with the Host and every request header it received
  (`/slow` waits 3 s). Requests sent with BusyBox `wget` from inside the container.
- Docs: https://doc.traefik.io/traefik/reference/routing-configuration/http/load-balancing/service/ (`passHostHeader`),
  https://doc.traefik.io/traefik/reference/routing-configuration/http/load-balancing/serverstransport/ (`forwardingTimeouts`),
  https://doc.traefik.io/traefik/reference/routing-configuration/http/middlewares/headers/,
  https://doc.traefik.io/traefik/reference/install-configuration/entrypoints/ (`forwardedHeaders`).
- Domains are `*.example.test`, addresses documentation ranges. The container's host name is replaced by `edge-host`.

| File | What |
| --- | --- |
| `static_capture.yaml` | entry points: `web` (:8080, no trusted forwarders) and `trusted` (:8082, every forwarder trusted) |
| `dynamic_capture.yml` | one router per option: host kept, `passHostHeader: false`, Host set by a headers middleware, `X-Forwarded-For` / `X-Real-Ip` removed, extra request/response headers, a transport with `forwardingTimeouts` |
| `user_middlewares_capture.yml` | a user file with `ipAllowList` and `buffering`, loaded next to the other file |
| `requests.txt` | every request and what the upstream received, plus status lines |
| `api_rawdata_unknown_field.json`, `log_unknown_field.jsonl` | the user file with an unknown middleware type |
| `api_rawdata_duplicate_name.json`, `log_duplicate_name.jsonl` | the user file defining a name another file already defines |

Findings that shape the implementation (04 §1.6, §4):

1. Host: by default the upstream receives the public name. `passHostHeader: false` sends the server URL's host as
   written, with the port (`127.0.0.1:9000`). A headers middleware with `customRequestHeaders: {Host: <name>}` sets
   the Host header to that name. `X-Forwarded-Host` keeps the public name in all three cases.
2. Traefik adds `X-Forwarded-For`, `X-Real-Ip`, `X-Forwarded-Host`, `-Port`, `-Proto` and `-Server` itself. On an
   entry point that does not trust the sender, client-sent `X-Forwarded-For` / `X-Real-Ip` are replaced by the
   client's address; on a trusted one the client's `X-Forwarded-For` is kept and the address appended, and its
   `X-Real-Ip` is passed on.
3. `customRequestHeaders: {X-Forwarded-For: ""}` removes the header completely (the proxy does not add it back); the
   same for `X-Real-Ip`. An empty value removes any request header, including one the client sent; an empty
   `customResponseHeaders` value removes a response header.
4. `forwardingTimeouts.responseHeaderTimeout: 1s` answers **504** after one second when the upstream is slower;
   faster requests through the same transport pass. `dialTimeout` and `idleConnTimeout` load without error.
5. **One unknown key in any dynamic file makes the file provider load none of its files**: `/api/rawdata` has no
   routers, middlewares or services of the provider, and the log says `field not found, node: notAMiddleware`.
   A user file must therefore be checked strictly before it is written; a broken user file takes every service down.
6. A middleware name that another file already defines is **skipped with a warning** ("HTTP middleware already
   configured, skipping"): the first definition wins, the API shows no error for it. Verify cannot see this, so user
   names must never collide with Hashi's own names.
7. `ipAllowList` from another address answers 403; `buffering.maxRequestBodyBytes` answers 413 over the limit.
