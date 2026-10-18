# SEC-001 - Active V2 dependencies have unresolved high severity advisories

Priority: P2 (dependency maintenance; raise for any established reachable exploit)

Category: Security / dependency vulnerability

Specification reference: Main implementation specification, sections 8, 29, and 30 (secret handling, secure dependencies, and dependency auditing).

## Problem

Current dependency scans flag active V2 dependencies with High advisories. The lockfiles and project files still resolve the affected versions. Some reports apply only when specific parser or SCP code paths receive untrusted input, so impact needs to be matched to actual usage; the findings still need a recorded patch or a documented non-applicability rationale.

## Evidence

- `dotnet list Hashi.slnx package --vulnerable --include-transitive` reports `SSH.NET 2024.2.0` with GHSA-q939-rpr3-3284 and GHSA-mggc-4xg6-vcxf, and `Microsoft.OpenApi 2.0.0` with GHSA-v5pm-xwqc-g5wc.
- The project pins SSH.NET in `src/Hashi.Infrastructure/Hashi.Infrastructure.csproj`; `Microsoft.OpenApi` is pulled transitively by `Microsoft.AspNetCore.OpenApi` in `src/Hashi.Api/Hashi.Api.csproj`.
- The SSH.NET advisories list version 2026.0.0 as patched. They concern unsafe server-controlled SCP download names and server-side command execution through SCP remote-path handling. Confirm whether Hashi calls the affected SCP APIs and whether any path is attacker-controlled before assigning exploitability. Sources: [GHSA-q939-rpr3-3284](https://github.com/advisories/GHSA-q939-rpr3-3284), [GHSA-mggc-4xg6-vcxf](https://github.com/advisories/GHSA-mggc-4xg6-vcxf).
- Microsoft.OpenApi 2.0.0 is affected by circular-reference recursion that can terminate a process when parsing a crafted OpenAPI document. The advisory lists 2.7.5 as patched for the 2.x line. Source: [GHSA-v5pm-xwqc-g5wc](https://github.com/advisories/GHSA-v5pm-xwqc-g5wc).
- `corepack pnpm audit --prod` in `web/` reports 16 findings: 8 high, 7 moderate, and 1 low. The high advisories are GHSA-j22f-vq7h-c4qm, GHSA-mcm9-63f2-9j32, GHSA-x5rw-q4pp-hg5g, and GHSA-r9w8-h9r3-54w4 (`devalue 5.8.1`); GHSA-28wg-ghj8-5hjv and GHSA-2v37-7h3g-55p8 (`nanoid 3.3.12`); GHSA-r28c-9q8g-f849 (`postcss 8.5.15`); and GHSA-68fv-2mgg-jv7q (`source-map-js 1.2.1`). The same scan reports moderate findings for `@sveltejs/kit 2.61.1` and `devalue`, plus one low `devalue` finding. Several have fixes available within the package's current major line; audit each finding and update the lockfile.

## Expected outcome

### Current reachability disposition — 2026-10-18

The complete production implementation was manually read. SSH uses SFTP rather than the affected SCP APIs, and no attacker-supplied OpenAPI document parser entry point was identified. Those specific advisory paths are not reachable through the current application operations identified in this review. They remain update/CI-maintenance work, not evidence of a current remote Hashi compromise. The frontend is shipped with adapter-static, so server-only SvelteKit advisory paths must not be assigned to a nonexistent deployed Node SSR server. Client/bundled and build-tool paths still require package-specific patch review. Full and production audits are retained separately in the supplemental verification record. Do not globally suppress advisories or count duplicated graph entries as separate source defects.

## Repair outcome

Every active V2 advisory is either fixed with compatible patched dependencies or explicitly dispositioned with evidence that the vulnerable code path is unreachable in production. The normal CI and security workflows pass after the dependency changes.

## Acceptance criteria

- Update SSH.NET to a patched supported version, or document why affected APIs cannot be reached and define a time-bound update plan.
- Resolve or document the `Microsoft.OpenApi` advisory with a compatible `Microsoft.AspNetCore.OpenApi` / `Microsoft.OpenApi` combination.
- Update the web dependency lockfile so all applicable High findings are cleared; document any remaining findings and their production reachability.
- Run .NET build and tests, frontend check/lint/tests/build, and the repository's dependency-audit workflow after the updates.
- Do not suppress audit warnings globally to make CI green.
