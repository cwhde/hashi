# H-052: HashiInternalUrlResolver Always Uses HTTP for Internal URL

**Priority:** Low
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §7.2, §7.7

**Status:** Fixed
**Branch:** h/backend-quality

## Description

`HashiInternalUrlResolver` in `src/Hashi.Infrastructure/Platform/HashiInternalUrlResolver.cs` always constructs the internal URL using `http://`:

```csharp
// HashiInternalUrlResolver.cs
var baseUrl = $"http://127.0.0.1:{ports.Admin}";
```

For the setup flow, the spec §7.2 requires collecting: "Internal Hashi URL/IP and port." This implies the user can specify the full URL scheme. Additionally, §7.7 states: "Setup creates a system resource for Hashi itself: Domain: selected admin public domain. Target: internal Hashi app. TLS: enabled."

If the admin public resource uses HTTPS (TLS), the internal URL between the Traefik proxy and Hashi might also use HTTPS in some deployment configurations (e.g., when Traefik and Hashi communicate over TLS). Always using HTTP prevents this configuration.

## Evidence

```csharp
// HashiInternalUrlResolver.cs
// Hardcoded http:// scheme
var baseUrl = $"http://127.0.0.1:{ports.Admin}";
```

## Expected Outcome

The internal URL scheme should be configurable:
1. Default to `http://` for localhost loopback communication (fine for most deployments)
2. Allow the admin to configure `https://` if the deployment requires it
3. Use the configured internal URL from `AppSettingsEntity.InternalUrl` if present

## Fix Guidance

1. Check `AppSettingsEntity.InternalUrl` first and use it if set.
2. If not set, construct the URL from the configured admin port and scheme.
3. Add `InternalScheme` or extend `InternalUrl` to accept scheme-qualified URLs.
4. Validate that the internal URL is reachable from within the Hashi container.

## Acceptance Criteria

- [ ] Internal URL is configurable via settings
- [ ] Default is `http://127.0.0.1:{admin_port}` (current behavior preserved)
- [ ] HTTPS internal URL is supported when explicitly configured
- [ ] `InternalUrl` app setting is honored if present
- [ ] System resource for Hashi uses correct internal URL
