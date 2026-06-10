# H-065: No Required Connection Minimum Enforcement — Last DNS/Traefik/Firewall Can Be Deleted

**Priority:** Critical
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §3 Non-Negotiable Rule 12 (Required connection types cannot be deleted below their minimum count); §3 Rule 13 (Required minimums are one DNS provider, one Traefik connection, and one Linux firewall host after setup completes)

**Status:** Fixed
**Branch:** h/security-2

## Description

No code enforces minimum connection type counts. The connection DELETE handler has no minimum check. No RequiredMinimum entity or validation exists. After setup, the user could delete all DNS providers, all Traefik connections, or all firewall hosts, leaving Hashi unable to perform its core functions. The spec explicitly requires that these minimums cannot be violated.

## Evidence

ConnectionEndpoints DELETE handler has no minimum count check. No validation prevents deleting the last DNS provider, Traefik connection, or firewall host after setup is complete.

## Expected Outcome

Deleting a connection should fail if it would drop below the required minimum for that type (1 DNS provider, 1 Traefik connection, 1 firewall host after setup). System resources with RequiredForAccess deletion policy should also be protected.

## Fix Guidance

Add a RequiredConnectionTypeMinimums check in the connection delete endpoint. Before deletion, count connections of the same type. If the count would fall below 1 after setup is complete, reject the deletion with a clear error message. Also add DeletionPolicy check to resource delete logic.

## Acceptance Criteria

- [ ] Deleting the last DNS provider returns 400 with explanatory error
- [ ] Deleting the last Traefik connection returns 400 with explanatory error
- [ ] Deleting the last firewall host returns 400 with explanatory error
- [ ] System resources with RequiredForAccess policy cannot be deleted
