# AUD-129: Duplicate route priorities collide in generated router and service names

Priority: P2

Category: Routing / route identity

Status: open; confirmed by manual source trace.

## Finding

Routes are persisted without a unique priority constraint or duplicate validation. Generated names use only resource slug and Priority. Two routes with equal priority emit duplicate YAML router/rewrite keys, while services are DistinctBy that same name and one target is silently discarded. The editor can submit multiple default-priority routes, so this is not limited to direct database corruption.

## Evidence

src/Hashi.Infrastructure/Platform/PlatformServices.cs:450-477; Persistence/HashiDbContext.cs route index is not unique; src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:251-267,297-313,341-342.

## Expected outcome and acceptance criteria

Give routes stable unique identities in generated names, or enforce and explain a unique-priority rule at every write boundary. Equal match priorities must not silently drop a backend or create invalid YAML.
