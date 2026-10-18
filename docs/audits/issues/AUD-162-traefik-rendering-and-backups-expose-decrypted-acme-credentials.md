# AUD-162: Traefik rendering and backups expose decrypted ACME credentials

Priority: P1

Category: Security / secret disclosure and storage

Status: open; confirmed by manual end-to-end source trace.

## Finding

CertificateSetupService decrypts the EAB secret for Traefik options, and the renderer inserts its key ID/HMAC into StaticConfigYaml. GET `/api/traefik/render` returns that full static YAML under ordinary admin.read authorization, without the secret reveal scope, recent reauthentication or an unlocked admin vault. The same real static file is read into LastBackupStaticYaml and persisted as ordinary database text during apply. A read-only API session or a database backup can thus expose the decrypted EAB credential through paths outside the vault's intended controls. Remote file mode timing is separately AUD-003; this issue concerns API disclosure and database-at-rest copies.

## Evidence

- `src/Hashi.Infrastructure/Platform/CertificateSetupService.cs:127-145,169-178`: purpose-key decryption.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:639-645`: plaintext `hmacEncoded` in rendered config.
- `src/Hashi.Api/Features/Traefik/TraefikEndpoints.cs:15-28`: full static config returned.
- `src/Hashi.Api/Hosting/AdminApiAuthMiddleware.cs:88-100`: ordinary safe-method read scope.
- `src/Hashi.Infrastructure/Platform/TraefikSyncService.cs:86-91`: backup text; persisted host state has no encryption conversion for this value.

## Expected outcome and acceptance criteria

Separate secret-free review/preview DTOs from privileged deployment material, protect any explicit reveal with the intended controls and encrypt or remove secret-bearing database backup copies. Preserve a usable protected rollback path. With synthetic EAB credentials, ordinary read/preview APIs and plaintext database exports must not contain the HMAC; authorized provider deployment must still receive it through a restricted path.
