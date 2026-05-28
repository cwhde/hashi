# B11 - Settings surface and widget persistence are incomplete

Priority: Medium

Spec conflicts: section 24 and the overview/dashboard requirements in sections 20, 21, and 22. Settings must cover all listed categories, and overview widget preferences must be persisted rather than local-only.

## Problem

The backend settings API exposes only general settings, monitoring settings, and Edge SSO session length. The settings page exposes General, Overview Widgets, and Notifications, and it explicitly states that widget preferences are stored locally until an API ships.

Most spec categories are missing or only partially represented: Security, Appearance asset overrides, Dashboard display, DNS, Traefik, Firewall, Notifications cooldown/routing defaults, and Pulse defaults/staleness thresholds.

## Evidence

- `src/Hashi.Api/Features/Setup/SetupAdvanceEndpoints.cs:108-229` maps only `/api/settings/general`, `/api/settings/monitoring`, and `/api/settings/edge-sso/session`.
- `src/Hashi.Infrastructure/Persistence/Entities/CoreEntities.cs:3-39` has a small `AppSettingsEntity` without fields for most settings categories listed in the spec.
- `web/src/routes/(admin)/settings/+page.svelte:74-78` describes the page as general preferences, overview widgets, and asset overrides, but the page does not implement broad settings categories.
- `web/src/routes/(admin)/settings/+page.svelte:123-124` says overview widgets are "stored locally until settings API ships".
- `web/src/lib/overview/widgets.ts:20` defines a browser `localStorage` key.
- `web/src/lib/overview/widgets.ts:32-44` loads and saves widget preferences only through `localStorage`.

## Expected outcome

The settings API and UI should cover the categories in the spec, and dashboard/overview preferences should persist in PostgreSQL so they work across browsers and devices.

## Fix guidance

Add typed settings models and endpoints for the missing categories. Move overview widget configuration from `localStorage` into persisted settings. Keep local storage only as a temporary cache if needed, not the source of truth.

## Acceptance criteria

- Settings categories exist for General, Security, Appearance, Monitoring, Dashboard, DNS, Traefik, Firewall, Notifications, and Pulse.
- Overview widget visibility/order persists in the database.
- Reloading on another browser or device shows the same widget preferences after login.
- Tests cover backend persistence for the new settings categories and frontend save/load behavior for widgets.
