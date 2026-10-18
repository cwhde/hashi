# AUD-058: Traefik connection apply and rollback skip recent reauthentication

Priority: P1

Category: Security / privileged operation authorization

Spec reference: Base §§9,25; admin-session addendum §§10,11.

## Finding

The UI uses POST /api/traefik/connections/{id}/apply and /rollback. RequiredScope recognizes apply but RequiresReauthentication recognizes only /api/traefik/apply, /sync/apply, and firewall paths, so these actual Traefik connection operations run without recent passkey approval. /api/traefik/rollback and /install likewise fall through. Rollback also gets ordinary admin.write rather than sync.apply because its path lacks /apply.

## Evidence

- src/Hashi.Api/Features/Traefik/TraefikEndpoints.cs:71-101 registers connection/direct apply,rollback,install.
- src/Hashi.Api/Hosting/AdminApiAuthMiddleware.cs:88-143 and145-263 omits these sensitive route shapes from scope/reauth rules.

## Expected outcome

Attach explicit scope and recent-reauth requirements to the actual endpoint metadata rather than fragile path-name matching.

## Acceptance criteria

- Both connection and direct apply/rollback/install require the intended privileged scope and recent passkey assertion.
- Ordinary write-only sessions cannot replace remote proxy configuration.
- Canonical route aliases and trailing slashes enforce the same policy.

Verification: confirmed by manual source tracing; no live firewall/provider change was performed.
