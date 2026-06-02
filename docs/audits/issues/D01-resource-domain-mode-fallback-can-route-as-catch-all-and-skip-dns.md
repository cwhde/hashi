# D01 - Resource domain fallback can route as catch-all and skip DNS

Priority: High

Spec conflicts: section 6 requires explicit resource domain modes: root, subdomain, and full custom domain. Section 15.4 requires resources without a detected Linux firewall host to require a manual IP or Pulse target and then generate the correct CNAME or A/AAAA record.

## Problem

Resources do not have a domain-mode field, and the create path accepts a blank domain. Blank domains then split across incompatible behavior: Traefik renders the resource as a catch-all host router, Edge forward auth cannot look the resource up by domain, public dashboard hides it, and DNS desired-state generation skips it before the DNS generator can apply the slug/root-domain fallback.

This means a no-domain resource can become publicly routed for every host while not receiving the resource-specific DNS, dashboard, or auth/rule behavior that the spec expects from explicit root/subdomain/full-custom-domain modes.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:153-159` lists domain mode as a common resource field.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:851-865` defines the resource DNS behavior for detected hosts, manual IP targets, and Pulse targets.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:68-87` defines `CreateResourceRequest` with only nullable `Domain`; there is no domain-mode field.
- `src/Hashi.Core/Validation/RequestValidators.cs:6-18` validates name, kind, target scheme, target host, target port, and public port, but not domain mode or blank-domain target requirements.
- `web/src/routes/(admin)/resources/+page.svelte:95-99` sends `domain: form.domain || null`, and `web/src/routes/(admin)/resources/+page.svelte:342-343` exposes only a free-form Domain input.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:335-349` renders blank domains as a `HostRegexp` rule matching `{host:.+}`.
- `src/Hashi.Infrastructure/Platform/EdgeAuthService.cs:29-32` only finds resources where `Domain` is non-null and exactly matches the forwarded host.
- `src/Hashi.Infrastructure/Dns/DnsDesiredStateBuilder.cs:54-56` filters generated DNS resources to `x.Enabled && x.Domain != null`.
- `src/Hashi.Core/Dns/DnsRecordGenerator.cs:124-134` can derive a fallback FQDN from `Slug` and `RootDomain`, but the desired-state builder never calls it for blank-domain resources.
- `src/Hashi.Infrastructure/Platform/PublicDashboardService.cs:82-88` drops resources with blank domains from the public app dashboard.

## Expected outcome

Resource domain mode should be explicit and should drive Traefik, DNS, Edge auth lookup, and dashboard URLs consistently. Blank/free-form domain input should not produce a catch-all public router.

## Fix guidance

Add a persisted domain-mode field or equivalent typed representation. Validate create/update requests so root/subdomain/custom-domain modes have the required inputs and so no-domain resources cannot be published accidentally. Make Traefik, DNS desired-state, Edge auth, and dashboard URL generation all resolve the same canonical public resource host.

## Acceptance criteria

- Resource create/update exposes and validates root, subdomain, and full custom domain modes.
- A resource with an omitted domain cannot render a catch-all `HostRegexp` router unless there is an explicit, validated wildcard mode.
- DNS desired-state generation produces the expected CNAME or A/AAAA record for root/subdomain resources, including Pulse/manual targets.
- Edge forward auth can find the resource by the same resolved public host used by Traefik.
- Public dashboard URLs match the resolved public host and do not silently hide a route that Traefik publishes.
- Tests cover blank-domain rejection or explicit fallback behavior, DNS generation, Traefik rule rendering, and Edge auth lookup.
