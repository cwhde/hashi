# AUD-004 - Successful WebAuthn login challenges remain reusable

Priority: High (P1)

Category: Security / authentication replay

Specification reference: `docs/implementation-spec/hashi-v2-implementation-spec.md`, sections 3 and 9 (secure passkey authentication); WebAuthn challenge/response protocol requirements.

## Problem

The login and reauthentication endpoints look up their WebAuthn options from `WebAuthnChallengeStore`, verify an assertion, but never remove or mark the challenge as consumed. The cache entry remains valid for five minutes. A captured successful assertion can therefore be replayed against the same challenge during that window. Authenticators that report a zero sign counter do not provide a counter-based replay signal, so replay may create additional admin sessions or repeat reauthentication.

## Evidence

- `src/Hashi.Infrastructure/Auth/WebAuthnChallengeStore.cs` implements `GetLogin` and `GetRegistration` with `IMemoryCache.Get`; it has no consume/remove operation.
- `src/Hashi.Api/Features/Auth/AuthEndpoints.cs` calls `GetLogin` in both `/passkeys/login/complete` and `/reauthenticate/complete`, then validates the assertion without consuming the challenge.
- `PasskeyAuthService.CompleteLoginAsync` updates the authenticator sign counter, but authenticators with no counter support can report zero on each assertion.

## Expected outcome

Each WebAuthn challenge is accepted at most once. A successful login or reauthentication consumes its challenge atomically; subsequent use of the same challenge identifier is rejected. Failed assertions may be retried only according to a deliberately defined attempt policy.

## Acceptance criteria

- Add atomic consume semantics for login and reauthentication challenges.
- Consume a challenge on successful assertion validation and reject subsequent requests using it.
- Bound failed attempts and preserve the short expiry.
- Add a regression test that submits the same valid assertion twice and proves the second request cannot authenticate or refresh reauthentication.
