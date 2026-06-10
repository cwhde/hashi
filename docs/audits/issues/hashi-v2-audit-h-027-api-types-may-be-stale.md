# H-027: web/src/lib/api/types.ts May Be Stale

**Priority:** Low
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §4 (Frontend - API client)

**Status:** Fixed
**Branch:** h/spec-compliance-1

## Description

The spec requires:
```
API client: generated TypeScript types from OpenAPI.
```

The implementation has:
- `web/src/lib/api/schema.d.ts` - Auto-generated from OpenAPI
- `web/src/lib/api/types.ts` - Hand-written domain type definitions (~365 lines)
- `scripts/generate-api-client.sh` - Generates types from OpenAPI

The `types.ts` file contains hand-written type definitions that may overlap with or deviate from the auto-generated `schema.d.ts`. This could lead to type inconsistencies if the API evolves.

The CI workflow verifies OpenAPI contract freshness:
```yaml
- name: Verify OpenAPI contract (spec §30)
  run: |
    ./scripts/export-openapi.sh
    ./scripts/generate-api-client.sh
    git diff --exit-code openapi/hashi.json web/src/lib/api/schema.d.ts
```

This ensures `schema.d.ts` stays in sync with the OpenAPI spec. However, `types.ts` is not verified for consistency.

## Evidence

The `types.ts` file contains domain-specific types that may not be auto-generated from OpenAPI. The `client.ts` file uses `schema.d.ts` for API calls but may also reference `types.ts` for domain types.

## Expected Outcome

- TypeScript types are consistent with OpenAPI spec
- No stale type definitions
- Type generation is automated

## Fix Guidance

1. Verify that `types.ts` is necessary (for domain types not in OpenAPI)
2. If types are duplicated, consolidate into auto-generated types
3. Add CI check to verify `types.ts` consistency if needed

## Acceptance Criteria

- [x] OpenAPI types are auto-generated (implemented)
- [x] CI verifies OpenAPI contract freshness (implemented)
- [ ] Verify types.ts is not stale or duplicated
