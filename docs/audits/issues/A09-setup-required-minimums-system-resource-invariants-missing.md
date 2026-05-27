# A09 - Setup completion and resource invariants miss required minimums

Priority: High

Spec conflicts: non-negotiable rules 10, 11, 12, 13, and 21; setup sections 7 and 24.

## Problem

Setup completion checks passkey, recovery vault, vault unlock, and HTTPS verification, but it does not enforce the required minimums from the spec: one DNS provider, one Traefik connection, and one Linux firewall host after setup completes.

System resources cannot be disabled or deleted, but they can be edited through the normal resource update path. That violates the rule that system resources can be edited only through their owning workflow. The resource entity also does not carry enough ownership/sync metadata to enforce system/imported/managed/user-created behavior robustly.

## Evidence

- `src/Hashi.Infrastructure/Auth/SetupCompletionService.cs:14-54` completes setup without checking DNS, Traefik, or firewall connection counts.
- `src/Hashi.Infrastructure/Persistence/Entities/DnsEntities.cs:95-100` defines only DNS provider, Traefik host, and firewall host connection type names.
- `src/Hashi.Infrastructure/Platform/PlatformServices.cs:67-70` blocks disabling a system resource.
- `src/Hashi.Infrastructure/Platform/PlatformServices.cs:72-181` then allows broad field updates on system resources.
- `src/Hashi.Infrastructure/Platform/PlatformServices.cs:207-209` blocks system resource deletion only.
- `src/Hashi.Infrastructure/Persistence/Entities/PlatformEntities.cs:3-48` has only `IsSystem` and lacks owning workflow/sync state fields.

## Expected outcome

Setup must not complete until required post-setup connections exist and are validated. System resources must be editable only by their owning setup/system-resource workflows. Required connection types cannot be deleted below minimum counts.

## Fix guidance

Add completion checks for required connection counts and health. Add owner/managed/imported/user-created metadata for resources. Gate system resource updates by an internal workflow method or explicit owner token. Add connection deletion/minimum enforcement before exposing delete operations later.

## Acceptance criteria

- Setup completion fails without at least one DNS provider, Traefik host, and firewall host.
- Normal `/api/resources/{id}` update cannot alter system resources outside the owning workflow.
- Tests cover required minimum failures and system-resource update rejection.
