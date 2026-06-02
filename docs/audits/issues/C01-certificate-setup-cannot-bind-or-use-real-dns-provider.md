# C01 - Certificate setup cannot bind or use the real DNS provider

Priority: High

Spec conflicts: section 7.4 requires ACME email, EAB credentials, DNS provider binding, DNS challenge delay, resolver list, and encrypted ACME secrets. Section 10.2 requires the Traefik ACME resolver to use Google Trust Services with Hetzner DNS challenge.

## Problem

The certificate setup flow does not bind to a DNS provider connection, and the validation path checks for the wrong connection type. Real DNS provider connections are stored as `dns_provider`, but certificate setup looks for `dns`, so a correctly configured DNS provider is treated as missing.

Even if validation is bypassed, Traefik rendering hard-codes the ACME provider as Hetzner and receives only EAB credentials from certificate setup. There is no selected DNS provider connection, no stored provider binding, and no path that carries the chosen provider token into the Traefik ACME runtime.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:281-297` requires DNS provider binding and encrypted ACME secrets in certificate setup.
- `src/Hashi.Contracts/Api/ConnectionContracts.cs:3-6` defines the real DNS provider connection type as `dns_provider`.
- `src/Hashi.Infrastructure/Persistence/Entities/DnsEntities.cs:159-162` maps `ConnectionTypeNames.DnsProvider` to that contract value.
- `src/Hashi.Infrastructure/Platform/CertificateSetupService.cs:31` checks `x.Type == "dns"` when reporting whether DNS exists.
- `src/Hashi.Infrastructure/Platform/CertificateSetupService.cs:49` repeats the same wrong type check during validation.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:160-165` defines `CertificateSetupRequest` without any DNS provider connection id or binding field.
- `src/Hashi.Infrastructure/Platform/CertificateSetupService.cs:121-127` builds Traefik options from ACME/EAB settings only.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:465-491` hard-codes `provider: hetzner`.
- `tests/Hashi.UnitTests/CertificateSetupServiceTests.cs:79-85` seeds the incorrect `"dns"` type, so the tests mask the production mismatch.

## Expected outcome

Certificate setup should validate against real enabled DNS provider connections, persist the chosen provider binding, and render/apply Traefik ACME configuration using the selected provider and its secret material through the service-sync vault path.

## Fix guidance

Replace string literals with `ConnectionTypeNames.DnsProvider`. Extend `CertificateSetupRequest` and persisted settings with a selected DNS provider connection id. During Traefik sync, resolve that connection, decrypt the provider token through the appropriate unattended-safe secret path, and render/apply the ACME DNS challenge configuration from the selected provider instead of an unbound hard-coded value.

## Acceptance criteria

- Certificate setup validates successfully with a real `dns_provider` connection.
- Tests seed `ConnectionTypeNames.DnsProvider` and fail if the wrong connection type is used.
- Certificate setup persists a DNS provider binding.
- Traefik ACME rendering/apply uses the selected provider and can access the required provider token without a logged-in browser session.
- Missing, disabled, or unsupported DNS provider bindings produce a clear validation error before saving or applying Traefik config.
