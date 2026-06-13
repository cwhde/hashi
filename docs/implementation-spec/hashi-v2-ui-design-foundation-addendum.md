# Hashi V2 Addendum: UI Design Foundation and Theme System

## 1. Purpose

This document defines the visual and interaction foundation for the Hashi V2 administrative and public interfaces.

The goals are:

1. Replace the current visual direction with a calm, modern, rounded operational interface.
2. Make dense infrastructure data easy to scan without making the application feel crowded.
3. Establish one consistent component language for navigation, tables, forms, panels, charts, overlays, and status presentation.
4. Provide four complete default themes and a controlled custom-theme system.
5. Make interactions feel immediate while still using restrained motion to clarify state changes.
6. Preserve performance, accessibility, and maintainability as first-class design requirements.

This addendum supersedes the color and styling requirements in section 22, "Visual Design", of the base Hashi V2 implementation specification. It refines the existing frontend stack and UI rules but does not replace the product behavior, navigation list, security requirements, or domain workflows defined by the base specification and earlier addenda.

## 2. Scope

This addendum specifies:

1. Product-level design philosophy.
2. Visual hierarchy and density.
3. Rounded geometry and component shape rules.
4. Color tokens and the four default themes.
5. Custom theme behavior.
6. Typography, icons, controls, forms, tables, panels, and overlays.
7. Motion and interaction feedback.
8. Charts and data visualization.
9. Responsive behavior.
10. Accessibility and performance requirements.
11. The frontend libraries and implementation techniques to use.
12. Design-system acceptance criteria.

The following decisions are intentionally deferred to a later interface-architecture addendum:

1. The final page and route hierarchy.
2. The final navigation grouping and labels.
3. The exact contents of the Overview page.
4. Which information appears on each domain page.
5. The exact placement of every action, field, chart, and detail panel.
6. The migration order for individual existing pages.

## 3. Compatibility With Existing Specifications

The implementation must preserve these existing requirements:

1. Hashi remains a dense operational tool rather than a marketing dashboard.
2. The frontend uses exactly one main component system.
3. SvelteKit 5, TypeScript, Tailwind CSS v4, shadcn-svelte patterns, and Bits UI remain the primary frontend foundation.
4. Lucide remains the default icon set.
5. uPlot remains the default library for status, latency, security, and other time-series charts.
6. CodeMirror remains the editor for YAML and shell scripts.
7. Risky actions continue to expose consequences, previews, confirmation, results, and audit information as required by the base specification.
8. Cards are used only for repeated items, modals, bounded tools, or genuinely grouped information.
9. Nested cards, decorative gradient blobs, oversized promotional layouts, and ornamental dashboards remain prohibited.
10. Text, controls, charts, tables, and status indicators must remain usable at supported desktop and mobile widths.

## 4. Design Philosophy

### 4.1 Calm Operations

Hashi should feel like a precise control surface for infrastructure.

The interface must communicate:

* What is healthy.
* What needs attention.
* What changed.
* What action is available.
* What risk an action carries.

It must not compete with the data through decorative effects, oversized typography, excessive color, or unnecessary containers.

### 4.2 Overview Before Detail

Information needed for routine decisions should be visible without opening another screen.

Secondary configuration, explanations, histories, raw payloads, advanced fields, and uncommon actions should use progressive disclosure through:

* Expandable rows.
* Detail drawers.
* Popovers.
* Tabs inside a bounded tool.
* "Advanced" sections.
* Context menus.

Progressive disclosure must not hide current status, destructive consequences, validation failures, or required next actions.

### 4.3 Dense, Not Cramped

Hashi should fit meaningful operational information on screen, but density must come from hierarchy and alignment rather than tiny controls.

Required characteristics:

* Compact page headers.
* Stable table columns.
* Predictable row heights.
* Short labels.
* Clear section boundaries.
* Limited explanatory copy in routine views.
* Comfortable click targets.
* Consistent spacing increments.

### 4.4 Consistent Rounded Geometry

The selected visual direction is rounded.

Rounded geometry must be used consistently. The interface must not mix sharp table containers with highly rounded panels or alternate arbitrarily between square and pill-shaped controls.

Pills are reserved for semantics that are naturally compact and self-contained:

* Status labels.
* Filters.
* Segmented controls.
* Tags.
* Toggle tracks.

Large content panels must use rounded rectangles, not pill geometry.

### 4.5 Quiet Surfaces

Most hierarchy should come from spacing, typography, borders, and small surface changes.

The interface must avoid:

* Heavy drop shadows.
* Strong gradients.
* Neon glows.
* Glass effects on every panel.
* Multiple competing accent colors.
* Large decorative background images behind operational data.
* A separate visual treatment for every widget.

### 4.6 Immediate and Trustworthy

Every interaction should acknowledge input immediately.

The UI must:

* Update local control state in the same frame when safe.
* Show a pressed, selected, pending, success, or failure state without ambiguity.
* Preserve the user's position while details expand or collapse.
* Avoid full-page reloads for local operations.
* Avoid motion that delays access to information.
* Never imply that a remote operation succeeded before the API confirms it.

## 5. Visual References

The design direction may draw principles from the following products without copying their branding or page structure:

* Pocket ID: rounded controls, calm dark surfaces, restrained borders, progressive disclosure, and readable settings forms.
* Cloudflare Dashboard: dense navigation, strong tables, efficient inline editing, compact page headers, and operational hierarchy.
* Pangolin and NetBird: infrastructure-oriented navigation, searchable resource views, and clear primary actions.
* Gatus and Nezha: compact health summaries, status strips, and minimal time-series presentation.
* Claude: warm off-white and brown color relationships for the bright theme.
* Visual Studio dark themes: deep violet/navy foundations with controlled yellow highlights.

These references are inputs, not templates. Hashi must retain its own information model and must not inherit unnecessary complexity from any reference product.

## 6. Design Tokens

All visual styling must resolve through semantic design tokens. Components must not contain theme-specific color literals.

The minimum token groups are:

* Canvas and shell.
* Surface levels.
* Text levels.
* Borders and separators.
* Primary accent.
* Focus.
* Selection.
* Success.
* Warning.
* Danger.
* Informational state.
* Chart series.
* Scrim and overlay.
* Radius.
* Spacing.
* Motion duration and easing.

Theme names must be represented on the root element with a stable attribute such as \`data-theme\`. Component code consumes semantic variables such as \`--background\`, \`--surface-1\`, and \`--accent\`; it must not branch on theme names.

## 7. Geometry and Spacing

### 7.1 Radius Scale

The default radius scale is:

| Token | Value | Use |
| --- | ---: | --- |
| \`--radius-xs\` | 6px | Small badges, compact chips, chart tooltips |
| \`--radius-sm\` | 10px | Inputs, compact buttons, table inline controls |
| \`--radius-md\` | 14px | Standard buttons, menus, table containers |
| \`--radius-lg\` | 18px | Panels, repeated item cards, drawers |
| \`--radius-xl\` | 24px | Dialogs and major bounded tools |
| \`--radius-pill\` | 999px | Status pills, segmented controls, toggle tracks |

Components must use the nearest token. One-off radius values are prohibited unless a third-party primitive requires them.

### 7.2 Spacing Scale

Layout spacing must use a 4px base scale.

Common values are 4, 8, 12, 16, 20, 24, 32, and 40px. Larger gaps require a layout reason and must not be used to create artificial emptiness.

### 7.3 Stable Dimensions

Controls and repeated data structures must have stable dimensions:

* Compact icon button: 32 x 32px.
* Standard control height: 36px.
* Prominent control height: 40px.
* Compact table row: 40px minimum.
* Standard table row: 48px minimum.
* Primary navigation row: 36px minimum.
* Status indicator dot: 8px unless a specific visualization requires another size.

Loading, hover, selection, badges, and error content must not unexpectedly resize rows or toolbars.

## 8. Typography

Hashi uses a system sans-serif stack for all operational UI. The warm theme changes color and surface character, not the application into an editorial serif interface.

Requirements:

1. Body text defaults to 14px.
2. Dense metadata may use 12px but must retain sufficient contrast and line height.
3. Page titles should normally be 24 to 28px.
4. Panel titles should normally be 14 to 16px.
5. Table headers should be compact, medium weight, and visually quieter than row content.
6. Monospace is used for IP addresses, hostnames where alignment helps, ports, identifiers, hashes, paths, and code.
7. Letter spacing is zero.
8. Font size must not scale directly with viewport width.
9. Uppercase is reserved for very short category labels and must not be used for ordinary navigation or headings.

## 9. Color System

### 9.1 Theme Set

Hashi ships with four complete themes:

1. Graphite.
2. Warm Linen.
3. Verdant.
4. Violet Night.

Graphite is the default unless the user or deployment configuration selects another theme.

### 9.2 Graphite

Graphite is a neutral near-black theme with a clear blue accent.

| Semantic role | Value |
| --- | --- |
| Canvas | \`#08090B\` |
| Shell | \`#0D0F12\` |
| Surface 1 | \`#13161A\` |
| Surface 2 | \`#191D23\` |
| Elevated | \`#20252C\` |
| Border | \`#2B323B\` |
| Text | \`#F2F5F7\` |
| Muted text | \`#9DA6B2\` |
| Primary accent | \`#2563EB\` |
| Accent foreground | \`#FFFFFF\` |
| Accent hover | \`#3B82F6\` |
| Focus | \`#60A5FA\` |

Graphite should feel neutral, crisp, and minimally colored. Blue is used for navigation selection, primary actions, links, focus, and the principal chart series.

### 9.3 Warm Linen

Warm Linen is a bright theme inspired by warm paper, muted clay, and brown-gray interface chrome.

| Semantic role | Value |
| --- | --- |
| Canvas | \`#F3EEE5\` |
| Shell | \`#EAE1D4\` |
| Surface 1 | \`#FAF7F1\` |
| Surface 2 | \`#F0E7DA\` |
| Elevated | \`#FFFDFC\` |
| Border | \`#D4C7B6\` |
| Text | \`#2B251F\` |
| Muted text | \`#74695D\` |
| Primary accent | \`#B6533A\` |
| Accent foreground | \`#FFFFFF\` |
| Accent hover | \`#98432F\` |
| Focus | \`#D97757\` |

Warm Linen must not become yellow, beige-on-beige, low contrast, or ornamental. White and near-white surfaces remain available so data tables and forms retain crisp separation.

### 9.4 Verdant

Verdant is a very dark green theme with a clean yellow accent.

| Semantic role | Value |
| --- | --- |
| Canvas | \`#030A07\` |
| Shell | \`#06120D\` |
| Surface 1 | \`#0A1A12\` |
| Surface 2 | \`#10251A\` |
| Elevated | \`#173223\` |
| Border | \`#27513B\` |
| Text | \`#EDF7F0\` |
| Muted text | \`#94AA9B\` |
| Primary accent | \`#E8CE55\` |
| Accent foreground | \`#08110C\` |
| Accent hover | \`#F5DE74\` |
| Focus | \`#FFE99A\` |

Green surfaces provide atmosphere but must remain dark enough that success green is still distinguishable from the background. Yellow is used as the theme accent and must not be the sole indicator of warning state.

### 9.5 Violet Night

Violet Night is substantially darker than the original Hashi Shades of Purple theme. It uses near-black violet surfaces with a controlled yellow accent.

| Semantic role | Value |
| --- | --- |
| Canvas | \`#05030A\` |
| Shell | \`#0A0712\` |
| Surface 1 | \`#110C1D\` |
| Surface 2 | \`#191128\` |
| Elevated | \`#221638\` |
| Border | \`#382653\` |
| Text | \`#F4F0FA\` |
| Muted text | \`#AA9FBC\` |
| Primary accent | \`#E6CF62\` |
| Accent foreground | \`#120C1E\` |
| Accent hover | \`#F3DF82\` |
| Focus | \`#FFF0A8\` |

Violet must remain an undertone rather than saturating every component. Large surfaces should read as near-black first and violet second.

### 9.6 Semantic State Colors

Semantic status colors are independent from theme accents:

| State | Base value |
| --- | --- |
| Success | \`#22C55E\` |
| Warning | \`#F59E0B\` |
| Danger | \`#EF4444\` |
| Information | \`#38BDF8\` |

Each theme may adjust lightness slightly for contrast, but the meanings must remain stable.

Color must never be the only signal. Status presentation must combine color with text, icon, shape, or pattern.

The listed default text, muted-text, accent, and accent-foreground pairs must be verified against WCAG contrast calculations in automated theme tests. The palette values above are the baseline contract; implementation changes require equivalent or better contrast and must preserve the intended visual character.

## 10. Custom Themes

Hashi must allow a user-defined theme without exposing every internal token by default.

The basic custom-theme editor includes:

* Canvas color.
* Surface color.
* Text color.
* Muted text color.
* Border color.
* Primary accent.
* Optional chart accent set.
* Radius preference within the supported rounded range.
* Transparency preference.

Advanced token overrides may be exposed behind a separate advanced section.

Requirements:

1. The editor previews changes without committing them.
2. The user can cancel and return to the last saved theme.
3. The user can reset to any default theme.
4. Hashi validates text, control, focus, and status contrast before saving.
5. Invalid or incomplete custom themes must fall back safely rather than rendering unreadable UI.
6. Theme data must be versioned so future token additions can receive defaults.
7. Theme preference should be available before the main application paints to prevent a flash of the wrong theme.
8. Import and export may be supported as a small versioned JSON document.

## 11. Surfaces, Borders, and Transparency

### 11.1 Surface Hierarchy

Use at most these common surface levels in one view:

1. Canvas.
2. Shell or navigation.
3. Primary content surface.
4. Elevated interactive surface.

A border and a surface change should not both be strong. When the surface difference is sufficient, use a subtle border. When surfaces are nearly equal, use a clearer border.

### 11.2 Shadows

Shadows are reserved for overlays, menus, drawers, and dialogs.

Ordinary page panels and tables should use borders and surface contrast instead of floating-card shadows.

### 11.3 Transparency

Transparency is optional and restrained.

It may be used for:

* Sticky application chrome.
* Menus.
* Drawers.
* Dialogs.
* Small floating toolbars.

It must not be used on every panel or table row.

Recommended overlay surfaces use 88 to 96 percent opacity and no more than 16px backdrop blur. Hashi must provide an opaque fallback when backdrop filtering is unsupported or when the user enables reduced transparency.

## 12. Navigation and Page Chrome

This addendum does not redefine the navigation tree, but it defines its behavior and visual treatment.

Requirements:

1. Navigation remains visible and stable during routine work on desktop.
2. The active destination is indicated by a quiet filled background, an accent marker, or both.
3. Navigation groups use concise labels and predictable indentation.
4. The application header is compact and contains only persistent context and global actions.
5. Page titles and primary actions belong in the content header, not in oversized hero sections.
6. Mobile navigation uses an accessible drawer and preserves the same grouping and labels.
7. Expansion state may animate, but navigation must remain immediately interactive.

## 13. Panels, Cards, and Information Grouping

Page sections should generally be unframed layouts separated by spacing and headings.

Use a framed panel when:

* A tool has a clear boundary.
* A form must be grouped.
* A chart and its controls form one unit.
* A repeated item needs an independent action surface.
* A dialog or drawer needs containment.

Do not wrap every statistic or paragraph in a card.

Cards must not be nested inside other cards. If a panel needs internal organization, use:

* Separators.
* Subheadings.
* Rows.
* Tabs.
* Definition lists.
* Split layouts.

## 14. Tables and Repeated Data

Tables are the default for resources that users need to scan, sort, filter, compare, or edit in volume.

Requirements:

1. Column alignment remains stable during loading and updates.
2. Important identity and state columns remain visible before secondary metadata.
3. Row actions use one clear primary action or an overflow menu.
4. Inline editing expands within the table or uses a side drawer when the form is too large.
5. Expandable rows use a short transition and do not scroll the user unexpectedly.
6. Tables support empty, loading, error, filtered-empty, and partial-data states.
7. Dense tables allow optional user-controlled column visibility.
8. Horizontal overflow must be intentional and keyboard accessible.
9. Mobile layouts may convert a table into compact repeated rows only when the comparison value of columns would otherwise be lost.

## 15. Forms and Settings

Forms should resemble calm Pocket ID-style settings surfaces while retaining Hashi's operational density.

Requirements:

1. Labels appear above controls unless an established compact row pattern is clearer.
2. Help text explains consequences or formats, not obvious control behavior.
3. Related fields use a shared grid and align vertically.
4. Advanced fields are collapsed by default when they are not required for the normal path.
5. Validation appears next to the affected field and in a form-level summary when submission fails.
6. Destructive settings are visually separated from ordinary settings.
7. Save actions are sticky only for long forms where the action would otherwise leave the viewport.
8. Toggles are used only for immediate binary settings.
9. A checkbox is used for multi-selection or explicit acknowledgement.
10. Select menus, comboboxes, segmented controls, sliders, and steppers must match the data type rather than being replaced by generic text inputs.

## 16. Buttons, Icons, and Menus

### 16.1 Buttons

Button hierarchy:

1. Primary: the main safe action in the current context.
2. Secondary: common supporting actions.
3. Ghost: low-emphasis toolbar and row actions.
4. Destructive: actions that delete, revoke, block, or irreversibly replace state.

Only one primary button should normally appear in a bounded action area.

### 16.2 Icons

Lucide icons are required when a suitable icon exists.

Icon-only buttons must:

* Use a stable square hit area.
* Have an accessible name.
* Show a tooltip when the icon's meaning is not universally obvious.
* Preserve icon position during pending state.

### 16.3 Menus

Overflow menus contain secondary or uncommon actions. The interface must not hide the routine primary action inside an overflow menu.

Destructive menu items require clear styling and the confirmation behavior defined by the relevant workflow.

## 17. Motion and Interaction Feedback

Hashi uses restrained motion. The previous instant expansion behavior should be replaced with short transitions that explain spatial change without slowing work.

### 17.1 Durations

| Interaction | Duration |
| --- | ---: |
| Hover and pressed feedback | 80 to 120ms |
| Menu and popover | 100 to 140ms |
| Row or advanced-section expansion | 140 to 180ms |
| Drawer and dialog | 160 to 220ms |
| Page-level content transition | 120 to 180ms |

Animations longer than 250ms require a specific usability reason.

### 17.2 Properties

Prefer:

* Opacity.
* Transform.
* Grid-row or measured height for disclosure.
* Border and background color.

Avoid animating large shadows, filters, full-page blur, or layout properties that cause repeated reflow.

### 17.3 Easing

Use one standard ease-out curve for entrances and one ease-in curve for exits. Components must not invent their own motion personality.

### 17.4 Reduced Motion

When \`prefers-reduced-motion: reduce\` is active:

* Spatial transitions become immediate or opacity-only.
* Auto-animated charts are disabled.
* Decorative motion is removed.
* Loading indicators remain perceivable without continuous large movement.

## 18. Loading, Pending, and Error States

1. Local feedback should appear within 100ms of input.
2. A spinner should not replace button text for operations expected to finish almost instantly; use a subtle pending state.
3. Skeletons are appropriate when a stable content layout is known and loading is expected to exceed roughly 200ms.
4. Existing content should remain visible during background refresh when it is not misleading.
5. Stale data must be marked when freshness matters.
6. Errors must explain what failed and whether retrying is safe.
7. Toasts are for transient confirmation; durable failures and required actions remain visible in context.
8. Optimistic updates are allowed only when rollback is clear and the action is not security-sensitive or destructive.

## 19. Charts and Data Visualization

### 19.1 Library Choice

uPlot remains the default charting library for dense or continuously updated time-series data.

Small sparklines, status strips, and simple progress visuals may use lightweight SVG or CSS when this avoids unnecessary chart instances.

No additional general-purpose chart library should be added without a demonstrated requirement that uPlot and simple SVG cannot satisfy.

### 19.2 Chart Rules

Charts must:

1. Use semantic theme tokens rather than hard-coded colors.
2. Use flat fills and clear strokes rather than gradients or 3D effects.
3. Default to no more than four simultaneously emphasized series.
4. Keep axes and grid lines visually subordinate to data.
5. Provide a readable legend when more than one series is present.
6. Provide hover or keyboard-readable values where the chart supports inspection.
7. Show units in labels or tooltips.
8. Distinguish missing data from zero.
9. Use status color only for status meaning.
10. Preserve a stable chart height during loading and updates.

Graphite uses blue as its principal non-semantic series. Verdant and Violet Night use yellow as the principal series. Warm Linen uses clay as the principal series. Secondary series use coordinated but clearly distinguishable colors.

## 20. Responsive Behavior

Hashi is desktop-first because it is an infrastructure administration tool, but all supported workflows must remain usable on mobile.

Requirements:

1. Desktop layouts prioritize comparison and simultaneous context.
2. Tablet layouts reduce column count before reducing type size.
3. Mobile layouts stack toolbars and form columns while keeping primary actions visible.
4. Tables may scroll or transform into structured rows depending on the comparison task.
5. Drawers and dialogs must fit within the viewport and allow internal scrolling.
6. Fixed controls must respect safe areas.
7. Text and controls must not overlap at any supported viewport.
8. Touch targets are at least 40px where space permits and never below 32px for compact expert controls.

## 21. Accessibility

The design system must meet WCAG 2.2 AA for supported workflows.

Requirements:

1. Text and essential icons meet contrast requirements in every default theme.
2. Focus indicators are visible on all interactive elements.
3. All interactions are keyboard accessible.
4. Focus is managed correctly for dialogs, drawers, menus, and inline editors.
5. Color is not the sole status indicator.
6. Form errors are programmatically associated with fields.
7. Live updates use appropriate announcements without becoming noisy.
8. Charts provide textual summaries or accessible data equivalents.
9. Tooltips are supplementary and never the only source of required information.
10. Themes and custom colors are validated against required contrast pairs.
11. Reduced motion and reduced transparency preferences are honored.

## 22. Performance

The redesign must not trade responsiveness for appearance.

Requirements:

1. Do not add a general JavaScript animation framework.
2. Use CSS transitions and Svelte's built-in transition primitives.
3. Avoid permanent backdrop blur across large scrolling surfaces.
4. Lazy-mount charts and expensive detail tools when they are not visible.
5. Destroy chart instances and observers when components unmount.
6. Avoid rerendering complete tables for a single-row state change.
7. Use virtualization only for data sets large enough to justify its complexity.
8. Do not fetch imagery or decorative assets for routine admin screens.
9. Theme changes should update CSS variables without rebuilding component trees.
10. The UI must remain functional when animation or transparency is unavailable.

## 23. Frontend Implementation

### 23.1 Existing Stack

The redesign uses the existing frontend stack:

* SvelteKit 5 and TypeScript.
* Tailwind CSS v4.
* shadcn-svelte component conventions.
* Bits UI primitives.
* \`tailwind-variants\`, \`clsx\`, and \`tailwind-merge\` for controlled variants.
* Lucide icons.
* uPlot.
* CodeMirror 6.
* Vitest, Testing Library, and Playwright.

No second component system is allowed.

### 23.2 Component Architecture

Components should be divided into:

1. Primitive UI components: button, input, menu, dialog, drawer, table, badge, tooltip, and similar controls.
2. Layout components: application shell, page header, toolbar, split view, panel, and responsive grid.
3. Domain components: resource table, sync plan, monitor chart, security event row, and similar Hashi-specific tools.

Domain components must consume primitives rather than duplicating their interaction and styling logic.

### 23.3 Theme Example

The final token list may grow, but the implementation should follow this shape:

~~~css
:root,
[data-theme='graphite'] {
  color-scheme: dark;
  --background: #08090b;
  --shell: #0d0f12;
  --surface-1: #13161a;
  --surface-2: #191d23;
  --border: #2b323b;
  --foreground: #f2f5f7;
  --muted-foreground: #9da6b2;
  --accent: #2563eb;
  --accent-foreground: #ffffff;
  --accent-hover: #3b82f6;
  --focus: #60a5fa;
  --radius-control: 10px;
  --radius-panel: 18px;
}

[data-theme='verdant'] {
  color-scheme: dark;
  --background: #030a07;
  --shell: #06120d;
  --surface-1: #0a1a12;
  --surface-2: #10251a;
  --border: #27513b;
  --foreground: #edf7f0;
  --muted-foreground: #94aa9b;
  --accent: #e8ce55;
  --accent-foreground: #08110c;
  --accent-hover: #f5de74;
  --focus: #ffe99a;
}
~~~

### 23.4 Motion Example

Disclosure components should use shared motion tokens:

~~~css
:root {
  --motion-fast: 100ms;
  --motion-normal: 160ms;
  --motion-slow: 220ms;
  --ease-out: cubic-bezier(0.2, 0.8, 0.2, 1);
  --ease-in: cubic-bezier(0.4, 0, 1, 1);
}

.disclosure-content {
  transition:
    grid-template-rows var(--motion-normal) var(--ease-out),
    opacity var(--motion-fast) linear;
}

@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    scroll-behavior: auto !important;
    transition-duration: 1ms !important;
    animation-duration: 1ms !important;
    animation-iteration-count: 1 !important;
  }
}
~~~

### 23.5 Chart Tokens

uPlot configuration must resolve colors at render time from CSS variables. Values such as the current hard-coded \`#FAD000\` and \`#A599E9\` must not remain embedded in chart components.

## 24. Testing Requirements

### 24.1 Component Tests

Tests must cover:

* Component variants.
* Keyboard operation.
* Disclosure state.
* Disabled and pending behavior.
* Validation and error states.
* Theme-independent semantic classes or tokens.

### 24.2 Visual Tests

Playwright screenshots must cover representative views in:

* All four default themes.
* Desktop width.
* Tablet width.
* Mobile width.
* Reduced-motion mode.

The test set must include at least one dense table, one settings form, one overlay, and one chart.

### 24.3 Accessibility Tests

Automated checks must be supplemented by keyboard and focus-order verification for representative workflows.

### 24.4 Theme Tests

Tests must verify:

1. Theme persistence.
2. No flash of an incorrect theme during startup.
3. Safe fallback for invalid custom theme data.
4. Contrast validation for custom themes.
5. Chart and status colors update when the theme changes.

## 25. Implementation Sequence

The design foundation should be implemented in this order:

1. Define the semantic token schema.
2. Implement the four default themes.
3. Normalize primitive control geometry and interaction states.
4. Implement motion, reduced-motion, transparency, and reduced-transparency behavior.
5. Normalize application shell and page-level layout primitives.
6. Refactor chart colors and shared chart configuration.
7. Build the custom-theme editor and persistence.
8. Establish representative visual and accessibility tests.
9. Produce the separate interface-architecture addendum.
10. Migrate domain pages according to the approved interface architecture.

The initial token and primitive work must not reorganize domain pages before the interface-architecture decisions are approved.

## 26. Acceptance Criteria

The design foundation is complete when:

1. The proposal-only routes and components are absent from the production repository.
2. The UI uses a consistent rounded geometry system.
3. Graphite, Warm Linen, Verdant, and Violet Night are complete selectable themes.
4. Graphite uses a blue primary accent.
5. Verdant and Violet Night use yellow primary accents while preserving distinct warning semantics.
6. Violet Night is substantially darker and less saturated than the original Shades of Purple theme.
7. Warm Linen uses warm brown, clay, cream, and crisp light surfaces without becoming low contrast.
8. Components consume semantic tokens and contain no theme-specific branching.
9. Charts consume theme tokens rather than hard-coded palette values.
10. Common disclosures, menus, dialogs, drawers, and state changes use the shared restrained motion system.
11. Reduced-motion and reduced-transparency preferences are honored.
12. Tables, forms, panels, buttons, navigation, and overlays follow one component language.
13. All default themes satisfy the required accessibility contrast and focus behavior.
14. The redesign adds no second component library and no general animation library.
15. Representative visual tests cover all themes and supported viewport classes.
16. The existing domain functionality and backend contracts remain unchanged by the design-foundation phase.
17. Page composition and information architecture remain deferred until the dedicated follow-up addendum is approved.
