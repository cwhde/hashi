# AUD-104: WAF pins an unpublished plugin release and loads only a fraction of CRS

Priority: P1

Category: Missing feature / WAF installation

Status: open; confirmed by manual source trace.

## Finding

TraefikConfigRenderer pins github.com/jcchavezs/coraza-http-wasm-traefik v0.6.0. The upstream tag inventory checked on 2026-10-18 contains only v0.1.0, v0.2.0, v0.2.1, v0.2.2 and v0.3.0; requesting v0.6.0/README.md also returns404. Fresh plugin resolution cannot install the specified release. Separately the WAF renderer includes only REQUEST-900, REQUEST-941 (XSS) and RESPONSE-959. It never includes SQL injection rules or the full configured CRS set, recommended engine configuration or CRS setup. The advertised general OWASP CRS protection therefore is not implemented by the emitted directives. This does not assert that every XSS rule is ineffective or that no included rule can block.

## Evidence

src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:162-164; src/Hashi.Core/Security/WafModels.cs:44-53. Main specification section12 and phase10. Primary repository: https://github.com/jcchavezs/coraza-http-wasm-traefik ; git ls-remote --tags against that repository returned the five releases listed above.

## Expected outcome and acceptance criteria

Select an actually published compatible plugin/artifact, configure its documented engine and full CRS initialization, and independently exercise clean traffic, SQLi, XSS, request-body inspection, exclusions and DetectOnly versus On through the real pinned Traefik runtime.
