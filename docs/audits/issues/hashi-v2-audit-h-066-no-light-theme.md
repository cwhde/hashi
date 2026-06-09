# H-066: No Light Theme Implemented

**Priority:** High
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §22

## Description

The app only implements a dark theme. The `app.css` file defines CSS variables under `.dark` but there is no `:root` or `.light` variant with bright pink/violet colors. The theme setting in the UI is a plain text input with no toggle or dropdown. The spec requires a light theme that is bright pink/violet, high contrast, not beige, not washed out, and not monochrome.

## Evidence

- `web/src/app.css` only defines `.dark` selector with the Shades of Purple palette
- No light theme CSS variables exist
- Settings page theme input is a plain text field with no validation

## Expected Outcome

A light theme with pink/violet accent colors, bright background, high contrast foreground, toggled via the theme setting.

## Fix Guidance

1. Add a `:root` or `.light` class with bright pink/violet palette CSS variables.
2. Replace the theme text input with a dropdown or toggle offering "dark" and "light" options.
3. Wire the toggle to set the class on the HTML element.

## Acceptance Criteria

- [ ] Switching theme to "light" in settings applies a distinct bright pink/violet palette
- [ ] Light theme has high contrast and is not beige/washed-out/monochrome
- [ ] Theme selector offers only valid theme choices via dropdown/toggle
