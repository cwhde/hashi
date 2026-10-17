# UI-001 - Required UI design foundation is documented but not implemented

Priority: Medium

Category: Missing feature / specification compliance

Specification reference: `docs/implementation-spec/hashi-v2-ui-design-foundation-addendum.md`, especially sections 4, 6, 7, 18, 20, 23, and 24.

## Problem

The UI design addendum is merged into `main`, but the frontend still uses the earlier binary theme and shared styles. The required four named themes, semantic token system, shared primitives, representative visual scenes, and visual tests have not been implemented. The result is a functional admin interface that does not yet meet the approved design foundation.

This is a user-facing completeness and consistency gap. It ranks below missing operational workflows and high-impact safety defects, but should be scheduled after the app's runtime/install path has been verified.

## Evidence

- The design addendum defines Graphite, Verdant, Warm Linen, and Violet Night themes and requires runtime theme selection and semantic design tokens.
- Its section 23.6 calls out the current binary `:root`/`.dark` model, remote default background, shared hard-coded color classes, and chart colors for correction.
- Its acceptance criteria require shared primitives and visual validation across themes and viewport classes.
- Current frontend searches show no `data-theme` implementation; shared app styles still include the earlier Hashi palette and theme conventions.
- The merged `ui/redesign-foundation` branch contains the design addendum only; it does not contain frontend implementation changes.

## Expected outcome

Implement the shared theme tokens and primitives, migrate representative pages, and validate the required overview, resource, settings, analytics, and narrow-viewport scenes without changing domain behavior or API contracts.

## Acceptance criteria

- All four named themes work through semantic tokens and persist the selected theme.
- Shared shell, navigation, panel, table, form, status, and chart primitives match the addendum.
- Required representative scenes render and remain usable at their specified viewports.
- Theme and responsive visual tests cover the shared primitives and representative scenes.
- Existing domain behavior and API contracts remain intact.
