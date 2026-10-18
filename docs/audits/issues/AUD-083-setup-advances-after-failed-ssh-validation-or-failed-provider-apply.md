# AUD-083: Setup advances after failed SSH validation or failed provider apply

Priority: P1

Category: Setup / truthful validation

Status: open; confirmed by manual source tracing.

## Finding

Traefik and firewall setup call validateConnection but ignore its Succeeded=false response and retain the created connection ID. They display validated and allow progression. SystemResourceStep similarly ignores syncSystemResource.succeeded and advances after a200 failure response. Thus network/permission/provider errors are presented as successful setup milestones, leaving an unusable installation and obscuring the actual failure.

## Evidence

web/src/lib/components/setup/steps/TraefikConnectionStep.svelte:105-110,126-130; web/src/lib/components/setup/steps/FirewallHostStep.svelte:102-107,119-135; web/src/lib/components/setup/steps/SystemResourceStep.svelte:34-43

## Expected outcome

Check semantic success fields, retain actionable errors and gate progression on the required verified outcome. Invalidate validation when its inputs change.

## Acceptance criteria

HTTP200 validation/apply failures do not mark steps complete or display success; corrected inputs can retry without duplicating connections.

No live credentials, host scripts or services were changed for this review.
