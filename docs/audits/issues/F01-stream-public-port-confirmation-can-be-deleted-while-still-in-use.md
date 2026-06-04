# F01 - Stream public-port confirmation can be deleted while still in use

Priority: High

Spec conflicts: section 10.5 says new public ports require confirmation and removing the last resource using a public port removes the generated entry point and firewall opening after confirmation.

## Problem

TCP/UDP public-port confirmation is stored as a single entry point row that is reassigned to whichever resource last synced that `(port, protocol)` pair. If two stream resources share one public port, deleting the later-synced resource removes the confirmed entry point even though the earlier resource still uses the same public port.

The renderer then filters stream resources by confirmed ports, so the remaining resource can silently disappear from the generated Traefik static/dynamic config and any generated firewall opening tied to that entry point can also be removed. That violates the "last resource using a public port" requirement and turns a safe delete of one resource into an outage for another resource.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:524-525` requires confirmation for new public ports and removal only when the last resource using the public port is removed.
- `src/Hashi.Infrastructure/Platform/TraefikEntryPointService.cs:20-37` looks up a single entry point by port/protocol and then overwrites `existing.ResourceId` with the current resource id.
- `src/Hashi.Infrastructure/Platform/TraefikEntryPointService.cs:39-42` deletes entry points only by `ResourceId`, without checking whether another enabled stream resource still uses that same port/protocol.
- `src/Hashi.Infrastructure/Platform/PlatformServices.cs:274-291` calls `entryPoints.RemoveForResourceAsync(id)` during resource deletion before removing the resource.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:53-58` filters TCP/UDP resources by confirmed `(EffectivePublicPort, protocol)` keys.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:98-106` renders stream entry points only from the filtered stream resource list.

## Expected outcome

Public-port confirmation should be keyed by the public port and protocol, and it should remain active until no enabled resource uses that public port/protocol anymore. Removing a resource must not remove the shared entry point or firewall opening while another resource still depends on it.

## Fix guidance

Stop treating `TraefikEntryPointEntity.ResourceId` as the sole owner of a public port confirmation. Either model confirmation as a port/protocol-level object and derive active users from resources, or model many resource-port users separately. On resource delete/update/disable, count remaining enabled TCP/UDP resources with the same effective public port and protocol before removing or queuing removal of the entry point.

Add regression coverage for two TCP or UDP resources sharing a public port, deleting one of them, and verifying that the confirmed entry point and rendered Traefik entry point remain until the last user is removed.

## Acceptance criteria

- Two enabled stream resources can share the same public port/protocol without either one taking ownership of the confirmation away from the other.
- Deleting, disabling, or changing one of several users of a shared public port does not remove the confirmed entry point.
- Removing the final enabled user of a public port queues or performs the entry-point/firewall-opening removal only through the confirmation path required by the spec.
- Traefik rendering continues to include remaining stream resources after one shared-port resource is deleted.
- Tests cover shared-port create, update, delete, and render behavior.
