# AUD-046: ACME settings are interpolated into YAML without semantic validation

Priority: P2 — malformed ACME inputs can break or alter generated Traefik configuration

Category: Configuration safety / input validation

Spec reference: `docs/implementation-spec/hashi-v2-implementation-spec.md` §§7.4, 10, and 25 require ACME settings to render valid Traefik configuration and require generated configuration validation before apply.

## Finding

Certificate setup validates the email only by checking that it contains `@`; resolver entries are checked only for nonempty strings; EAB key ID and HMAC are only checked for nonempty values. `TraefikConfigRenderer.RenderAcmeBlock` then interpolates email, EAB values, and resolver strings directly into YAML. Resolver strings are placed inside double quotes without YAML escaping, and email/EAB values are emitted as unquoted YAML scalars. Newlines, quotes, or YAML metacharacters can therefore make the generated file invalid or create syntactically valid configuration outside the intended field. Parsing the final YAML cannot prove that the resulting configuration represents the user's intended ACME values.

## Evidence

- `src/Hashi.Infrastructure/Platform/CertificateSetupService.cs`: `ValidateAsync` only checks email for `@`, and resolver items for whitespace; it does not validate resolver address syntax or constrain EAB characters.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs`: `RenderAcmeBlock` interpolates `AcmeEmail`, EAB values, and quoted resolver values directly into YAML without context-aware serialization.
- `src/Hashi.Core/Traefik/TraefikConfigValidator.cs`: final syntax validation does not establish that a parsed YAML document contains only the intended ACME fields.

## Expected outcome

Validate ACME email, resolver host/port, and EAB value formats before saving. Serialize YAML scalars and strings with a YAML library or a context-safe escaping routine, then validate the parsed structure and intended field values before apply.

## Acceptance criteria

- Email accepts only supported address syntax and rejects controls/newlines.
- Resolver entries parse as supported host/IP and port values and reject YAML delimiters/controls.
- EAB key ID and HMAC follow provider formats and are rendered as safely serialized scalars.
- Inputs cannot add or alter adjacent YAML keys, lists, resolvers, or routers.
- Invalid ACME settings fail planning before any remote write.
