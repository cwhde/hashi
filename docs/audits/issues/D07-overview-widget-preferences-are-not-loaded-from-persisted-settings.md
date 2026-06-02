# D07 - Overview widget preferences are not loaded from persisted settings

Priority: Low

Spec conflicts: section 21 requires overview widgets to be toggled and reordered in settings. Section 24 lists dashboard settings including default sort and visibility defaults.

## Problem

The dashboard settings API can now persist overview widget JSON, and the settings page saves widget toggles to that API. The actual Overview page, however, initializes widget preferences only from local storage and never fetches the persisted dashboard settings. A layout saved in one browser/device therefore does not apply when viewing the Overview page from another browser/device unless the settings page is opened first.

This is a residual version of the earlier widget persistence gap: persistence exists, but the main consumer still treats local storage as the source of truth.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:1154-1167` requires overview widgets to be toggleable/reorderable in settings and lists the default widgets.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:1281-1284` lists dashboard settings, including default sort and visibility defaults.
- `src/Hashi.Api/Features/Setup/SetupAdvanceEndpoints.cs:278-305` exposes `/api/settings/dashboard` for persisted `OverviewWidgetsJson`.
- `web/src/routes/(admin)/settings/+page.svelte:52-58` loads dashboard settings, and `web/src/routes/(admin)/settings/+page.svelte:82-97` saves widget changes to both local storage and the dashboard settings API.
- `web/src/lib/overview/widgets.ts:27-38` falls back to local storage when parsing settings JSON is missing or invalid.
- `web/src/lib/overview/widgets.ts:55-69` reads and writes only `localStorage` for `loadWidgetPrefs`/`saveWidgetPrefs`.
- `web/src/lib/components/admin/AdminOverviewView.svelte:8-12` initializes `prefs` with `loadWidgetPrefs()`.
- `web/src/lib/components/admin/AdminOverviewView.svelte:24-62` fetches dashboard data, health, resources, monitors, security, DNS, Pulse, and sync runs, but not `/api/settings/dashboard`.
- `web/src/lib/components/admin/AdminOverviewView.svelte:138-140` still tells users that widget layout is stored locally until the settings API ships.

## Expected outcome

The Overview page should load persisted dashboard widget settings directly and use local storage only as a cache or offline fallback.

## Fix guidance

Fetch `/api/settings/dashboard` when the Overview page mounts, parse `OverviewWidgetsJson`, and update `prefs` from the API response. Keep local storage as a fallback/cache after the API result, not the canonical source. Remove the stale local-only footer text.

## Acceptance criteria

- Overview widget preferences saved in Settings are reflected on the Overview page in a fresh browser session.
- Local storage is used only when the API is unavailable or no persisted settings exist.
- The stale "stored locally until settings API ships" copy is removed.
- Tests cover settings save, overview load from API, and offline fallback behavior.
