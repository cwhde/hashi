# ADR 0004 — Live dragging in the Public pages editor uses our own small module

Date: 2026-11-22 · Status: accepted (the owner left the choice to Claude, 2026-11-22)

## Context

The Public pages editor (07 §8.1) orders one layout for both public pages. The owner rejected the first mock because
the browser's native drag and drop shows nothing until the item is let go; it has to feel like a phone's home screen or
Heimdall: the dragged item follows the pointer and the others slide out of its way while it moves.

What the editor needs from dragging:

- Two views of one order at once: the outline and the page preview. A drag in either moves the entry in both.
- Two kinds: entries inside groups (also across groups) and whole groups; "Other" stays last.
- Several checked entries dragged together, one Undo for the whole drag, one save (PUT) at release.
- Mouse, pen and touch; on touch a swipe must still scroll, so a drag starts from the grip or after holding still.
- Keyboard moves (Alt+Up/Down) and screen reader announcements, independent of the pointer code.
- The order lives in Svelte state; Svelte's keyed `{#each}` and `animate:` render it.

## Options

1. **A general sorting library that moves DOM nodes itself** (SortableJS style). It reorders elements behind Svelte's
   back, so the keyed blocks and the state disagree and have to be patched up after each drop; it knows one list at a
   time, not one order shown twice.
2. **A Svelte drag-and-drop library that owns the item arrays** (svelte-dnd-action style). Each drop zone keeps its own
   copy of its items with placeholder entries during a drag; two views of one order would mean several linked zones
   kept in step by hand, plus multi-drag and group drag on top.
3. **Our own module**: pointer events, Svelte's `flip` for the sliding, the state changes while the pointer moves. The
   mock version (`web/src/routes/design/sortable.ts`, about 180 lines) already does everything above and was tested in
   the mock: live reorder in both views, across groups, group drag, touch hold, swipe still scrolls.

## Decision

Option 3. `sortable.ts` moves to `web/src/lib/sortable.ts` as a Svelte action with a small host interface
(`start`, `over`, `end`); the editor's layout state implements it. Rules that came out of the mock and stay:

- Hit testing uses each item's place in the layout (its box minus any running transform), never where it is drawn:
  testing drawn boxes while others slide made items swap back and forth under a still pointer.
- Within one group the side follows the direction of travel; across groups, the pointer's half of the target.
- A release after a drag is not a click. Controls inside an item keep their own clicks, except a whole-item card.
- Reduced motion: the slide and lift animations drop to none.

Hashi stays at one runtime web dependency (openapi-fetch) apart from the UI kit.

## Consequences

- We own the bugs. Every rule above gets a test: unit tests for the host's `over()` decisions, and Playwright tests
  that drag with a mouse and with touch (hold, then drag; a swipe does not drag) and read the order before release.
- Accessibility does not depend on dragging: Alt+Up/Down, the selection bar's Move to, and announcements cover every move.
- If a third, different sortable surface appears (for example nested trees), look again at a library before stretching
  this module.
