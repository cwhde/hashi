# H-099: Access Log Fields Not Minimized to Required Set

**Priority:** Low
**Conflict Type:** wrong_implementation
**Spec Reference:** §10.2 (Access log: minimal useful fields)

**Status:** Not Started
**Branch:** 

## Description

The Traefik static config renderer sets access log format to JSON with header redaction (`defaultMode: drop` with selective keep), but general access log fields are not restricted. The spec requires "minimal useful fields" — all default Traefik fields are logged, not just the minimal set needed for security analysis.

## Evidence

- Access log fields.general section has no `defaultMode: drop` with selective keep
- All default Traefik fields are logged (StartUTC, RequestHost, etc.)
- Header redaction is correctly configured but field-level filtering is not

## Expected Outcome

Access log should only contain minimal useful fields. Unnecessary fields (StartUTC, RequestHost, etc.) should be dropped. Header redaction should be maintained as-is.

## Fix Guidance

1. Add `fields.general.defaultMode: drop` with selective keep for needed fields (`ClientAddr`, `ClientHost`, `DownstreamContentSize`, `Duration`, `OriginStatus`, `RequestAddr`, `RequestMethod`, `RequestPath`, `RequestProtocol`, `RouterName`, `ServiceName`).
2. Ensure header redaction is maintained as-is.

## Acceptance Criteria

- [ ] Access log only contains minimal useful fields
- [ ] Unnecessary fields (StartUTC, RequestHost, etc.) are dropped
- [ ] Header redaction is maintained as-is
