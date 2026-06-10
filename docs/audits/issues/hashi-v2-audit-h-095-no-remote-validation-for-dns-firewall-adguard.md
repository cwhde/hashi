# H-095: No Remote Validation for DNS, Firewall, and AdGuard Apply

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** §25 (Apply must run remote validation)

**Status:** Not Started
**Branch:** 

## Description

Only Traefik Apply has remote validation (`ValidateStagedRemoteConfigAsync`). DNS, Firewall, and AdGuard apply without post-write remote verification. This means a failed write (SSH error, provider API error, network issue) could be silently accepted as successful.

## Evidence

- `ValidateStagedRemoteConfigAsync` exists only for Traefik
- DNS, Firewall, and AdGuard apply paths have no post-apply verification step

## Expected Outcome

Each subsystem should verify its apply succeeded via read-back. Failed verification should produce a clear error.

## Fix Guidance

1. For DNS: read back records from provider after apply.
2. For Firewall: read back the deployed file and compare hash after apply.
3. For AdGuard: list rewrites and verify after apply.
4. Failed verification should produce clear error messages.

## Acceptance Criteria

- [ ] DNS Apply verifies records via provider read-back
- [ ] Firewall Apply verifies deployed file hash matches
- [ ] AdGuard Apply verifies rewrites via API read-back
- [ ] Failed verification produces clear error
