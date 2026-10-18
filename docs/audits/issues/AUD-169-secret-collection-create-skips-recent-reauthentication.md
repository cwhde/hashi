# AUD-169: Secret collection create skips recent reauthentication

Priority: P2

Category: Security / sensitive mutation authorization

Status: open; confirmed by manual middleware/endpoint trace.

## Finding and evidence

RequiresReauthentication recognizes `/api/vault/secrets/` with a trailing separator, while secret creation is POST `/api/vault/secrets`. That collection request falls through without recent reauthentication. It still requires ordinary authentication, secrets.manage and an unlocked session vault, so this is not anonymous secret storage or a scope bypass. It permits a stale privileged session to create secret material through a path omitted from the sensitive-operation policy. AUD-100 covers the separate trailing-slash GET reveal bypass.

Evidence: `src/Hashi.Api/Hosting/AdminApiAuthMiddleware.cs:107-109,165-168`; `src/Hashi.Api/Features/Vault/VaultEndpoints.cs:87-104`.

## Expected outcome and acceptance criteria

Use endpoint metadata/policies covering the complete sensitive operation rather than fragile string prefixes. Secret creation and reveal must enforce their intended scope and recent reauthentication consistently for equivalent routed forms, with explicit setup-only exceptions if required.
