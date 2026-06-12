# H-090: PlatformEndpoints.cs Is a 1482-Line Mono File

**Priority:** Medium
**Conflict Type:** bad_implementation
**Spec Reference:** §28 (Repository structure: Features/ directory with per-feature endpoint files)

**Status:** Fixed
**Branch:** audit-series-h

## Description

`PlatformEndpoints.cs` is 1482 lines and contains 14 distinct endpoint groups (Resource, Traefik, Firewall, Status, Public, EdgeAuth, EdgeChallenge, EdgeSsoAdmin, Security, Pulse, AdGuard, Waf, InternalAgentDns, Script, Notification). Per the spec convention, each should be its own file under `Features/{Domain}/`.

## Evidence

- `PlatformEndpoints.cs` contains all 14+ endpoint groups in a single file
- No `Features/` directory structure exists for endpoint organization

## Expected Outcome

Each domain should have its own `Features/{Domain}/{Domain}Endpoints.cs` file. No single endpoint file should exceed 300 lines. Import paths must remain correct after the split.

## Fix Guidance

1. Split `PlatformEndpoints.cs` into individual feature folders matching the domain (`Features/Pulse/PulseEndpoints.cs`, `Features/Security/SecurityEndpoints.cs`, etc.).
2. Ensure each file is under 300 lines.
3. Verify all import paths remain correct.

## Acceptance Criteria

- [x] Each domain has its own Features/{Domain}/{Domain}Endpoints.cs file
- [x] No single endpoint file exceeds 300 lines
- [x] Import paths remain correct after split
