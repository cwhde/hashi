# AUD-165: Session status never reports completed setup

Priority: P2

Category: Wrong implementation / authentication contract

Status: open; confirmed by manual source trace.

## Finding and evidence

GET `/api/auth/session` derives SetupComplete from a Name claim equal to `setup-complete`. SignInAsync always issues Name=`admin`, and a full source search finds no producer of that setup-complete claim. The response therefore reports false after real setup completion. The login page uses that value when redirecting an already authenticated user, sending them through setup incorrectly. AdminAppShell separately reads real setup state, so this is not evidence that every normal login is permanently blocked.

Evidence: `src/Hashi.Api/Features/Auth/AuthEndpoints.cs:195-208,338-356`; `web/src/routes/login/+page.svelte:24-30`; `AdminAppShell.svelte` uses getSetupStatus instead.

## Expected outcome and acceptance criteria

Return setup completion from the authoritative setup service/state, keeping it consistent across session/status/setup endpoints and after completion without stale cookie claims. A completed installation must return SetupComplete=true and navigate directly to the intended admin view.
