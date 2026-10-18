# AUD-087: Several advertised manual allow bypass flags are not enforced

Priority: P2

Category: Security / manual bypass settings

Status: open; confirmed by manual source tracing.

## Finding

The manual-entry contract/UI persists BypassAdaptiveEscalation, BypassRateLimit and BypassSso, but DecideForwardAuthAsync consumes only BypassBlocking and BypassChallenge. Rate limiting is bypassed by BypassBlocking regardless of BypassRateLimit. SSO bypass is never applied. Access-log ingestion checks legacy FirewallAllowedSubjects rather than the new manual-entry adaptive bypass, so allowed subjects still accumulate automatic abuse bans.

## Evidence

src/Hashi.Infrastructure/Platform/SecurityDecisionService.cs:91-110,178-199,405-445; src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:44-69,1122-1133; src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:441-449

## Expected outcome

Apply each explicit bypass only to its named decision stage and scope, using one consistent effective-policy resolver across ingestion and forward auth.

## Acceptance criteria

Each bypass flag independently controls its documented stage; a default allow never bypasses SSO/CAPTCHA; adaptive bypass prevents automatic escalation but retains telemetry.

No live credentials, host scripts or services were changed for this review.
