# H-042: TraefikConfigRenderer Only Supports Google Trust Services Cert Resolver

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §7.4, §10.3

**Status:** Fixed
**Branch:** h/backend-quality

## Description

`TraefikConfigRenderer` in `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs` hardcodes the ACME certificate resolver to `gts` (Google Trust Services):

```csharp
// TraefikConfigRenderer.cs — RenderAcmeBlock or similar
certResolver: gts
```

The spec §7.4 states initial support for: "Google Trust Services ACME. Hetzner DNS challenge." While GTS is the first supported provider, the renderer provides no mechanism to select an alternative provider or use Let's Encrypt (the most commonly used free ACME provider).

For a homelab product, requiring Google Trust Services (which requires EAB credentials and may have rate limits or domain restrictions different from Let's Encrypt) creates an unnecessary dependency and onboarding friction. Many users expect Let's Encrypt support out of the box.

## Evidence

```csharp
// TraefikConfigRenderer.cs — certResolver is hardcoded to "gts"
certResolver = "gts"
```

The spec §24 (Settings > Traefik) lists "ACME defaults" as configurable, implying the ACME provider should be configurable, not hardcoded.

## Expected Outcome

1. The certificate resolver type should be configurable (at minimum: `gts` and `letsencrypt`).
2. The Traefik static config should support the appropriate ACME CA server URL for each provider.
3. Let's Encrypt should work without EAB credentials (EAB is optional for Let's Encrypt).

## Fix Guidance

1. Add an `AcmeProvider` setting (e.g., `"gts"` or `"letsencrypt"`) to `AppSettingsEntity`.
2. In `TraefikConfigRenderer`, use this setting to select the CA server URL and whether EAB is required.
3. For Let's Encrypt: use `https://acme-v02.api.letsencrypt.org/directory` (production) or `https://acme-staging-v02.api.letsencrypt.org/directory` (staging).
4. For GTS: use `https://dv.acme-v02.api.pki.goog/directory` (production) or test environment.
5. Update the setup flow to allow choosing the ACME provider during certificate provider setup (§7.4).

## Acceptance Criteria

- [ ] `AcmeProvider` setting is configurable via settings
- [ ] Both `gts` and `letsencrypt` generate valid Traefik static config blocks
- [ ] Let's Encrypt configuration works without EAB credentials
- [ ] GTS configuration correctly includes EAB key ID and HMAC
- [ ] Traefik config validation passes for both providers
- [ ] Setup flow offers choice of ACME provider
