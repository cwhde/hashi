# H-093: Missing static.juzo.io Asset References

**Priority:** Low
**Conflict Type:** missing_implementation
**Spec Reference:** §22 (Default assets can be fetched from https://static.juzo.io/)

**Status:** Not Started
**Branch:** 

## Description

No references to `static.juzo.io` exist in the V2 codebase. The spec requires icons, logos, and backgrounds from this CDN. The NavRail uses a letter "H" instead of an icon image.

## Evidence

- No `static.juzo.io` URLs anywhere in the codebase
- NavRail renders a letter "H" instead of a logo image
- No favicon link referencing the CDN

## Expected Outcome

Logo and favicon should reference `static.juzo.io` URLs. NavRail should show a logo image instead of a letter. Assets should be cached locally after first fetch.

## Fix Guidance

1. Replace the "H" letter in NavRail with an `img` tag referencing `static.juzo.io`.
2. Add favicon link to `app.html`.
3. Add optional background image support.
4. Implement local caching of fetched assets.

## Acceptance Criteria

- [ ] Logo and favicon reference static.juzo.io URLs
- [ ] NavRail shows logo image instead of letter
- [ ] Assets are cached locally after first fetch
