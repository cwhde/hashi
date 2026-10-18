# Supplemental verification — 2026-10-18

Baseline: `49f34dc1a8b7b3ec7332588b39311d66fe13cabc`, plus local audit documentation. The manual ledger records the separate source reading. These tools supplement that reading; passing tools and historical tests do not prove feature correctness. No pre-existing project tests were executed in this restarted pass. No production credentials, servers or provider state were used in the independent reproductions.

## Independent reproductions

| Check | Method | Result and boundary |
| --- | --- | --- |
| Vault capability isolation, AUD-096 | Scratch .NET executable links repository AES-GCM/HKDF helpers and reproduces the persisted envelope chain using synthetic secrets | Session-only plaintext recovered using database blobs plus deployment key. Source inspection establishes that real services store this chain; the executable does not instantiate real vault services or a database. |
| Consumed response bodies, AUD-097 | Installed `openapi-fetch`, synthetic in-memory `fetch` response | Parsed data exists, response body is consumed, application fallback yields `{}`. No network request. |
| Reveal trailing slash, AUD-100 | Ephemeral loopback ASP.NET host with the reveal route and the middleware predicate | Both paths route successfully; `/reveal/` fails the raw suffix predicate. This demonstrates routing/predicate divergence; source establishes resulting scope/reauth selection. It does not claim login, IP binding or vault unlock are bypassed. |
| Invalid trusted CIDR, AUD-101 | Scratch executable compiles actual `ForwardedClientContextResolver` | `192.0.2.0/33` trusts peer `203.0.113.17` and accepts forged XFF `198.51.100.42`. Valid IPv4 prefixes are outside this specific defect. |
| Chart reactive loop, AUD-155 | Actual Svelte component compiled with installed compiler; independent jsdom mount with only uPlot/CSS stubbed |13 constructions/12 destroys before a deliberate safety bound stopped the recurrent effect. Raw result retained; inherited tests mock this component. |
| Installer trap lifetime, AUD-138 | Bash scratch function reproduces only function-local variable/EXIT trap lifetime under set-u | After function returns, trap fails with `tmp_binary: unbound variable`. No installation or file mutation performed. |

Scratch sources and raw results are retained outside application source under the task's `work/repros/` and `work/` directories. They are independent review probes, not additions to the inherited test suite.

## Fresh scanners

| Tool | Result | Disposition |
| --- | --- | --- |
| NuGet vulnerability audit | SSH.NET2024.2.0: GHSA-q939-rpr3-3284 and GHSA-mggc-4xg6-vcxf; Microsoft.OpenApi2.0.0: GHSA-v5pm-xwqc-g5wc | SEC-001. Runtime SSH uses SFTP, not the affected SCP download APIs; OpenAPI is generated, with no identified untrusted document-parser entry point. Patch planning remains appropriate, but the reported High package severity is not evidence of an exploitable runtime path in Hashi. |
| `pnpm audit --prod --json` |16 findings:8 high,7 moderate,1 low;121 dependencies | SEC-001; package/advisory records require deployment reachability assessment. Static frontend deployment differs from a SvelteKit server. |
| Full `pnpm audit --json` |63 findings:32 high,24 moderate,7 low;367 dependencies | Includes development/build tooling and repeated vulnerable dependency paths. Not63 independent application vulnerabilities. Retain build-tool risk separately from production reachability. |
| Legacy `npm audit --omit=dev --json` |14 affected package entries:2 critical,9 high,3 moderate;177 dependencies | Separate legacy graph. Legacy publishing still exists, so record upgrade/exposure work for that artifact; do not attribute its dependency tree to V2. |
| `go vet ./...` | Passed | Supplemental static analysis of Pulse; not runtime or feature acceptance. |
| gosec2.29.0 | One G304 (variable filename in `os.ReadFile`) | False positive at Pulse main.go271: only callers pass the literal `/proc/1/cgroup` at230/246. No attacker-controlled filename path identified. |
| govulncheck1.9.0 with local Go1.26.8 on Windows |13 distinct standard-library advisories, with multiple interface-expanded traces; all report fixes in1.26.9 | Toolchain findings, not13 source defects. Pulse uses an HTTP client; many reported server-handler traces are conservative interface expansion. Release builds pin a different Go version/platform; assess that exact Linux toolchain before claiming release reachability or clearance. |
| govulncheck binary scan of independently rebuilt Linux/amd64 Go1.22.12 agent |63 unique advisory identifiers /884 records | SEC-002. Confirms the release toolchain needs updating. Binary package/symbol enumeration includes server/newer-API/platform cases that do not establish reachable Pulse exploits; not63 independent vulnerabilities. No existing tests or production agent executed. |
| Gitleaks8.30.1, redacted HEAD history scan |370 commits /7.61MB, no leaks reported | Applies to scanned HEAD history only, not proof that uncommitted files or excluded/local files contain no secrets. No secret values were printed. |

Raw scanner reports are retained in the task workspace. Some audit commands exit nonzero when findings exist; this is not a scanner execution failure. No dependency versions, application implementations or deployment state were changed by this review.

## External behavior checked

- MaxMind's reader rejects an incompatible database type before lookup: [official source](https://github.com/maxmind/GeoIP2-dotnet/blob/main/MaxMind.GeoIP2/DatabaseReader.cs). AUD-102 traces Hashi's Country-to-City fallback and availability checks.
- The configured Coraza plugin repository publishes tags through v0.3.0; `git ls-remote --tags` returned no v0.6.0 on2026-10-18. [Official repository](https://github.com/jcchavezs/coraza-http-wasm-traefik). AUD-104 records the installation issue and incomplete CRS directive list separately.

## No deployment clearance

These results do not establish that fresh installation, migrations, real Traefik/Coraza loading, real provider mutation, firewall activation/rescue or scheduled host jobs work. Findings identify several reasons they currently cannot be treated as complete. Deployment verification belongs in the repair acceptance criteria after those defects are corrected.
