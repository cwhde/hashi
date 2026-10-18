# AUD-059: Pulse Linux installer is blocked by admin authentication

Priority: P1

Category: Missing working feature / agent installation

Spec reference: Base §§4,17; extended addendum §17.8.

## Finding

The installer download endpoint GET /api/pulse/install/linux.sh is marked AllowAnonymous, but the custom AdminApiAuthMiddleware does not consult AllowAnonymous metadata. Its hardcoded public list omits this route and only exempts Pulse heartbeat suffixes. A normal unauthenticated curl from the target host receives401 instead of the shell script, breaking the generated one-line installation flow.

## Evidence

- src/Hashi.Api/Features/Pulse/PulseEndpoints.cs:89-104 registers anonymous installer download.
- src/Hashi.Api/Hosting/AdminApiAuthMiddleware.cs:26-44,266-310 bypasses only listed public routes and heartbeat.

## Expected outcome

Explicitly admit the public installer download while retaining admin authentication for agent management and per-agent install metadata.

## Acceptance criteria

- Unauthenticated GET of the installer on the admin listener returns the intended script.
- Agent token management and install metadata remain authenticated.
- The generated one-line install fetch works from a fresh host without a browser session.

Verification: confirmed by manual source tracing; no live firewall/provider change was performed.
