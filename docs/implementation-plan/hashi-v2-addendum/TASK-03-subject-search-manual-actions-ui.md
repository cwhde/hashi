# TASK-03: Subject Search, Manual Actions, and Timeline UI

## Goal

Build the incident-response workflow from the addendum: search any security subject, inspect effective decision/history/events/buckets, and perform audited manual allow/block/block-duration actions.

## Spec Context

- Addendum sections: 5.1, 5.3, 5.4, 11.2, 11.3, 12.1, 12.2, 12.3, 16.2, 16.3, 18.

## Current Code Anchors

- Current security page: `web/src/routes/(admin)/security/+page.svelte`
- Security dashboard contracts: `src/Hashi.Contracts/Api/PlatformContracts.cs`
- Security endpoints: `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs`
- Auth/reauth middleware: `src/Hashi.Api/Hosting/AdminApiAuthMiddleware.cs`
- Frontend reauth helper: `web/src/lib/auth/reauth.ts`
- Shared layout/components: `web/src/lib/components/layout/`, `web/src/lib/components/overview/OverviewWidget.svelte`
- API client: `web/src/lib/api/client.ts`

## API Deliverables

Add endpoints:

- `GET /api/security/subjects/search?q=...`
- `GET /api/security/subjects/{id}`
- `GET /api/security/subjects/{id}/events`
- `GET /api/security/subjects/{id}/buckets`
- `GET /api/security/subjects/{id}/effective-decision`
- `POST /api/security/manual-entries`
- `PATCH /api/security/manual-entries/{id}`
- `DELETE /api/security/manual-entries/{id}`
- `POST /api/security/manual-entries/{id}/expire`
- `POST /api/security/blocks`
- `PATCH /api/security/blocks/{id}`
- `POST /api/security/blocks/{id}/extend`
- `POST /api/security/blocks/{id}/shorten`
- `POST /api/security/blocks/{id}/make-permanent`
- `POST /api/security/blocks/{id}/expire`
- `POST /api/security/blocks/{id}/preview-firewall-sync`

All unsafe endpoints require CSRF. High-risk actions require recent reauthentication.

## UI Deliverables

Extend the Security route into:

- Dashboard summary widgets:
  - Top challenged IPs.
  - Top blocked IPs.
  - Recent manual actions.
  - Blocklist matches over time.
  - CAPTCHA solved/failed/ignored.
  - Active soft/firewall blocks.
  - Stale blocklist sources.
  - Stale GeoIP/ASN database status.
- Global search that accepts IP, CIDR, ASN, country, region, and text over recent events.
- Subject detail view with:
  - current effective decision
  - matching manual allow/block entries
  - matching blocklist entries
  - matching resource rules
  - active adaptive/challenge/soft/firewall states
  - ban/challenge history
  - request bucket summary
  - firewall application state
  - available actions
  - chronological timeline with filters

Use dense operational layouts and the existing shadcn-svelte/Bits/Tailwind style. Avoid nested cards.

## Manual Actions

Supported from subject detail:

- Add/remove manual allow.
- Add/remove manual block.
- Convert temporary block to permanent.
- Shorten or extend duration.
- Expire immediately.
- Escalate soft block to firewall block for IP/CIDR.
- De-escalate firewall block to soft block.
- Trigger firewall sync preview for firewall-affecting changes.
- Show affected resources before applying.

Firewall-affecting actions must produce or attach to a firewall sync plan.

## Tests

- API tests for search, detail, timeline, effective-decision, manual entries, and blocks.
- Middleware tests for reauth paths:
  - permanent manual block
  - firewall block
  - firewall sync preview/apply
- Frontend unit tests for search parsing and timeline filtering.
- E2E tests:
  - search IP and view timeline
  - manually block IP
  - manually allow IP
  - extend/shorten ban

## Acceptance

- A user can search a subject and understand exactly why Hashi allows, challenges, soft-blocks, or firewall-blocks it.
- All manual actions produce audit events.
- Manual allow still does not bypass SSO or CAPTCHA by default.
