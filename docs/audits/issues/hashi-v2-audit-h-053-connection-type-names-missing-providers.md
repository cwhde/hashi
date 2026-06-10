# H-053: ConnectionTypeNames Missing AdGuard, OIDC, and Notification Provider Constants

**Priority:** Low
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §6

**Status:** Not Started
**Branch:** 

## Description

The `ConnectionTypeNames` constants in `DnsEntities.cs` only define three connection types:

```csharp
public static class ConnectionTypeNames
{
    public const string DnsProvider = ConnectionTypeContractNames.DnsProvider;
    public const string TraefikHost = ConnectionTypeContractNames.TraefikHost;
    public const string FirewallHost = ConnectionTypeContractNames.FirewallHost;
}
```

The spec §6 lists these connection types:
- DNS provider ✓
- Traefik host ✓
- Linux firewall host ✓
- **AdGuard Home instance** ✗
- **OIDC SSO provider** ✗
- **Notification provider** ✗
- NetBird management connection (optional) ✗

While the entities for these connection types exist (e.g., `AdGuardConnectionEntity`, `OidcProviderEntity`, `NotificationProviderEntity`), their types are not represented in the shared `ConnectionTypeNames` constants, making it impossible to differentiate them in a unified connection-type field or generic connection queries.

This is primarily a code completeness issue — the connection types exist as separate entity tables rather than discriminated `Type` field values. Whether this is by design (separate tables per provider) or an oversight is unclear, but the missing type constants create an inconsistency.

## Evidence

```csharp
// DnsEntities.cs:167-172 — only 3 types defined
public static class ConnectionTypeNames
{
    public const string DnsProvider = ...;
    public const string TraefikHost = ...;
    public const string FirewallHost = ...;
    // Missing: AdGuardHome, OidcProvider, NotificationProvider, NetBird
}
```

## Expected Outcome

Either:
1. Add missing connection type constants (`AdGuardHome`, `OidcProvider`, `NotificationProvider`, `NetBirdManagement`)
2. Or explicitly document that these providers use separate entity tables and don't participate in the `Type`-based connection discrimination

## Fix Guidance

1. Add constants for all connection types listed in spec §6.
2. Ensure they are used consistently wherever connection types are referenced.
3. Align with `ConnectionTypeContractNames` in the contracts project.

## Acceptance Criteria

- [ ] All spec-listed connection types have corresponding constants
- [ ] Constants are used consistently in entity hydration, validation, and API responses
- [ ] Or: documentation explains the separate-entity pattern for optional providers
