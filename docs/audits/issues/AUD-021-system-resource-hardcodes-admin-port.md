# AUD-021: Setup-created admin resource hardcodes port 8080

Priority: P2 — custom-port deployment failure

Category: Wrong configuration / deployment correctness

Spec reference: implementation spec §§5.2 and 27 require configurable admin/public ports and a working admin-domain route.

## Finding

The API listens on the configured `HashiPortOptions.Admin` port, but `SystemResourceSetupService` creates the Hashi Admin resource with `TargetPort = 8080`. When the configured admin port differs from 8080, the generated Traefik route points at the wrong local port and the admin domain can fail even though Hashi is listening correctly. The system resource's existing-row update path changes its domain and ownership but does not correct the target port either.

## Evidence

- `src/Hashi.Api/Program.cs` configures Kestrel using `HashiPortOptions.Admin`.
- `src/Hashi.Infrastructure/Services/SystemResourceSetupService.cs`: new Hashi Admin resources use literal target port `8080`; existing system resources are not updated from runtime port configuration.
- `src/Hashi.Infrastructure/Platform/HashiInternalUrlResolver.cs` already uses the configured admin port for internal URLs, so the two routing paths can disagree.

## Expected outcome

Use the same configured admin port as Kestrel and internal service URL generation when creating or reconciling the system resource.

## Acceptance criteria

- Fresh setup with a non-default admin port creates the admin resource pointing to that port.
- Re-running system-resource sync repairs an existing stale `TargetPort`.
- Default-port configuration remains unchanged.
- Port configuration is tested through startup configuration, system-resource reconciliation, and rendered Traefik routing.
