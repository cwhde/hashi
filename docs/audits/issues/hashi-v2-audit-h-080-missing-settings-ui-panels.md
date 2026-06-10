# H-080: Missing Settings UI Panels

**Priority:** High
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §24

**Status:** Not Started
**Branch:** 

## Description

The settings page only renders panels for General, GeoIP, Internal Agent DNS, CAPTCHA, Overview Widgets, and Notifications. Missing panels: Security (session duration, edge SSO, adaptive auth defaults, WAF defaults, block TTLs, GeoIP updates), Appearance (theme, logo, icon, public page assets, widget order), Monitoring (check interval, timeout, allowed HTTP codes, latency thresholds, retention), DNS (default TTL, prune policy, import behavior), Traefik (config paths, log paths, ACME defaults, middleware editor), Firewall (trusted CIDRs, default port confirmation, persistence mode, NetBird settings), and Pulse (heartbeat interval, stale threshold). Backend APIs exist for some of these (e.g., `/api/settings/monitoring`, `/api/settings/edge-sso/session`) but the frontend never calls them.

## Evidence

- `web/src/routes/(admin)/settings/+page.svelte` only renders 6 `PanelSection` components
- Backend has settings endpoints for monitoring and edge SSO that are never called from the frontend

## Expected Outcome

All 10 spec-defined settings categories have dedicated UI panels with typed form controls, not raw JSON editors.

## Fix Guidance

1. Add `PanelSection` components for each missing category.
2. Each should call the appropriate backend endpoint and render typed form controls.
3. Start with Security, Monitoring, and Firewall as the most operationally important.

## Acceptance Criteria

- [ ] Settings page shows all 10 spec-defined categories
- [ ] Each category has typed form controls (not raw JSON)
- [ ] Changes are persisted via the appropriate backend endpoints
