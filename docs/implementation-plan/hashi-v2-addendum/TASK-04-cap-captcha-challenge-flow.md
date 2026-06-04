# TASK-04: Cap CAPTCHA Challenge Flow

## Goal

Integrate Hashi with an existing Cap Standalone instance for self-hosted CAPTCHA challenge flows.

Hashi must not deploy or manage Cap itself.

## Spec Context

- Original spec sections: 11, 13, 19, 21, 25.
- Addendum sections: 7, 11.1, 12.5, 13.5, 14, 16, 18, 19 Phase C.
- Research references: `RESEARCH-RESOURCES.md` Cap CAPTCHA section.

## Current Code Anchors

- Current forward-auth endpoint: `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs`
- Current edge auth service: `src/Hashi.Infrastructure/Platform/EdgeAuthService.cs`
- Current OIDC challenge/login flow: `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs`
- System resource setup: `src/Hashi.Infrastructure/Services/SystemResourceSetupService.cs`
- Resource service and sync: `src/Hashi.Infrastructure/Platform/PlatformServices.cs`, `src/Hashi.Infrastructure/Platform/TraefikSyncService.cs`
- App settings and secret storage: `CoreEntities.cs`, `SecretRecordService.cs`, `RuntimeSecretEligibility.cs`
- Frontend routes: `web/src/routes/`

## Backend Deliverables

Add Cap integration settings:

- Enable flag.
- Cap public challenge base URL.
- Site key.
- Secret key stored as service-sync secret.
- Verification timeout.
- Optional headless/instrumentation expectation flag.
- Public challenge resource domain.
- Optional Cap admin resource domain.

Before implementation, re-check the Cap quickstart and standalone API docs linked in `RESEARCH-RESOURCES.md`. Confirm the current widget endpoint shape, siteverify request/response, token lifetime/single-use behavior, and the distinction between site key, key secret, API key, and dashboard admin key.

Add a Cap client:

- Verify tokens server-side.
- Redact secret details from logs/errors.
- Use timeout and clear error classes for unavailable Cap.

Add challenge endpoints:

- `GET /api/edge-challenge/start`
- `POST /api/edge-challenge/verify`
- `GET /api/edge-challenge/status`

Keep public challenge endpoints separate from admin endpoints.

Add challenge state lifecycle:

- Set `challenge_required = true`.
- Record reason and triggering buckets.
- Count requests while challenged.
- Count CAPTCHA page requests, verify attempts, failures, ignored challenge hits.
- Successful solve clears only active challenge state and resets/decays triggering buckets.
- Successful solve does not clear offense history, bypass SSO, or grant a free browsing window.

## Resource Deliverables

When CAPTCHA integration is enabled, create and maintain a required system resource for the public challenge flow:

- Required while CAPTCHA is enabled.
- Not deletable while CAPTCHA is enabled.
- Editable only through CAPTCHA settings/setup.
- Does not require SSO.
- Does not require CAPTCHA.
- Not subject to adaptive challenge rules.
- Still subject to hard firewall blocks.
- Exposes only public challenge flow paths.
- Uses a real reverse-proxy domain, not `hashi.home.arpa`.

Optional Cap admin dashboard resource:

- Normal resource.
- User configurable and deletable.
- Real reverse-proxy hostname.
- Strongly recommend SSO/admin-only access.
- Do not require CAPTCHA by default to avoid loops.

## Browser and API Behavior

Browser-like requests:

- Redirect to the Hashi challenge page.
- Preserve original URL.
- After successful challenge, redirect back if safe and same-origin/resource-safe.
- Fall back to resource root for unsafe/cross-origin return URLs.

API-like requests:

- Return `403` or `429`.
- Include machine-readable `challenge_required`.
- Do not blindly redirect based on `Accept`/request type.

Challenge page:

- Minimal assets.
- No SSO requirement.
- No protected API calls.
- Shows attempted resource/domain.
- Submits Cap token to Hashi, not directly to upstream services.
- Never exposes Cap secret key/admin API key.

## Frontend Deliverables

- Challenge page/route.
- Setup fields in optional setup.
- Settings panel for Cap integration and secret rotation.
- Clear distinction between required CAPTCHA public resource and optional Cap admin resource.

## Tests

- CAPTCHA verify success/failure.
- Cap unavailable behavior.
- Browser redirect vs API response behavior.
- Challenge solve clears active challenge only.
- No default 60-minute free pass.
- Continued protected-resource hits while challenged escalate.
- Required challenge resource cannot be deleted while CAPTCHA is enabled.
- Challenge resource does not require SSO/CAPTCHA and is still subject to firewall hard blocks.

## Acceptance

- Challenged subjects cannot reach normal protected upstream resources.
- CAPTCHA public flow remains reachable.
- Successful solve only clears current challenge state.
- Continued spam escalates to soft/firewall block according to policy.
