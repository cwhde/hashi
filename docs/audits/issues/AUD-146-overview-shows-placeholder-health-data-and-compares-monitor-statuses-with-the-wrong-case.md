# AUD-146: Overview shows placeholder health data and compares monitor statuses with the wrong case

Priority: P1

Category: Missing feature / operational overview

Status: open; confirmed by manual source trace.

## Finding

AdminOverviewView compares monitors with Up/Degraded/Down while workers and monitor entities use lowercase up/degraded/down, so counts can remain zero for real monitors. Several required operational widgets still contain literal placeholders: Traefik pending changes, DNS drift, certificate expiry and a firewall widget that displays the Pulse agent count instead of firewall host health. Data is loaded once on mount and initial failures are silently hidden. This is missing operational information, independent of the lower-priority design foundation.

## Evidence

web/src/lib/components/admin/AdminOverviewView.svelte:26-58,94-119; src/Hashi.Infrastructure/Platform/MonitorCheckWorker.cs uses lowercase statuses; dashboard endpoint returns those monitor models.

## Expected outcome and acceptance criteria

Use the canonical status vocabulary, provide real backed data for the required widgets, refresh it appropriately and show load failures. Controlled datasets with down/degraded hosts, pending drift and expiring certificates must produce accurate actionable summaries.
