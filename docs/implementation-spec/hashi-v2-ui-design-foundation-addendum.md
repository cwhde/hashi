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

The reference images are not required implementation assets. This section records the visible characteristics that must survive even when an implementer cannot see those images.

These references are inputs, not templates. Hashi must retain its own information model and must not inherit unnecessary complexity, branding, or exact page structures from any reference product.

### 5.1 Pocket ID Settings

The Pocket ID references show a nearly black full-page canvas with a broad left settings navigation and large rounded content panels.

Visible characteristics:

1. The sidebar occupies roughly 16 to 19 percent of the viewport width on a wide desktop.
2. Sidebar destinations are plain text rows. The selected row uses a quiet charcoal fill with fully rounded ends and no bright outline.
3. The main account form sits inside one large dark-gray panel with a thin border and a corner radius visually around 24 to 30px.
4. The panel title is small relative to the panel. It uses an icon, a 20 to 24px heading, and substantial space for actual controls rather than decorative copy.
5. Profile details are arranged in a two-column form. Labels sit directly above long, low inputs.
6. Inputs use slightly lighter charcoal fill, thin gray borders, pill-like ends, and approximately 40 to 44px height.
7. The save action is a small high-contrast light button aligned to the lower-right corner. It is not a full-width call to action.
8. Secondary sections such as passkeys use their own large rounded panels. Items inside them are unframed rows separated by spacing rather than nested cards.
9. Row actions are small icon buttons placed at the far right.
10. The application grid uses four columns on a very wide viewport. Each app card has one icon block, app name, hostname, a subdued last-used timestamp, an overflow action, and a compact launch button.
11. Empty black canvas is allowed around the content. The interface does not attempt to fill every pixel with panels.

Hashi adopts Pocket ID's rounded panel language, calm form construction, and selective use of whitespace. Hashi does not adopt its unusually large empty margins on data-heavy pages or its serif branding typography.

### 5.2 Cloudflare Account and Domain Views

The Cloudflare references show a dark operational console with a narrow persistent sidebar, a compact top context bar, dense content, and very little decorative styling.

Visible characteristics:

1. The sidebar is approximately 220 to 235px wide on a 1920px viewport and runs from the top to the bottom of the screen.
2. Navigation contains grouped labels, shallow indentation, 32 to 38px rows, small icons, and chevrons for expandable groups.
3. The selected destination uses a low-contrast dark-gray fill. Selection does not depend on a thick colored border or large icon.
4. The account-home canvas leaves a deliberate blank margin before a centered working area. The content itself is arranged as a compact three-column dashboard.
5. Dashboard panels use a one-pixel gray border, almost no shadow, a dark surface only slightly lighter than the canvas, and small headings in their top bars.
6. The panels contain actual data, lists, and actions. They do not contain large decorative illustrations or oversized numbers.
7. The DNS records page uses almost the entire available content width. A search field and compact toolbar sit immediately above a dense table.
8. Table rows are approximately 36 to 42px high. Headers are quiet, columns align strictly, and the edit action stays at the far right.
9. Editing a DNS record expands an editor directly beneath the selected row across the full table width. The rest of the table remains visible and does not navigate away.
10. The expanded editor uses one horizontal field row, a secondary attributes disclosure, and a compact save/cancel/delete action row.
11. Overview and analytics pages place compact statistics above or beside charts. Charts use thin lines, muted grids, and no decorative fill.
12. The domain overview demonstrates a useful three-region layout: navigation on the left, principal charts in the center, and compact actions or configuration summaries on the right.

Hashi adopts Cloudflare's density, content alignment, table behavior, compact toolbars, and inline-editing model. Hashi rounds the resulting surfaces consistently instead of reproducing Cloudflare's mostly sharp containers.

### 5.3 Pangolin

The Pangolin references show a dark network administration interface with a persistent left navigation, a centered working column, and table-first resource management.

Visible characteristics:

1. The navigation is about 230px wide at 1920px and uses grouped sections with compact icons.
2. The content column begins with a small product mark, a 24 to 28px page title, one line of muted description, and then the page tool.
3. A dismissible information banner is framed with a quiet accent-tinted border. It contains a short title, concise explanation, and one action aligned right.
4. The resource list is one bounded table panel. Search appears on the left; refresh and add actions appear on the right.
5. Table controls are 32 to 36px high. The primary add action is the only strongly colored control.
6. Rows contain status, uptime, access, authentication, enabled state, and actions without separate cards for each value.
7. Edit is visible as a compact row action; uncommon actions sit in an overflow menu.
8. Resource settings use a compact summary strip above tabs. Tabs then expose one large settings tool at a time.
9. Advanced settings remain inside the same content flow and do not open a visually unrelated screen.

Hashi adopts Pangolin's resource-list composition, low color usage, and action hierarchy. Hashi should be slightly more rounded and should avoid orange as a global brand accent because accent color is theme-dependent.

### 5.4 NetBird

The NetBird reference shows the densest acceptable end of the target range.

Visible characteristics:

1. A narrow sidebar and compact 40 to 48px top bar reserve almost all remaining space for a resource table.
2. The title, explanatory sentence, search, page-size control, and refresh action are grouped in a shallow header area.
3. The add action is isolated at the far-right edge of the same horizontal area.
4. Network rows are short and full-width, with one identity cell and several action/status columns.
5. Related actions are embedded in their columns as compact buttons rather than placed in separate panels.
6. There is substantial empty canvas below the table when only a few rows exist; the table is not padded with fake content.

Hashi may use this density for large resource sets, but must retain the larger rounded controls and stronger form readability taken from Pocket ID.

### 5.5 Gatus

The Gatus reference shows a dark navy status dashboard with one search/filter toolbar and a grouped grid of endpoint cards.

Visible characteristics:

1. A group heading spans the width and includes a compact failure-count badge.
2. Endpoint cards are arranged in three columns at desktop width.
3. Each card contains endpoint identity and address on the left, a status badge on the right, and one thin history strip across the lower half.
4. The history strip consists of many narrow vertical samples. Green samples mean healthy and red samples mean unhealthy.
5. Latency appears as one small number near the strip rather than as a separate chart panel.
6. Time labels sit at the two ends of the strip.
7. Card borders are visible but quiet. Cards do not use shadows, gradients, or ornamental icons.

Hashi adopts the status history strip and grouped endpoint overview. It must use cards for status endpoints only when card comparison is more useful than a table.

### 5.6 Nezha

The Nezha references show two useful patterns: a compact server overview and a detail view dominated by charts.

Visible characteristics:

1. The overview begins with four small summary panels for total servers, online servers, offline servers, and network totals.
2. Server cards appear below in a two-column grid. Each card combines state, identity, system metadata, compact progress bars, and upload/download totals.
3. The detail page begins with one broad server summary panel containing status, uptime, architecture, memory, disk, region, CPU, load, traffic totals, boot time, and last-active time.
4. A centered segmented control switches between detail and network modes.
5. Detail charts use a three-column grid with equal card sizes, sparse grid lines, thin colored traces or fills, a title, and a current value.
6. The network view places service latency summaries in a horizontal strip above one wide line chart.
7. The charts use dark surfaces and restrained series colors; data remains more prominent than axes or borders.

Hashi adopts Nezha's compact metric grouping, equal chart geometry, and current-value placement. It does not adopt the decorative blue page gradient shown in the reference.

### 5.7 Claude Warm Interface

The Claude references define the intended character of Warm Linen.

Visible characteristics:

1. The canvas is warm off-white rather than pure white.
2. The sidebar is a slightly darker cream, separated from the content by a fine warm-gray line.
3. Menus use warm cream surfaces, thin taupe borders, very soft shadows, and approximately 8 to 12px corner radii.
4. Ordinary controls and cards remain almost neutral. Saturated terracotta/clay appears primarily on the main action and small brand marks.
5. Dividers and borders are brown-gray, not cool blue-gray.
6. The interface uses several close warm surface tones to create hierarchy without reducing text contrast.
7. Large editorial serif text appears in Claude's product greeting, but the operational controls remain compact and readable.

Hashi adopts the warm surface relationships, restrained clay accent, menu treatment, and low-saturation hierarchy. Hashi keeps its system sans-serif typography and does not use giant editorial greetings.

### 5.8 Visual Studio Dark Violet

The Visual Studio reference defines the depth target for Violet Night.

Visible characteristics:

1. The overall canvas is a nearly black navy-violet.
2. Toolbars, side panels, editor canvas, tabs, and bottom panels are separated through small changes in violet/navy lightness rather than bright borders.
3. Yellow is used for active markers, warnings, selected traces, and small high-importance details.
4. Purple and magenta exist in code syntax, but the application chrome itself remains dark and controlled.
5. Large surfaces are not bright purple. Violet is perceived as an undertone.

Hashi adopts the near-black violet layering and yellow accent relationship. It does not reproduce syntax-highlighting rainbow colors across application controls.

### 5.9 Verdant Target Mock

The supplied Verdant mock is the closest visual approximation of the target shell and dashboard density.

Visible characteristics:

1. The canvas, sidebar, header, and panels are all extremely dark green-black.
2. Surfaces are distinguished by one-pixel desaturated green borders and subtle changes in green-black fill.
3. The expanded sidebar occupies roughly 22 percent of the 833px-wide screenshot because the screenshot is narrow; at normal desktop width it should use the fixed dimensions defined below.
4. The active navigation row uses a translucent yellow-green fill and a small yellow icon.
5. The primary Add action is yellow with near-black text. Secondary icon actions are dark with green borders.
6. The content area uses a compact three-column dashboard. Panel corner radii are clearly rounded but not pill-shaped.
7. Security, performance, and activity summaries occupy the first row. Lists and an action panel occupy the second row. A full-width next-steps strip occupies the bottom.
8. Yellow appears in the primary action, active state, warning icon, and directional arrows. It does not flood panel backgrounds.
9. Small green values and sparklines communicate positive status while yellow remains the interaction accent.
10. The screenshot uses a faint green atmospheric glow near the shell edges. Hashi may reproduce this only as a static, extremely low-contrast canvas treatment; it must not become a bright gradient or reduce text clarity.

### 5.10 Rejected Visual Direction

The earlier proposals were rejected because they looked like a low-budget science-fiction interface rather than a clean administration tool.

The redesign must therefore avoid:

1. HUD-like frames, corner brackets, scan lines, glowing outlines, and fake terminal decoration.
2. Large luminous gradients, neon purple/blue washes, and bright glass panels.
3. Oversized status rings, radar visuals, or decorative charts without operational value.
4. Floating islands of controls with no alignment to the page grid.
5. A separate card for every number.
6. Excessively rounded pills used as general containers.
7. Inconsistent mixtures of sharp and rounded geometry.
8. Large headings or empty hero-like areas that push operational content below the fold.
9. Animation that exists to make the interface look futuristic.
10. Theme colors applied to every border, label, icon, and surface.

### 5.11 Target Synthesis

The final target is not one of the references in isolation.

It is:

* Pocket ID's rounded surfaces and form clarity.
* Cloudflare's navigation discipline, dashboard composition, table density, and inline editing.
* Pangolin's resource-list hierarchy and restrained primary actions.
* NetBird's efficient use of width for large infrastructure sets.
* Gatus's status history strips.
* Nezha's chart grids and compact metric summaries.
* Claude's warm neutral palette construction.
* Visual Studio's near-black violet depth.
* The supplied Verdant mock's green-black and yellow theme execution.

An implementation is off target if it is attractive in isolation but cannot display a large resource table, a long settings form, a status grid, and a multi-series chart with the same visual language.

### 5.12 Quantitative Desktop Target

At viewport widths of 1440px and above:

| Element | Target |
| --- | --- |
| Expanded sidebar | 224 to 240px |
| Collapsed rail | 60 to 68px |
| Top context bar | 48 to 56px high |
| Main horizontal padding | 24px at 1280px, 32px at 1440px, up to 40px at 1920px |
| Main vertical padding | 20 to 32px |
| Compact content gap | 12px |
| Standard content gap | 16px |
| Major section gap | 24 to 32px |
| Page title | 24 to 28px, 1.2 line height |
| Page description | 13 to 14px, maximum readable width around 720px |
| Toolbar/control height | 32 to 36px |
| Standard table row | 44 to 48px |
| Dense table row | 40px minimum |
| Panel padding | 16px compact, 20 to 24px standard |
| Dashboard grid gap | 12 to 16px |
| Standard chart height | 220 to 280px |
| Sparkline height | 28 to 48px |

The main content should normally begin within 24 to 32px of the top context bar. A page must not insert a large decorative header between navigation and operational content.

Text-heavy settings forms should use a readable maximum width of approximately 1120 to 1280px. Tables, status grids, and wide charts may use the full available content width.

At 1920px, a routine page should normally show the page title, its primary toolbar, table or first dashboard row, and part of the following content without scrolling.

### 5.13 Visual Density Budget

On a typical desktop operational page:

1. Canvas and neutral surfaces should occupy at least 85 percent of visible area.
2. Theme accent should occupy no more than approximately 5 percent of visible area.
3. Semantic success, warning, danger, and information colors should appear only where they carry state.
4. No more than one large panel and two smaller panels should compete for first visual attention.
5. A routine toolbar should contain no more than one filled primary action.
6. Explanatory copy should usually fit in one or two lines. Longer explanations belong in help, disclosure, or documentation.
7. A page should use at most three simultaneous surface elevations.

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

### 7.4 Grid

Desktop content uses a 12-column grid with 12 to 16px gutters.

Common compositions:

* Three equal operational panels: 4 + 4 + 4 columns.
* Primary tool with supporting context: 8 + 4 columns.
* Wide primary tool with narrow inspector: 9 + 3 columns.
* Two equal forms or charts: 6 + 6 columns.
* Full-width table, editor, or timeline: 12 columns.

Grid spans describe layout, not card count. A 4-column region may contain an unframed list, chart, table, or settings group.

At widths below 1200px, three-column layouts collapse to two columns. At widths below 800px, ordinary multi-column layouts collapse to one column. Forms may retain two columns down to approximately 900px when each field still has at least 320px of usable width.

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

### 9.7 Exact Color Application

The theme accent is used for:

* The single primary action in a bounded area.
* Active navigation indicators.
* Selected tabs and segmented controls.
* Keyboard focus.
* Links.
* The principal non-semantic chart series.
* Small directional or disclosure emphasis where needed.

The theme accent is not used for:

* Every icon.
* Every panel border.
* Ordinary body text.
* All table values.
* Panel backgrounds.
* Success, warning, or danger messages.

Panel borders use the border token at full opacity or a tested lower-opacity derivative. They do not use the accent token except for selected, focused, or actively edited containers.

Active navigation should normally use an accent-tinted background between 8 and 14 percent opacity plus foreground text. It must not use a fully saturated accent rectangle.

Hover surfaces should change by one surface level or use 4 to 8 percent foreground tint. Hover must not introduce a new hue.

Graphite should appear approximately black and charcoal with small blue signals. Warm Linen should appear cream and warm gray with clay signals. Verdant should appear green-black with yellow signals. Violet Night should appear near-black with a violet undertone and yellow signals.

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

### 12.1 Application Shell Anatomy

The desktop shell is constructed as follows:

1. A fixed left navigation column.
2. A compact top context bar spanning only the content region.
3. One independently scrolling main content region.
4. Optional drawers layered above the content; no permanent third rail unless a workflow explicitly requires an inspector.

The expanded navigation contains, from top to bottom:

1. Product or deployment identity in a 48 to 56px header.
2. Optional global search or command trigger.
3. Grouped primary destinations.
4. Flexible empty space.
5. Low-frequency destinations such as settings, documentation, or version information.

Navigation rows use:

* 16px icons.
* 13 to 14px labels.
* 36px height.
* 8 to 10px row radius.
* 8 to 12px horizontal gap between icon and label.
* 12 to 16px horizontal inset from the sidebar edge.

Group labels use 11 to 12px muted text and sit 16 to 20px above the first row in a group. Deep navigation trees are prohibited; one expandable child level is the routine maximum.

The top context bar may contain breadcrumbs, the current environment or connection context, global pending state, theme control, and account menu. It must not duplicate the page title.

### 12.2 Page Header Anatomy

A routine page header contains:

1. Optional breadcrumb or short eyebrow.
2. Page title.
3. One sentence of description when necessary.
4. Contextual status or count when useful.
5. One primary action and a small number of secondary actions aligned right.

The title/action row should fit within 56 to 72px on desktop. Long explanations, onboarding banners, and filters appear below it.

Filters and search belong in a toolbar attached visually to the data tool they control, not scattered through the global header.

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

### 13.1 Standard Panel Recipe

A standard panel has:

* 18px radius.
* One-pixel border.
* Surface 1 background.
* No shadow.
* 20px padding at desktop.
* 16px padding at compact widths.

Its optional header contains a 16px icon, 14 to 16px title, optional one-line description, and actions aligned to the right. The header uses 12 to 16px bottom spacing. It does not receive a separate filled background unless it is also a toolbar.

Panel body rows use separators only when rows need comparison. Otherwise use 12 to 16px vertical spacing.

### 13.2 Repeated Item Card Recipe

A repeated card such as an app, endpoint, agent, or server has:

* Fixed internal alignment across all siblings.
* Identity in the upper-left.
* Status or overflow action in the upper-right.
* Secondary metadata below the identity.
* Primary action in a consistent lower-right location when cards are actionable.
* Status history, progress, or compact metrics in the lower region.

Cards in one grid must have equal minimum height. Missing metadata reserves or intentionally removes space consistently; it must not cause every card to have a different action position.

### 13.3 Information Banner Recipe

Information banners are 12 to 16px radius, one-pixel border, and 12 to 16px padding. They contain an icon, short title, no more than two lines of text, optional action, and optional dismiss control.

Warning and danger banners use semantic colors sparingly: a tinted border, icon, and title are preferred over a fully saturated background.

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

### 14.1 Table Anatomy

A full data tool is constructed in this order:

1. Optional compact summary or warning.
2. Toolbar.
3. Column header.
4. Data rows.
5. Optional expanded inline editor.
6. Footer with item count, page size, and pagination.

The toolbar uses a 36px search control approximately 280 to 360px wide on desktop. Filters follow the search. Refresh, import, export, display options, and the primary add action align to the right.

Table geometry:

* Header height: 36 to 40px.
* Standard row height: 44 to 48px.
* Dense row height: 40px.
* Horizontal cell padding: 12 to 16px.
* Header font: 12 to 13px, medium weight.
* Body font: 13 to 14px.
* Row separator: one pixel.
* Checkbox column: 36 to 40px.
* Overflow/action column: 40 to 80px depending on whether a visible primary row action exists.

Identity cells may include a 28 to 32px icon or avatar, but the icon must not increase the row beyond its selected density.

### 14.2 Inline Editing

Short records such as DNS entries, simple resource targets, and list items should edit inline when practical.

The selected row receives an accent marker or accent-tinted border. Its editor expands immediately below it and spans the full table width.

The editor:

* Preserves the column context where practical.
* Uses a 140 to 180ms height/opacity transition.
* Places save and cancel together.
* Separates delete from save/cancel.
* Keeps surrounding rows visible.
* Closes only after explicit cancel, successful save, or a clearly confirmed navigation.

Forms too complex for one or two horizontal rows use a right-side drawer rather than an oversized inline editor.

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

### 15.1 Settings Page Recipe

A settings page should visually resemble the Pocket ID account and OIDC client references:

1. A compact page header or back link.
2. One large rounded settings panel per major concern.
3. A panel title and optional short description.
4. A two-column field grid on desktop.
5. Full-width fields only for long values such as URLs, certificates, or scripts.
6. Toggles placed beside their labels and explanation, not in a detached control column.
7. Advanced options collapsed at the bottom of the same panel.
8. Save aligned to the bottom-right or placed in a sticky footer for long forms.

Standard input geometry:

* Height: 40px.
* Radius: 10 to 14px.
* Horizontal padding: 12px.
* Label-to-input gap: 6px.
* Help-text gap: 6px.
* Field-to-field vertical gap: 16 to 20px.
* Column gap: 16 to 24px.

Textareas start at 96 to 128px height. Code and YAML fields use CodeMirror and must not be styled as ordinary textareas.

### 15.2 Detail Disclosure

Summary information that the user may need to copy or inspect, but rarely edit, may appear in a top summary panel with a centered or right-aligned "Show more details" disclosure.

Expanded details must remain inside the panel, use definition-list alignment, and avoid opening a modal for read-only metadata.

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

### 17.5 Required Interaction Motion

The following interactions must have visible but brief motion:

* Sidebar group expansion: height plus opacity, 160ms.
* Advanced form disclosure: height plus opacity, 160ms.
* Inline table editor: height plus opacity, 160ms.
* Popover/menu: 4px vertical translate plus opacity, 120ms.
* Dialog: opacity plus scale from 0.98 to 1, 180ms.
* Right drawer: translate from 12 to 20px outside the viewport plus opacity, 200ms.
* Tab content: opacity only, 100ms; the content region must not slide horizontally.
* Toggle: thumb translation, 120ms.
* Button press: at most 1px downward translation or a small surface change, 80ms.

Opening motion uses the standard ease-out curve. Closing motion may be 20 to 30 percent shorter. No interaction waits for an exit animation before beginning the requested action.

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

### 19.3 Chart Construction

A standard chart panel contains:

1. Title in the upper-left.
2. Current value or summary in the upper-right.
3. Optional compact time-range segmented control.
4. Plot area.
5. Legend below the plot or in unused header space.

Plot geometry:

* Standard height: 220 to 280px.
* Compact metric chart: 140 to 180px.
* Primary series stroke: 1.5 to 2px.
* Secondary series stroke: 1 to 1.5px.
* Grid line: one pixel at low contrast.
* Area fill: zero to 12 percent opacity.
* Point markers: hidden by default and shown on hover.
* Axis label: 11 to 12px.

For a Nezha-style chart grid, all chart panels in a row use the same height and align titles, current values, axes, and plot baselines.

For a Gatus-style history strip:

* Samples are 3 to 5px wide with 1 to 2px gaps.
* Samples use 2 to 3px radius at most.
* Healthy, degraded, unhealthy, and missing samples use semantic tokens.
* The strip has accessible text summarizing uptime and latest status.

For wide multi-service latency charts, the summary strip above the chart uses equal-width service cells with service name, current latency, and compact loss or trend values.

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

### 23.6 Required Changes From the Current Frontend

The current frontend is a functional starting point, but the redesign must explicitly correct these implementation details:

1. Replace the binary \`:root\` light and \`.dark\` theme model with \`data-theme\` and the four named themes.
2. Remove the default remote background image from the admin application body. Routine admin screens use the theme canvas. Optional user-selected backgrounds remain a separate personalization feature and must not reduce readability.
3. Replace the single global \`--radius: 0.5rem\` approach with the radius scale defined in this addendum.
4. Remove component-level \`text-white\` styling. Titles and labels use semantic foreground tokens so Warm Linen and custom themes render correctly.
5. Replace hard-coded Hashi palette utility usage such as \`bg-hashi-bg\`, \`bg-hashi-bg-dark\`, and \`text-hashi-contrast\` in shared components with semantic shell, surface, foreground, muted, and accent tokens.
6. Replace generic \`bg-card/40\` panel treatment with explicit surface levels. Transparency must be intentional and theme-tested.
7. Replace hard-coded chart strokes, fills, axes, and grids with runtime-resolved chart tokens.
8. Expand button variants so primary, secondary, ghost, outline, and destructive states have theme-independent hover, pressed, pending, disabled, and focus behavior.
9. Normalize shared panel components before restyling individual pages. Domain pages must not locally recreate panel borders, radius, padding, or title treatment.
10. Normalize navigation width, row height, icon size, active state, and group spacing before changing its content structure.
11. Preserve the existing offline-development behavior, API contracts, and domain functionality while replacing presentation.
12. Keep the current Svelte component boundaries where they already match domain ownership; visual redesign is not permission for unrelated backend or domain refactoring.

### 23.7 Required Shared Primitives

Before broad page migration, the frontend should expose shared implementations for:

* Application shell.
* Navigation group and navigation item.
* Page header.
* Toolbar.
* Standard panel.
* Information banner.
* Metric summary.
* Status pill.
* Data table and table toolbar.
* Inline row editor container.
* Empty, error, and loading states.
* Settings section.
* Form field and help text.
* Advanced disclosure.
* Segmented control.
* Dialog.
* Right-side drawer.
* Popover and overflow menu.
* Chart panel.
* Status history strip.

These primitives define structure as well as styling. A page must not manually reproduce their padding and border classes.

### 23.8 Representative Visual Validation Scenes

The redesign must be judged against complete representative scenes rather than isolated buttons.

#### Scene A: Operational Overview

At 1440 x 900 in Verdant:

1. Expanded 232px sidebar.
2. 52px top context bar.
3. Compact page header with title, optional time range, two icon actions, and one yellow primary action.
4. First dashboard row containing three equal summary panels.
5. Second row containing one list panel, one centered action or state panel, and one compact security/status list.
6. Third row containing one full-width strip of recommended or pending actions.
7. All first-row content visible without scrolling.
8. Dark green-black canvas, one-pixel green borders, white text, muted sage metadata, yellow interaction accent, and semantic green status values.

This scene should resemble the supplied Verdant mock in density and hierarchy, but use production data and Hashi's eventual approved overview content.

#### Scene B: Resource Management

At 1440 x 900 in Graphite:

1. Compact page title and one-line description.
2. Optional information banner.
3. One full-width table tool.
4. Search on the left; filters after search; refresh and blue Add action on the right.
5. At least eight visible 44 to 48px rows.
6. Identity, connection, health, access, enabled state, and actions aligned in columns.
7. One selected row expanded into an inline editor.
8. Remaining rows stay visible.

This scene combines Pangolin's page hierarchy, Cloudflare's table behavior, and Pocket ID's rounded geometry.

#### Scene C: Settings Form

At 1440 x 900 in Warm Linen:

1. Warm cream canvas and slightly darker cream shell.
2. Compact sidebar selection with no saturated background.
3. One large 18 to 24px-radius settings panel.
4. Panel header with icon, title, and one-line explanation.
5. Two-column fields with crisp near-white inputs.
6. A read-only summary region followed by editable fields.
7. Toggle rows with explanations.
8. Collapsed advanced options.
9. Clay primary save button in the lower-right.
10. Taupe borders, dark brown text, and no cool gray or purple tint.

This scene should visually sit between the supplied Claude menus and Pocket ID settings forms.

#### Scene D: Monitoring or Security Analytics

At 1440 x 900 in Violet Night:

1. Near-black violet canvas.
2. One broad summary panel at the top.
3. Compact time-range segmented control.
4. Three equal chart panels in one row or one wide chart with aligned summary cells above it.
5. Yellow principal series, muted violet secondary series, and semantic status colors where appropriate.
6. Thin grid lines, no bright area gradients, and no chart animation that delays reading.
7. Current values aligned consistently in chart headers.

This scene combines Nezha's chart geometry with the Visual Studio reference's dark-violet depth.

#### Scene E: Narrow Viewport

At 390 x 844:

1. Navigation becomes a drawer.
2. Page header actions collapse into one primary action plus overflow when necessary.
3. Two-column forms become one column.
4. Overview panels become one column.
5. Tables either retain purposeful horizontal scrolling or become structured rows; columns must not simply disappear without user control.
6. No clipped labels, overlapping controls, or viewport-wide fixed elements.

All five scenes must be implemented with the same primitives and token system. They are not separate proposals or theme-specific applications.

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
18. An implementer can reproduce the intended visual language without access to the original reference screenshots.
19. The operational overview, resource table, settings form, analytics, and narrow-viewport validation scenes satisfy the concrete compositions defined in section 23.8.
20. The finished interface does not contain any of the rejected science-fiction, neon, HUD, gradient-heavy, or indiscriminate card-grid traits listed in section 5.10.
