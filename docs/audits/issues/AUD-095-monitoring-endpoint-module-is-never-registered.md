# AUD-095: Monitoring endpoint module is never registered

Priority: P1

Category: Missing working feature / monitoring administration

Status: open; confirmed by source and committed contract cross-check.

## Finding

`StatusEndpoints.MapStatusEndpoints` defines the admin monitoring inventory, manual endpoint CRUD, rollups and events, but Program never calls it. The namespace import alone does not register routes. There is no other caller in the repository. Consequently `/api/status/endpoints`, `/api/status/rollups` and `/api/status/events` are unavailable in the running application. The admin Status page and its details/settings cannot load; automatic monitoring workers and anonymous public status endpoints being present do not provide these admin routes.

The committed OpenAPI document and generated TypeScript likewise contain no `/api/status` operations. The client uses `as never` casts to call these absent paths, concealing the contract error during type checking. This is a cross-layer integration defect beyond the earlier narrower monitoring findings.

## Evidence

- `src/Hashi.Api/Program.cs:142-169`: all route registration calls; Status call absent despite its using directive at line 11.
- `src/Hashi.Api/Features/Status/StatusEndpoints.cs:10-80`: uncalled module and six operations.
- `web/src/lib/api/client.ts:483`: inventory call to absent route, followed by update and rollup/event helpers.
- Full committed OpenAPI path list has no `/api/status` entry. A repository-wide search finds `MapStatusEndpoints` only at its declaration.

## Expected outcome

Register the monitoring module, expose accurate typed success/error contracts, regenerate OpenAPI and clients, and replace casts masking these absent operations with generated path types.

## Acceptance criteria

An authenticated admin can list/create/update/delete manual endpoints, fetch rollups/events, edit supported settings and load the Status page through the actual application host. All six operations appear in generated contracts. Anonymous callers remain excluded from admin operations and public status retains its explicit selection rules.
