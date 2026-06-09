# H-013: Frontend Framework Deviation from Spec

**Priority:** Critical
**Conflict Type:** wrong_implementation
**Spec Reference:** Main Spec §4 (Frontend), §15

## Description

The implementation spec explicitly states:
- **§4 Frontend**: "Framework: SvelteKit 5 with TypeScript"
- **§4 Frontend**: "Main component system: shadcn-svelte, backed by Bits UI primitives"

However, examining the actual codebase:
- `web/package.json` uses `sveltekit` and `svelte` packages
- `web/components.json` is configured for `shadcn-svelte`
- `web/svelte.config.js` uses `@sveltejs/adapter-static`
- Route files use `.svelte` extension with Svelte 5 runes (`$state`, `$derived`, `$effect`, `$props`)

After careful examination, the frontend IS SvelteKit with shadcn-svelte. The earlier confusion was due to the exploration agent's misidentification. The actual implementation matches the spec.

However, there is a deviation: the spec mentions "Bits UI primitives" as the backing for shadcn-svelte, but the `web/package.json` does not include `@bits-ui` as a dependency. The shadcn-svelte components are present but may not be using Bits UI primitives as specified.

## Evidence

Spec requirement:
```
Framework: SvelteKit 5 with TypeScript.
Main component system: shadcn-svelte, backed by Bits UI primitives.
```

Actual implementation:
- SvelteKit 5 with TypeScript: ✅ Present
- shadcn-svelte: ✅ Present (`components.json` configured)
- Bits UI primitives: ❓ Not confirmed in `package.json`

## Expected Outcome

- Frontend uses SvelteKit 5 with TypeScript
- shadcn-svelte components are backed by Bits UI primitives
- Component system matches spec exactly

## Fix Guidance

1. Verify that `@bits-ui` packages are included as dependencies (they may be transitive through shadcn-svelte)
2. If Bits UI is not used, document the deviation and reason
3. The core SvelteKit + shadcn-svelte implementation is correct

## Acceptance Criteria

- [x] SvelteKit 5 with TypeScript is used (implemented)
- [x] shadcn-svelte is the component system (implemented)
- [ ] Bits UI primitives are confirmed as backing shadcn-svelte
- [ ] Any deviations from spec are documented
