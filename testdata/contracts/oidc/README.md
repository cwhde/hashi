# OIDC: Pocket ID

captured_at: 2026-11-01 · tool_version: Pocket ID 2.19.1 (the same image digest the real server runs) ·
docs_url: https://openid.net/specs/openid-connect-core-1_0.html

Captured from a throwaway local container, never from the real instance. The real server's discovery document was read
over HTTPS and has the same shape.

Findings:

1. `state` under 8 characters is refused with a redirect carrying `error=invalid_state`. So is a PKCE verifier under 43.
2. A signed-out browser gets `302 /interaction?...`, a signed-in one `302 <callback>?code=…&iss=…&scope=…&state=…`.
3. ID tokens are RS256 with a `kid`. `groups` is an array of group **names**; the username is `preferred_username`.
4. Bad code or verifier: `400 {"error":"invalid_grant",…}`. Bad client: `401 {"error":"invalid_client",…}`.
5. Client secret authentication: `client_secret_basic` and `client_secret_post` both offered.
