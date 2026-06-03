# Hashi v2 spec compliance audit - E-series - 2026-06-02

Scope: fresh reread of `docs/implementation-spec/hashi-v2-implementation-spec.md`, review of prior audit issue/report formatting only for style, review of the current backend, frontend, workflow, deployment, DNS, resource, Pulse, and security implementation, and review of the latest public `ci.yml` and `docker-build-pulse.yml` failures for commit `52cdfc3481`.

## Verification

- Public Actions page reviewed: https://git.juzo.io/juzo/hashi/actions
- `ci.yml #322` web job log reviewed for the OpenAPI contract failure: https://git.juzo.io/juzo/hashi/actions/runs/353/jobs/839/logs
- `docker-build-pulse.yml #323` native artifact job log reviewed for the unsupported artifact action failure: https://git.juzo.io/juzo/hashi/actions/runs/354/jobs/841/logs
- This audit did not re-run the full test suite because the changes are documentation-only issue files.

## New Issues

- `E01-openapi-and-frontend-contract-artifacts-are-not-generator-clean.md` - committed OpenAPI/frontend contract artifacts are stale and the current CI OpenAPI verification step fails on main.
- `E02-pulse-native-artifact-workflow-uses-unsupported-upload-action.md` - Pulse native binaries build, but artifact upload fails because `actions/upload-artifact@v4` is unsupported on the current Gitea/GHES-compatible runner.
- `E03-dns-target-matching-drops-private-manual-and-routed-host-targets.md` - DNS matching drops private manual IPs before managed-host matching and omits NetBird-routed subnet and configured host FQDN/on-route data.
- `E04-resource-rule-actions-use-inconsistent-vocabularies.md` - resource rule actions can be stored with values that forward-auth enforcement does not recognize, causing matching rules to silently fall through.
- `E05-abuse-decision-state-machine-collapses-required-states.md` - abuse/security subject state is collapsed to `watch`, `challenge`, and `block`, missing the explicit staged states required by the spec.
- `E06-resource-model-omits-proxy-protocol-and-monitoring-hints.md` - the resource model omits the TCP proxy protocol option and monitoring protocol hint specified for resources.

## Notes

I did not re-file setup required-minimums, vault completion, passive sync, background job status, resource domain modes, rewrite modes, forward-auth forwarded context, blocklist shape, GeoIP update settings, WAF exclusions, public dashboard DTO safety, overview widget persistence, or Pulse heartbeat metadata because the current implementation materially covers those previously reported areas.

I also did not file the latest `ci.yml` backend and Pulse-agent setup failures as repository issues. The public logs available for the substantive web job show a deterministic repository diff; the other failed jobs appeared to fail during runner setup/log retrieval rather than at a project command with actionable output.
