# AUD-048: Firewall host manager UI is incomplete

Priority: P2 — persisted firewall hosts cannot be maintained through the normal admin UI

Category: Missing feature / firewall host management

Spec reference: implementation spec §14 defines desired state for each Linux firewall host and requires generated firewall configuration to be managed and synchronized.

## Finding

The `/firewall-hosts` page lists configured hosts and offers a form that renders a firewall script from unsaved local values. It does not expose create or update for persisted hosts, host-specific plans, apply, or rollback. Those API endpoints exist, and setup creates the initial host, but after setup an administrator cannot add another host or edit persisted host configuration in the UI. The page's “Render firewall script” action does not save a host and its generated script is not the persisted host plan.

## Evidence

- `web/src/routes/(admin)/firewall-hosts/+page.svelte`: `loadHosts()` only lists; `render()` calls `api.renderFirewall()` with local form fields. No host create/update/plan/apply/rollback calls are present.
- `web/src/lib/api/client.ts`: host create/update and plan endpoints are available to the frontend client.
- `src/Hashi.Api/Features/Firewall/FirewallEndpoints.cs`: defines POST `/api/firewall/hosts`, PUT `/api/firewall/hosts/{id}`, POST host plan, POST apply, and POST rollback.
- `docs/implementation-spec/hashi-v2-implementation-spec.md` §14 describes per-host persisted desired state and generated/synchronized firewall artifacts.

## Expected outcome

Provide a persisted host management workflow that distinguishes draft rendering from the saved host's plan and guarded apply/rollback operations.

## Acceptance criteria

- Administrators can create, edit, and inspect persisted firewall hosts after setup.
- A selected host's plan is derived from persisted desired state and clearly shows changes before apply.
- Apply and rollback are reachable with the required confirmation, lockout protection, and reauthentication behavior.
- Rendering a draft does not imply that it is saved or applied.
