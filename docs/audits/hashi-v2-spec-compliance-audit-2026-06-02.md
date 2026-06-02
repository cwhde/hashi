# Hashi v2 spec compliance audit - 2026-06-02

Scope: full reread of `docs/implementation-spec/hashi-v2-implementation-spec.md`, prior A/B/C audit reports and issue files, and review of the current backend, frontend, deployment, and Pulse agent implementation against the spec.

## Verification

- `git diff --check` returned clean.
- Added audit files were checked for trailing whitespace, final newlines, and non-ASCII characters.
- Runtime/frontend/Go verification was not rerun because this audit adds documentation-only issue files.

## New Issues

- `D01-resource-domain-mode-fallback-can-route-as-catch-all-and-skip-dns.md` - no explicit resource domain mode; blank-domain resources can render as catch-all routes while DNS/auth/dashboard skip or misidentify them.
- `D02-forward-auth-uses-proxy-ip-and-drops-request-context.md` - forward auth uses the proxy socket IP and omits forwarded method/context needed for rule evaluation and security buckets.
- `D03-blocklist-entries-are-ip-only-and-cannot-represent-required-block-state.md` - active block entries cannot represent ASN/country/region scopes, expiry, last-hit, source/creator, or per-host apply state.
- `D04-resource-rewrite-model-omits-replace-prefix-mode.md` - the resource rewrite model and renderer lack the required replace-prefix mode.
- `D05-pulse-install-surface-does-not-install-or-render-required-artifacts.md` - Pulse Linux install requires a preexisting binary/source checkout and Docker output is not a Compose snippet.
- `D06-geoip-setup-does-not-store-maxmind-credentials-or-update-databases.md` - GeoIP support only reads manually mounted local files; optional setup/update settings are missing.
- `D07-overview-widget-preferences-are-not-loaded-from-persisted-settings.md` - overview widget settings are persisted by the settings page but not loaded by the Overview page.

## Notes

I did not re-file prior A/B/C issues that appear materially addressed in the current tree, including Edge OIDC token validation, secure cookie flags, WAF event ingestion/exclusions, public dashboard safe DTOs, public status selection, script cron/manifest sync, and Pulse heartbeat metadata. The D-series issues focus on remaining spec conflicts observed in the current implementation.
