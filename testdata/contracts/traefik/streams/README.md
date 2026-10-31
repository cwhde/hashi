# Traefik TCP/UDP streams (captured 2026-10-29)

- Remote software: Traefik v3.7.16 (official image `traefik:v3.7.16`), local disposable container, no real traffic.
- Docs: https://doc.traefik.io/traefik/reference/routing-configuration/tcp/router/rules-and-priority/,
  https://doc.traefik.io/traefik/reference/routing-configuration/tcp/service/,
  https://doc.traefik.io/traefik/reference/routing-configuration/udp/service/,
  https://doc.traefik.io/traefik/reference/install-configuration/entrypoints/.
- Upstreams are busybox `nc` echo servers on 127.0.0.1. Nothing real to sanitize.

| File | What |
| --- | --- |
| `static_streams.yaml` | static config Traefik accepted: `tcp-<port>` entry points as `":<port>/tcp"`, `udp-<port>` as `":<port>/udp"` |
| `dynamic_streams.yml` | TCP router `HostSNI(\`*\`)` on one entry point with a two-server load balancer and `proxyProtocol.version: 2` (accepted, no deprecation warning); UDP router + load balancer |
| `dynamic_streams_plain.yml` | plain TCP router without PROXY protocol (traffic test) |
| `dynamic_streams_broken.yml` | a TCP router naming a missing service, a UDP router on an entry point that does not exist |
| `api_rawdata_streams.json` | `GET /api/rawdata` with all of the above loaded: streams appear under `tcpRouters`, `tcpServices`, `udpRouters`, `udpServices`, names suffixed `@file`; TCP routers report `priority: -1`; broken routers have `status: "disabled"` and `error: [...]` (`the service "missing@file" does not exist`; `entryPoint "udp-15499" doesn't exist`, `no valid entryPoint for this router`); TCP services carry `serverStatus` per server, UDP services do not |
| `api_overview_streams.json` | `GET /api/overview`: separate `tcp` and `udp` totals (UDP has no middlewares) |

Behaviour observed: a byte stream sent to `tcp-15434` came back from the echo upstream through Traefik, and a UDP
datagram to `udp-15433` was echoed back. A client that closes its sending side right after writing (busybox
`echo | nc -w`) got no answer over TCP: test clients keep the connection open until they have read the reply.
The log reports a router on an unknown entry point as `EntryPoint doesn't exist` (level error).
