# AUD-137: AdGuard and OIDC selection use raw domains instead of resolved resource domains

Priority: P2

Category: Routing / resolved resource domains

Status: open; confirmed by manual source trace.

## Finding

AdGuard topology chooses Resource.Domain verbatim when nonempty and otherwise always slug.rootDomain. It does not apply ResourceDomainResolver, so root mode and relative subdomain mode can create the wrong rewrite. OIDC login provider selection similarly compares the absolute return host only to stored Domain, which can be null or a relative label; it then falls back to another default provider despite the resource's explicit provider. The Traefik definition uses the resolver, so these components disagree about the same resource identity.

## Evidence

src/Hashi.Infrastructure/Platform/AdGuardSyncService.cs:643-645; src/Hashi.Api/Features/EdgeAuth/EdgeAuthEndpoints.cs:126-138; PlatformServices.cs:851. AUD-070 separately covers accepting an already-issued session from the wrong provider.

## Expected outcome and acceptance criteria

Use one canonical domain/host resolution path throughout routing, DNS, provider selection and previews. Root, relative-subdomain and custom-FQDN resources must generate matching rewrites and select their configured provider.
