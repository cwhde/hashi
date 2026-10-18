# AUD-047: SSH connection UI omits private-key authentication

Priority: P2 — administrators cannot configure a supported connection type through the normal UI

Category: Missing feature / SSH connection setup

Spec reference: `docs/implementation-spec/hashi-v2-implementation-spec.md` §6.3 requires SSH connections to support passwords, private keys, and encrypted private keys.

## Finding

The connections page only renders host, port, username, and password inputs. Its form state defaults `authMode` to `password`, and the create handler sends private-key fields only if that value is `private_key`, but no control can change the mode and no private-key or passphrase input is rendered. The API and infrastructure support private-key authentication, but an administrator using the shipped UI cannot create such a connection.

## Evidence

- `web/src/routes/(admin)/connections/+page.svelte`: `form.authMode` is initialized to `password`; the form renders only a password field and never binds an authentication-mode selector or private-key fields.
- `create()` conditionally includes `privateKeyPem` only for `private_key`, an unreachable state from this page.
- `docs/implementation-spec/hashi-v2-implementation-spec.md` §6.3 explicitly lists password, private key, and private-key passphrase support.
- `src/Hashi.Core/Validation/RequestValidators.cs` and `src/Hashi.Infrastructure/Connections/ConnectionSshCredentialResolver.cs` support SSH auth modes beyond password.

## Expected outcome

Expose the supported authentication modes in the SSH connection form, with mode-specific credential fields and validation. Keep private-key material out of connection list/detail responses and ensure it is stored in the vault.

## Acceptance criteria

- The UI lets the administrator choose password or private-key authentication.
- Private-key mode exposes the PEM and optional passphrase fields and submits the matching auth mode.
- Password mode does not submit stale private-key values; private-key mode does not submit stale password values.
- Credentials are stored and returned according to the secret-handling requirements in the implementation spec.
