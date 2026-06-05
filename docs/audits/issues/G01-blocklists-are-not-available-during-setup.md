# G01 - Recommended blocklists are not available during setup

Priority: Medium

Spec conflicts: addendum sections 11.1 and 8.5

## Problem

The addendum requires setup to include recommended blocklist selection, custom URL entry, parsed-entry preview, and enforcement mode selection before enabling a source. The setup optional step has CAPTCHA, AdGuard, internal agent DNS, notifications, and GeoIP, but it does not expose any blocklist source setup path.

This means a fresh install can complete without ever being offered the security setup path that the addendum makes part of setup, and users must discover the later security settings page manually. It also means the required preview-before-enable flow is absent from setup.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:970` says setup must include optional steps for Cap CAPTCHA, recommended blocklists, and internal agent DNS.
- `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:987` lists blocklist setup fields: individual recommended feed checkboxes, custom URL option, parsed-entry preview before enabling, and enforcement mode.
- `web/src/lib/components/setup/steps/OptionalStep.svelte:17` through `web/src/lib/components/setup/steps/OptionalStep.svelte:22` define optional-step state for OIDC, AdGuard, internal agent DNS, notifications, GeoIP, and CAPTCHA only.
- `web/src/lib/components/setup/steps/OptionalStep.svelte:278`, `web/src/lib/components/setup/steps/OptionalStep.svelte:309`, `web/src/lib/components/setup/steps/OptionalStep.svelte:359`, and `web/src/lib/components/setup/steps/OptionalStep.svelte:371` render the optional panels, with no blocklist panel.
- `web/src/lib/setup/steps.ts:72` describes the optional step as "OIDC, AdGuard, notifications, GeoIP, and dashboard widgets.", again omitting blocklists.

## Expected outcome

The setup flow should offer recommended blocklist feeds and custom blocklist URLs as first-run optional setup, including parsed preview and explicit enforcement mode selection before enabling any source.

## Fix guidance

Add a blocklist section to the setup optional step that reuses the existing blocklist source APIs where possible. Seed/display recommended sources, allow custom source creation, expose middleware/firewall enforcement mode, and require a preview result before enabling a source from setup.

## Acceptance criteria

- Setup optional steps include recommended blocklist selection.
- Setup allows a custom blocklist URL and parser format/options where applicable.
- Setup previews parsed entries and parser errors before enabling a blocklist source.
- Setup lets the user choose enforcement mode before enabling.
- The setup step description and tests reflect the blocklist setup path.
