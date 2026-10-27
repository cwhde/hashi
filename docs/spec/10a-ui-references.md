# 10a — UI reference detail (detail for 10-ui.md)

> Written descriptions of the reference screenshots you gave the earlier agent (the images were never committed).
> They describe the **intent**; size and percentage estimates in them are indicative, not rules.

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
3. The expanded sidebar occupies roughly 22 percent of the 833px-wide screenshot because the screenshot is narrow.
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
