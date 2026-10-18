# AUD-128: Route fallback rewrites reference middleware that was never emitted

Priority: P2

Category: Routing / rewrite inheritance

Status: open; confirmed by manual source trace.

## Finding

When a resource has route rows, rewrite middleware generation emits only each route with a nonempty RewriteValue. RenderHttpRouter nevertheless falls back from a route null RewriteValue to the resource PathRewrite and adds <slug>-route-<priority>-rewrite. No middleware with that name exists for the inherited case. Applying a resource-level rewrite and adding a route that inherits it therefore produces an unresolved middleware reference and unusable router.

## Evidence

src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:247-267,337-362. The configuration passes basic YAML syntax despite containing a dangling semantic reference.

## Expected outcome and acceptance criteria

Resolve effective rewrite value/mode/prefix once and emit the same named middleware that the router references. Cover inherited, route-specific, explicitly disabled and absent rewrites with semantic reference checks.
