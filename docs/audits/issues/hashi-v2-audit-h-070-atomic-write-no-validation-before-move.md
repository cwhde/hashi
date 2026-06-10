# H-070: Atomic Write Has No Validation Before Move

**Priority:** High
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §3 Non-Negotiable Rule 8

**Status:** Fixed
**Branch:** audit-series-h

## Description

`SshRemoteExecutor.WriteAtomicCore()` writes to a temp file, then moves it to the final path. However, there is no validation step between writing the temp file and moving it. The spec explicitly requires validation before the atomic move. If invalid content is written to temp, it gets moved to the final path without any syntax or structure check.

## Evidence

- `SshRemoteExecutor.WriteAtomicCore()` flow: hash compare → write to `.hashi.tmp` → `mv` to final path
- No validation callback or syntax check between write and move

## Expected Outcome

After writing to temp, the temp file content should be validated (e.g., parse check for configs, syntax check for scripts) before moving to the final path.

## Fix Guidance

1. Add a validation callback parameter to `WriteAtomicCore` that validates the temp file content before the `mv` command.
2. For YAML configs, call the YAML parser.
3. For shell scripts, call shellcheck or basic syntax validation.
4. On validation failure, clean up the temp file and return an error.

## Acceptance Criteria

- [ ] Invalid YAML written to temp file is detected before move to final path
- [ ] Invalid shell script syntax is detected before move
- [ ] Temp file is cleaned up on validation failure
- [ ] Validation errors are returned to the caller with clear messages
