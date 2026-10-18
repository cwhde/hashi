# AUD-035: Resource values are interpolated into Traefik rules and YAML without full validation

Priority: P2 — malformed resource data can broaden routes or break generated proxy configuration

Category: Configuration injection / input validation

Spec reference: implementation spec §§5.2 and 10 require generated Traefik routing to reflect validated resource intent and require generated configuration validation before apply.

## Finding

Resource values that become Traefik configuration are not comprehensively validated or escaped. Custom domains and path match values are inserted inside Traefik rule backticks, while target schemes and hosts are inserted into YAML double-quoted upstream URLs/addresses. The create validator only requires nonempty names/scheme/host and valid ports/mode enumerations; `ResourceService.ValidateDomain` checks domain-mode consistency but not hostname syntax, and the update path assigns several target/routing values directly. YAML validation can catch malformed YAML but does not prevent a syntactically valid Traefik rule from being broadened by an embedded backtick/operator. Ordinary administrator input can therefore create a broken or wider-than-intended route and/or alter the generated service URL.

## Evidence

- `src/Hashi.Core/Validation/RequestValidators.cs`: `CreateResourceRequestValidator` only requires nonempty `TargetScheme`/`TargetHost`; it validates mode enums but not hostname, URI scheme, or control characters.
- `src/Hashi.Infrastructure/Platform/PlatformServices.cs`: `ValidateDomain` validates mode/domain relationships but does not validate DNS label syntax; create/update store target values and route/path values.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs`: `BuildPathRule` interpolates domain and path values between backticks; generated HTTP service URLs and TCP/UDP addresses interpolate scheme/host into YAML strings.
- `src/Hashi.Core/Traefik/TraefikConfigValidator.cs`: generated YAML syntax validation cannot by itself detect an unintended but syntactically valid Traefik matcher or upstream URL.

## Expected outcome

Validate each field according to its semantic role before persistence and render values with context-appropriate escaping/serialization. Validate the final parsed routing semantics and fail the plan before any remote write when values are invalid.

## Acceptance criteria

- Domain names accept valid IDNs/hostnames/IP literals as explicitly supported and reject whitespace, control characters, Traefik matcher delimiters, and invalid labels.
- Target scheme is restricted to supported schemes; target host parsing rejects URI delimiters, quotes, newlines, and unsupported path/user-info components.
- Path and regex values are safely represented as Traefik rule arguments and YAML strings; invalid or ambiguous expressions fail validation.
- Create and update use the same validation rules before persisting or triggering sync.
- A malformed value cannot broaden a router match or create a second YAML key/service; the plan reports the validation error and applies no files.
