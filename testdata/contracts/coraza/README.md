# Coraza WAF plugin contract (captured 2026-11-02)

Local containers: `traefik:v3.7.16` with `experimental.plugins` loading `github.com/jcchavezs/coraza-http-wasm-traefik`
v0.3.0 (the only Coraza plugin in the catalog from the Coraza maintainer; latest release 2024-10-29), embedded OWASP CRS
4.0.0, and a small upstream that echoes what it received. Two routers: `full` (`SecRequestBodyAccess On`, 06 §9
`enabled`) and `hdr` (`Off`, `headers_only`). Files: `static.yaml`, `dynamic.yaml`, `requests.txt` (status per request),
`access_log.jsonl`, `traefik_log_matches.jsonl`. Client addresses replaced by 192.0.2.10.

Findings that change 06 §9 and §6.2:

1. **Matches are not in the access log.** Coraza writes each matched rule as a line of Traefik's general JSON log
   (`level` warn/error, `routerName`, `middlewareName`, and a `message` with `[client "…"] Coraza: …`, `[id "…"]`,
   `[severity "…"]`, `[uri "…"]`, `[unique_id "…"]`). The access log only shows the 403. So the tail must also follow
   `traefik.log`, which is rotated with `copytruncate` (04 §3).
2. **Uploads break in `enabled` mode.** A 3 MB PUT with `SecRequestBodyAccess On` never reached the upstream: the access
   log shows `DownstreamStatus 0`, `RequestContentSize 163840`, while curl saw an empty 200. Small bodies arrive intact.
   Known upstream issues (#8, #15 of the plugin's repository).
3. **`headers_only` does not let uploads or WebDAV through.** CRS's default policy refuses `PUT` (911100) and content types
   such as `text/plain` and `application/octet-stream` (920420) without looking at the body, so the 3 MB upload gets 403
   in both modes. Allowing them needs exclusions or `tx.allowed_methods` / `tx.allowed_request_content_type`.
4. A numeric `Host` header adds anomaly score (920350); with a real name, plain GET, small JSON and form POSTs pass;
   SQL injection in the query (both modes) and in a form body (`enabled` only) gets 403.
5. Loading the plugin needs Traefik to download it from plugins.traefik.io at start (outbound HTTPS from every edge).

Second try (same day): `SecRequestBodyInMemoryLimit 13107200` in `enabled` mode turns the lost 3 MB upload into a
`500` with a broken pipe at the upstream, so large bodies stay broken there. In `headers_only`, widening
`tx.allowed_methods` (rule 900200) lets `PROPFIND` through to the upstream; content types are matched against a list
(`|*/*|` is no wildcard), so 920420 still refuses `application/octet-stream` until it is listed or excluded.

Third try, plugin loading (`plugin_load.txt`, `plugin_traefik.yml`):

6. **`experimental.plugins` downloads at every start.** Traefik clears `plugins-storage/` in its working directory and
   fetches the plugin from plugins.traefik.io on each start, even with an earlier copy. Without network every router
   using the middleware is `disabled` (API `status`, `error`) and answers 404; other routers and `/ping` stay fine, so a
   ping-only health check misses it.
7. **`experimental.localPlugins` needs no network.** The catalog zip (same bytes as the GitHub release asset) unpacked
   to `<working directory>/plugins-local/src/<moduleName>/` loads offline, with the full CRS, from any working directory.
8. Only lines with `Access denied` are blocking decisions (anomaly score evaluation, 949110); the `Warning` lines before
   them are single rule contributions, also for harmless requests (e.g. 920350 for a numeric `Host`). All lines of one
   request share `unique_id`.
