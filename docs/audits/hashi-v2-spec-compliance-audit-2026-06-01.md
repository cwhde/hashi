# Hashi v2 spec compliance audit - 2026-06-01

Scope: full reread of `docs/implementation-spec/hashi-v2-implementation-spec.md`, prior audit reports, and review of backend, frontend, deployment, and Pulse agent implementation against the spec.

## Verification

- `dotnet test Hashi.slnx /p:SkipFrontendBuild=true` passed after this write-up's issue files were added: 211 unit tests and 22 integration tests passed.
- `git diff --check` passed.
- Frontend, Go, and Docker verification were not rerun because this audit adds documentation-only issue files.

## New Issues

- `C01-certificate-setup-cannot-bind-or-use-real-dns-provider.md` - certificate setup checks the wrong DNS connection type and has no provider binding/token path for Traefik ACME.
- `C02-hashi-internal-urls-are-still-hard-coded-to-port-8080.md` - forward-auth, health routing, and Hashi API monitoring still assume the default admin port.
- `C03-waf-rendering-and-ingestion-are-incomplete.md` - multi-resource WAF YAML is invalid/ambiguous, exclusions are missing, and WAF events are not ingested.
- `C04-public-app-api-exposes-internal-resource-details.md` - anonymous public apps return the full admin resource payload and ignore the dashboard-disabled flag on the admin port.
- `C05-public-dashboard-omits-manual-dns-tiles-and-required-summary-fields.md` - manual DNS dashboard tiles, display-name enforcement, and required public dashboard summary details are missing.
- `C06-public-status-publishes-all-enabled-monitors.md` - public status has no public/private endpoint selection and exposes all enabled monitors.
- `C07-adguard-topology-sync-deletes-managed-rewrites-without-user-confirmation.md` - topology sync can delete Hashi-created manual rewrites and internally confirms destructive applies.
- `C08-dns-record-planning-cannot-handle-multi-value-records-or-conflicts.md` - DNS name/type keys block valid multi-value records and hide manual/generated conflicts.
- `C09-manual-dns-audit-events-store-subjects-in-the-outcome-field.md` - manual DNS audit calls pass subject data as positional outcome/subjectType arguments.
- `C10-edge-sso-still-uses-custom-volatile-oidc-flow.md` - Edge SSO still uses manual in-memory OIDC state and token exchange/validation instead of the required platform OIDC flow.
- `C11-pulse-agent-model-and-heartbeat-contract-are-incomplete.md` - Pulse lacks required model fields and heartbeat metadata.
- `C12-script-sync-does-not-install-host-cron-or-manifest.md` - script sync copies files but does not install host cron/timers or a hash manifest, and target defaults are wrong.
- `C13-admin-and-edge-cookies-can-be-issued-without-secure-flag.md` - admin, CSRF, and edge session cookies can be issued without `Secure`.

## Notes

I did not re-file prior A/B issues that appear materially addressed, such as notification secret storage, Pulse source availability, and the broad manual DNS CRUD absence. The C-series items focus on remaining spec conflicts observed in the current tree.
