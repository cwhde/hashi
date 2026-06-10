# H-097: Resource DetectedFirewallHost Has No Auto-Detection Logic

**Priority:** Low
**Conflict Type:** partial_implementation
**Spec Reference:** §6 (Detected Linux firewall host field on resource)

**Status:** In Progress
**Branch:** h/monitoring-dns-firewall
**Branch:** 

## Description

`ResourceEntity.FirewallHostId` is a manually-set FK. There's no auto-detection logic that determines which firewall host a target IP resides behind. The spec field name "DetectedFirewallHost" implies auto-detection.

## Evidence

- `FirewallHostId` is a manually-set foreign key with no auto-detection
- No code matches target IPs against firewall host managed subnets or NetBird CIDRs

## Expected Outcome

Resources should auto-detect their firewall host based on target IP. Auto-detection should check managed subnets and NetBird CIDRs. Manual override should take precedence over auto-detection.

## Fix Guidance

1. Add auto-detection logic in `DnsDesiredStateBuilder` or a new service that matches target IPs against firewall host managed subnets and NetBird CIDRs.
2. Store the detected host separately from the manually-specified override.
3. Manual override takes precedence over auto-detection.

## Acceptance Criteria

- [ ] Resources auto-detect their firewall host based on target IP
- [ ] Auto-detection checks managed subnets and NetBird CIDRs
- [ ] Manual override takes precedence over auto-detection
