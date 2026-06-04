# Research Resources for Addendum Implementation

This file captures external references found during addendum planning. Re-check these links immediately before implementation because blocklist formats, rate limits, Cap APIs, and Gitea runner behavior can change.

## Research Workflow

Use primary sources first:

- Maintainer documentation.
- Official API references.
- Direct feed metadata pages.
- Project repositories only when official docs are incomplete.

Use secondary sources only to discover leads or implementation examples. Do not treat blog/forum snippets as authoritative for policy, formats, licensing, or rate limits.

Before coding against an external feed or API:

1. Open the source documentation page and the direct URL.
2. Save the observed content type, format, comments/header style, update cadence, and rate-limit/license notes in the implementing PR.
3. Add parser fixtures from small synthetic examples, not copied full feeds.
4. Add one integration-style test with a local fake HTTP server that simulates redirects, ETag, Last-Modified, timeouts, oversize bodies, and malformed data.
5. Keep all production network fetches behind explicit user opt-in, timeout, size limits, and SSRF validation.
6. If an online researcher or sub-agent is used, scope it to current docs only and require citations plus direct URLs. The implementing agent must verify every recommendation against the primary source before coding.

## Blocklist Feeds

The addendum's recommended feeds should be seeded disabled by default. Each feed needs metadata, false-positive warning text, default format config, and a fetch interval that respects the source's stated guidance.

| Feed | Docs | Direct URL Candidates | Notes |
| --- | --- | --- | --- |
| Feodo Tracker Botnet C2 recommended IP list | https://feodotracker.abuse.ch/blocklist/ | https://feodotracker.abuse.ch/downloads/ipblocklist_recommended.txt, https://feodotracker.abuse.ch/downloads/ipblocklist_recommended.json | Maintainer says the recommended list is active/recent botnet C2 infrastructure and has lower false-positive risk than broader IoC lists. It is generated frequently. |
| Feodo Tracker Botnet C2 full IP list | https://feodotracker.abuse.ch/blocklist/ | https://feodotracker.abuse.ch/downloads/ipblocklist.txt, https://feodotracker.abuse.ch/downloads/ipblocklist.json | Consider as custom/advanced, not the default suggested Feodo seed, unless the user explicitly wants broader coverage. |
| Spamhaus DROP IPv4 | https://www.spamhaus.org/blocklists/do-not-route-or-peer/ | https://www.spamhaus.org/drop/drop_v4.json | Spamhaus currently documents JSON as the preferred form. Respect their minimum automated download interval. |
| Spamhaus DROP IPv6 | https://www.spamhaus.org/blocklists/do-not-route-or-peer/ | https://www.spamhaus.org/drop/drop_v6.json | Same handling as DROP IPv4, with IPv6 CIDR parser coverage. |
| Spamhaus ASN-DROP | https://www.spamhaus.org/blocklists/do-not-route-or-peer/ | https://www.spamhaus.org/drop/asndrop.json | Forward-auth only by default. Do not expand ASNs to firewall IP ranges in v1. |
| DShield recommended block list | https://www.dshield.org/hpbinfo.html | https://feeds.dshield.org/block.txt, https://www.dshield.org/block.txt | Current feed is tab-delimited /24 data with comments/header. Treat as higher false-positive risk than precise C2 feeds. |
| FireHOL Level 1 | https://iplists.firehol.org/?ipset=firehol_level1 | https://iplists.firehol.org/files/firehol_level1.netset, https://raw.githubusercontent.com/firehol/blocklist-ipsets/master/firehol_level1.netset | Composite list intended to be broadly safe, but still user-selected only. Parser should support `.netset` and comments. |

## Blocklist Parser and Fetch Research

Primary docs to consult:

- FireHOL ipset format/background: https://firehol.org/guides/ipset/
- Docker/private network ranges for SSRF deny rules: prefer IANA and RFC references where practical when implementing exact ranges.
- .NET `IPAddress` and `HttpClient` docs for parsing and redirect handling.

Implementation guidance:

- Keep feed definitions in code/data with docs URL and direct URL separate.
- Store a source content hash per successful fetch.
- Store every failed fetch run without deleting last known good entries.
- Parse into normalized subject values before touching effective decisions.
- Use a deny-by-default HTTP fetcher that resolves DNS and validates every redirect target and final remote IP.

## Cap CAPTCHA

Primary docs:

- Cap site and positioning: https://trycap.dev/
- Quickstart, widget, and siteverify flow: https://trycap.dev/guide/
- Standalone API: https://trycap.dev/guide/standalone/api
- Compliance/privacy notes: https://trycap.dev/guide/compliance.html
- Self-hosting docs for user reference only: https://cap.so/docs/self-hosting

Important implementation notes:

- Hashi integrates with an existing Cap Standalone instance. It must not deploy Cap.
- Cap site keys are public configuration.
- Cap key secrets are server-side secrets and must be stored with the service-sync vault.
- The Cap dashboard `ADMIN_KEY` is not the siteverify secret and must not be used for challenge verification.
- Tokens are single-use; verify once and then apply Hashi's own state transition.
- In high-security deployments, prefer self-hosting/pinning the Cap widget asset instead of using a floating CDN version.

## Gitea and Docker CI/CD

Primary docs:

- Gitea act runner cache configuration: https://docs.gitea.com/1.22/usage/actions/act-runner
- Gitea cache tutorial/background: https://about.gitea.com/resources/tutorials/enable-gitea-actions-cache-to-accelerate-cicd
- Docker multi-platform builds and cross-compilation: https://docs.docker.com/build/building/multi-platform/
- Docker BuildKit cache mounts: https://docs.docker.com/build/cache/optimize/
- Docker build variables: https://docs.docker.com/build/building/variables/

Implementation guidance:

- Preserve Gitea compatibility when choosing action versions.
- Treat `actions/cache` failures as warnings unless a generated artifact truly depends on the cache.
- Use Docker registry cache for image layers and BuildKit cache mounts for package/compiler caches inside Dockerfiles.
- Pin build stages to `$BUILDPLATFORM` when cross-compiling to avoid expensive QEMU SDK/toolchain execution.

## AdGuard and Internal DNS

Primary docs:

- AdGuard Home API OpenAPI source: https://github.com/AdguardTeam/AdGuardHome/blob/master/openapi/openapi.yaml

Implementation guidance:

- Keep using the existing Hashi AdGuard plan/apply/result path.
- Internal agent DNS must use a distinct Hashi-owned rewrite source.
- Do not delete manual AdGuard rewrites.
- Do not create Traefik routers for `hashi.home.arpa`.
