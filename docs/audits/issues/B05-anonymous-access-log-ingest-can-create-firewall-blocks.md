# B05 - Anonymous access-log ingest can create firewall blocks

Priority: Critical

Spec conflicts: non-negotiable rules 7, 18, 20, and 21; sections 12, 13, 14, and 30. Security signals must come from trusted sources, high-risk firewall changes require safe orchestration, and admin-affecting writes must not be anonymous.

## Problem

`POST /api/security/access-log` is explicitly anonymous and bypasses the admin auth middleware. Each request increments the abuse score for the supplied client IP. Once the score reaches the block threshold, Hashi inserts a blocklist entry and attempts to sync it to all firewalls.

Any network client that can reach the API can therefore submit fake access-log entries for a victim IP and cause Hashi to challenge or block that IP.

## Evidence

- `src/Hashi.Api/Hosting/AdminApiAuthMiddleware.cs:32` bypasses auth for `IsAccessLogIngest`.
- `src/Hashi.Api/Hosting/AdminApiAuthMiddleware.cs:210-211` defines that bypass as `/api/security/access-log`.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:520-524` maps `POST /api/security/access-log` and calls `.AllowAnonymous()`.
- `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:29-34` increments score and derives `watch`, `challenge`, or `block` state from the request.
- `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:115-127` creates a `BlocklistEntryEntity` when the state is `block`.
- `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:132-134` then calls firewall blocklist sync.

## Expected outcome

Access-log ingestion must only accept events from trusted local workers, authenticated Traefik integrations, or signed/scope-limited ingestion tokens. Untrusted public traffic must not be able to manufacture security events or firewall blocks.

## Fix guidance

Remove the anonymous public API path or protect it with a dedicated ingestion token, mTLS, loopback-only binding, or another trusted-channel mechanism. Keep the SSH access-log worker as the default ingestion source. Add rate limits and audit metadata for accepted ingestion sources.

## Acceptance criteria

- Anonymous requests to `/api/security/access-log` are rejected.
- A trusted ingestion path can still submit access-log events.
- Forged access-log requests cannot create blocklist entries.
- Tests cover anonymous rejection, trusted ingestion, and blocklist threshold behavior.
