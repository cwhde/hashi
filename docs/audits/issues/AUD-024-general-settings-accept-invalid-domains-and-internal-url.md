# AUD-024: General settings accept invalid domains and internal URLs

Priority: P2 — configuration correctness / sync reliability

Category: Missing validation

Spec reference: implementation spec §§5.2, 7, 8 and 24 require valid root/admin domains and usable internal URLs for routing, DNS, and provider synchronization.

## Finding

`PUT /api/settings/general` copies `RootDomain`, `AdminDomain`, and `InternalUrl` request strings directly into persisted settings without syntax or relationship checks. Those values are later used to build DNS record names, Traefik Host rules and service URLs, and internal callback URLs. Empty or malformed domains, URL paths/credentials, unsupported schemes, or a URL that points to the wrong port can be saved successfully and trigger a global sync with invalid or unreachable generated configuration.

## Evidence

- `src/Hashi.Api/Features/Setup/SettingsEndpoints.cs`: the general settings handler assigns the three request fields directly, saves, and immediately triggers global sync.
- `src/Hashi.Infrastructure/Platform/HashiInternalUrlResolver.cs`: `InternalUrl` is only trimmed and then concatenated with generated API paths.
- `src/Hashi.Infrastructure/Dns/DnsDesiredStateBuilder.cs` and `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs` consume the configured domains when generating DNS and Traefik configuration.

## Expected outcome

Validate values before persisting or triggering synchronization. Enforce DNS hostname syntax for root/admin domains, absolute HTTP(S) URL syntax for explicit internal URLs, and a clear rule for path prefixes, credentials, and port consistency.

## Acceptance criteria

- Nonempty root/admin domains are validated as hostnames; clearing behavior is explicit, and the admin domain cannot be cleared while the required admin resource depends on it.
- Explicit internal URLs must be absolute HTTP(S) URLs without unsupported query/fragment components; any allowed path prefix is normalized safely.
- The configured internal URL and admin listener port are checked for consistency or the precedence is documented.
- Invalid settings cannot be saved before a sync that will predictably fail.
