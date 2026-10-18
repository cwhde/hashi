# AUD-016: Traefik routers do not consistently use the configured ACME resolver

Priority: P3 — latent certificate configuration correctness

Spec reference: implementation spec §8.3 requires Google Trust Services ACME DNS challenge and certificate provisioning; renderer also exposes Let's Encrypt resolver choices.

## Finding

`TraefikConfigRenderer.RenderAcmeBlock` maps provider settings to resolver names (`letsencrypt-staging` maps to `letsencrypt-staging`; unknown/default values map to `gts`). However, `RenderHttpRouter` hardcodes `certResolver: gts` for every HTTPS resource. `RenderHealth` uses the raw `AcmeProvider` setting rather than the resolver name returned by the mapping. If a non-GTS value reaches `TraefikRenderOptions`, HTTPS resources continue to request `gts`, and the health router can reference a resolver that was never generated. The current settings UI/API exposes only the GTS path, so this is a latent inconsistency unless the persisted setting is changed outside the supported UI.

## Evidence

- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs`: `RenderAcmeBlock` maps ACME provider values to resolver names at lines around 631–646.
- `RenderHttpRouter` hardcodes `certResolver: gts` around line 368.
- `RenderHealth` emits `certResolver: {{options.AcmeProvider}}` around line 676 instead of using the same mapping.
- `src/Hashi.Infrastructure/Platform/CertificateSetupService.cs` passes the persisted `AcmeProvider` into `TraefikRenderOptions`.

## Expected outcome

Resolve the selected ACME provider to one canonical resolver name once, and use it consistently in the ACME static config, all HTTPS resource routers, and the Hashi health router. Reject unsupported provider values instead of silently mapping one name while emitting another.

## Acceptance criteria

- GTS configuration emits and references `gts` consistently.
- Let’s Encrypt and staging configuration emit and reference their matching resolver name consistently, or those choices are removed if unsupported by the product.
- Rendered configuration validation confirms every referenced resolver is defined.
- Changing the selected provider changes the resolver referenced by HTTPS routers.
