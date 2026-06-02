# D04 - Resource rewrite model omits replace-prefix mode

Priority: Medium

Spec conflicts: section 6 requires rewrite modes for replace prefix, rewrite exact path, regex replacement with capture groups, and strip prefix.

## Problem

Route-level rewrites support exact path replacement, regex replacement, and strip-prefix behavior, but there is no replace-prefix mode. Resource-level rewrites are even less expressive: the create request has only `PathRewrite`, no rewrite mode, so the renderer treats it as exact path replacement by default.

This prevents a resource from expressing the spec's replace-prefix behavior without falling back to custom Traefik middleware outside the resource rewrite model.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:182-186` lists the four required rewrite modes.
- `src/Hashi.Core/Resources/ResourceModels.cs:12-22` models route `RewriteMode` as a free-form nullable string, but there is no enum or validation containing `replace_prefix`.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:80-86` gives resource-level create requests `PathRewrite` but no rewrite-mode field.
- `web/src/lib/components/resources/ResourceRoutesEditor.svelte:153-165` offers only None, Replace path, Strip prefix, and Regex replace.
- `web/src/routes/(admin)/resources/+page.svelte:333-338` exposes resource-level path prefix and path rewrite target fields without a mode selector.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:294-319` renders `regex`, `strip_prefix`, or else `replacePath`; it has no replace-prefix branch.

## Expected outcome

Resource and route rewrite configuration should support all spec-listed modes, including replacing a matched prefix with a different prefix while preserving the suffix.

## Fix guidance

Introduce a typed rewrite mode enum or validated constants, add `replace_prefix`, and render it with Traefik-compatible `replacePathRegex` or another correct middleware composition that preserves the unmatched suffix. Add a resource-level rewrite mode field or normalize resource-level rewrites into the same route model.

## Acceptance criteria

- The API and UI expose replace-prefix as a first-class rewrite mode.
- Resource-level and advanced-route rewrites use the same validated rewrite mode set.
- Replace-prefix rendering preserves the suffix after the matched prefix.
- Existing replace-path, regex, and strip-prefix behavior remains unchanged.
- Tests cover all four rewrite modes and invalid mode rejection.
