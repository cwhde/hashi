# Candidate dispositions and rejected premises

The initial handwritten file notes preserve investigation history. The current issue register, matrix and this record supersede phrases such as “candidate”, “pending mapping” or “verify later” from that reading phase. Unproven hypotheses are not silently promoted to confirmed defects. All source files were read; runtime acceptance for repairs remains separate.

## Confirmed after cross-layer tracing

| Reading hypothesis | Current disposition |
| --- | --- |
| Missing status registration versus a namespace import | AUD-095; six admin operations are unregistered. |
| Vault service/admin capability leak and never initialized purpose keys | AUD-096; independent synthetic envelope recovery. |
| Consumed frontend response bodies | AUD-097; independent installed-client reproduction. |
| Rate response branch without real producer | AUD-098; declaration-only state is not implemented rate enforcement. |
| Configurable offense policy versus unused fields | AUD-099; unrelated arithmetic overflow remains AUD-066. |
| Reveal slash mismatch / secret collection prefix mismatch | AUD-100 and169; independent routing probe for slash behavior. |
| Invalid IPv4 trusted CIDR becomes broad IPv6 match | AUD-101; actual resolver source reproduced. |
| Country reader fallback / any-database availability | AUD-102; official reader rejects incompatible edition methods. |
| GeoIP semantic validation after replacement | AUD-103; atomic file replacement does not validate an MMDB. |
| Incomplete CRS / nonexistent pinned plugin | AUD-104; no claim that every included XSS rule cannot block. |
| Invalid kind poisons all rendering | AUD-105; creation validator checks nonempty, not the enum. |
|304 feed enable/mode and parse last-good behavior | AUD-106/107; metadata edits lose CIDR parser options in163. |
| Resource Pulse field without upstream consumer | AUD-108; connection target resolution is a different implemented path. |
| Probe overwrites agent lifecycle | AUD-109; old tokens remain invalid, no token resurrection claimed. |
| Script transport timeout, autonomous env/results and target defaults | AUD-010/080/110/111/148/166. |
| Middleware-only parser retains arbitrary siblings | AUD-112; privileged ownership boundary, not anonymous RCE. |
| AdGuard path/TLS/username and disabled/stale behavior | AUD-113/135/136/137, separate from real API shape074. |
| Active minimum-idle session expires | AUD-114; expiry guard runs before activity renewal. |
| Recovery severity filtering | AUD-115, separate from degraded transition026. |
| Nullable bucket uniqueness and lost counters | AUD-116; PostgreSQL default null semantics checked. |
| Request URI length passed into varchar256 prefix | AUD-117; telemetry failure occurs outside decision fallback. |
| Arbitrary geo headers cross the trusted transport | AUD-118; generated Traefik forwards client headers without sanitization. |
| Unknown rule fields silently broaden a match | AUD-119; different from unknown action009. |
| Search limiting before filtering | AUD-120. |
| Port removal reused before apply | AUD-121. |
| CAPTCHA reset affects history instead of actual legacy trigger | AUD-122. |
| Accepted session/composite subjects have no matcher | AUD-123. |
| Infrastructure monitor identity/orphan handling | AUD-124; generated source IDs are missing. |
| Forward-auth and response logs double count requests | AUD-125; correlation IDs are stored but not consumed. |
| Persistent/bypass resource actions are only response labels | AUD-126. |
| HTTP listener, inherited rewrite, priority and IPv6 URL defects | AUD-127/128/129/140. |
| Zone/default TTL/address-family/public classification | AUD-130/131/132/164; TXT case/value issues remain060. |
| Rescue disarm and live policy refresh | AUD-133/134; executable-mode failure separately085. |
| Installer function-local trap lifetime | AUD-138; independent Bash probe, no live installation. |
| Cached CSRF token identity | AUD-139; session ID is NameIdentifier, navigation retains the module. |
| Inert category settings / default forms after failed load | AUD-141/142. |
| Last-credential race / empty authenticated envelope | AUD-143/144. |
| Public HTTPS surface mode, overview placeholders, duplicate keys | AUD-145/146/147. |
| Host-only identity and global-root scope | AUD-149/150. |
| Internal DNS save/query proposal semantics | AUD-151/171. |
| Transport cancellation and key span lifecycle | AUD-152/153. |
| Unknown notification type skips secret extraction | AUD-154. |
| Svelte chart self-dependency | AUD-155; actual compiled component independently reproduced. |
| Timestamp-free strip / missing lifecycle controls / rollup race | AUD-156/157/158. |
| Fresh firewall package provisioning | AUD-159; preflight only rejects missing commands. |
| Public view freshness/errors and local storage failure | AUD-160/161; malformed order alone does not prove a crash. |
| Render/backups disclose EAB | AUD-162; ordinary read preview and database text are separate from vault protection. |
| Nonproduced setup claim, private-IP cookies and partial publication | AUD-165/167/168. |
| Malformed base64/numeric secret purposes | AUD-170. |
| Legacy defects | LEG-001 through008; separately deployed process, not V2 runtime. |

## Rejected or narrowed claims

| Hypothesis | Resolution |
| --- | --- |
| Monitoring product views require SQL database views | Rejected. Time-range product views are required; a SQL implementation is not. AUD-089 was corrected to the actual notification migration failure. |
| Every blank/missing Go module cache/go.sum indicates broken caching | Rejected. The agent has no third-party modules. Exact release compiler maintenance is SEC-002. |
| Gosec G304 is an attacker-controlled file read | Rejected. Both callers pass the literal `/proc/1/cgroup`. |
| Standard FIDO callbacks omit all attestation/signature verification | Rejected. The library performs cryptography; callbacks check credential uniqueness/ownership. Replay/enrollment gaps are004/005. |
| Random recovery keys become low entropy because humans save them | Rejected. The actual known fallback/server acceptance problem is082. |
| Different ports necessarily make cookies cross-site | Rejected. Site and origin are different concepts; localhost and non-localhost HTTP behavior is separately167. |
| Service vault remains permanently locked after early startup ensure | Rejected. The orchestrator retries after service-key initialization. |
| Script default-all hosts itself violates the spec | Rejected. Explicitly required. Misleading UI and all-disabled fallback are148/166. |
| Security profile SSO is wholly ignored | Narrowed. Renderer selects strict-mode middleware, which forces SSO. Same-host identity, provider isolation and previews remain wrong for separate reasons. |
| Normal deletion always leaves a referenced security profile dangling | Rejected as a general claim: an in-use guard exists. A concurrent integrity race requires separate demonstrated evidence. |
| All WAF On requests can never block | Not established. Included rules may block; unpublished artifact/full CRS configuration104 is independently confirmed. |
| Every SSH command necessarily runs under Debian dash | Rejected. SSH uses the configured account shell. Installer commands are not portable to every shell, but this is not proof of universal Debian failure. |
| Every TLS expiry probe must perform full certificate-trust monitoring | Not promoted merely from its permissive certificate callback: the specified check is expiry. Timeout and target defects are036/069; broader chain-health monitoring is a product choice. |
| Missing AAD alone proves an exploitable vault swap attack | Not established independently of the database-write threat model. Capability leak096 and plaintext disclosure162 are confirmed instead. |
| Missing explicit DataProtection persistence proves required session semantics are broken | Not promoted as a separately mandated feature. Container recreation/key persistence belongs in documented deployment acceptance; it must not be mistaken for existing deployment verification. |
| Unused entity columns require a particular implementation/table shape | Not assumed. Equivalent models can satisfy features; actual missing runtime/UI behavior is mapped in the matrix. |
| Incomplete tests become trustworthy after merely rerunning them | Rejected. Tests were read, and independent probes/scanners plus real integration acceptance remain separate. |
| All63 release-binary scanner advisories are exploitable agent bugs | Rejected. Package/symbol enumeration includes unreachable/server/platform/newer-API cases. Supported compiler maintenance remains SEC-002. |

## Other observations kept in scope

The handwritten ledger also records smaller robustness/maintenance observations: derivative OS detection, duplicate cache/widget preferences, API-only fields, missing audit detail, schema/FK/index design concerns, response disposal/zeroization and source-specific input bounds. A suggestive pattern without a demonstrated valid failing operation or a required missing behavior is not labeled a confirmed additional defect. This avoids inventing obligations (as the SQL-view and old H-series examples did). Existing confirmed issues cover the demonstrated unsafe or nonworking paths and their acceptance criteria include adjacent validation/failure boundaries.
