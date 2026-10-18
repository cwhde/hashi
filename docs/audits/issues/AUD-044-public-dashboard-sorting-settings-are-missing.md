# AUD-044: Public dashboard sorting and its settings are missing

Priority: P2 — the required sort control cannot change ordering and no default-sort setting exists

Category: Missing feature / public dashboard

Spec reference: `docs/implementation-spec/hashi-v2-implementation-spec.md` §§20 and 24 require a public dashboard sort selector and dashboard settings for default sort and visibility defaults.

## Finding

`PublicDashboardView` renders a sort `<select>` with only the `name` option, but its `filtered` derived value always sorts alphabetically and never reads the `sort` state. The dashboard settings API stores only overview widget preferences (`OverviewWidgetsJson`); it has no default public sort or visibility-default setting. An administrator cannot choose the required default behavior, and public visitors cannot select a different order.

## Evidence

- `web/src/lib/components/public/PublicDashboardView.svelte`: `sort` is bound to the selector, the only option is `name`, and `filtered` unconditionally sorts by `displayName`.
- `src/Hashi.Contracts/Api/CommonResponses.cs`: `DashboardSettingsResponse` and request contain only `OverviewWidgetsJson` (plus response timestamp).
- `src/Hashi.Api/Features/Setup/SettingsEndpoints.cs`: dashboard settings read/write only `OverviewWidgetsJson`.

## Expected outcome

Implement the specified public sort modes and persist the configured default sort and visibility defaults. The selector must control the actual ordering.

## Acceptance criteria

- Public visitors can select the supported sort modes and the list updates accordingly.
- The configured default sort is returned and applied on page load.
- Visibility defaults are persisted and applied according to the specification.
- Selecting a sort mode does not mutate or reorder the backing API response unexpectedly.
