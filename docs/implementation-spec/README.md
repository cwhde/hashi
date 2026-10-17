# Implementation specifications

These documents describe the required product behavior. Read the base specification first, then its addenda. A narrower addendum controls when it gives a more specific requirement.

1. [Hashi V2 implementation specification](hashi-v2-implementation-spec.md) — product, architecture, feature, deployment, security, and acceptance requirements.
2. [Hashi V2 implementation addendum](hashi-v2-implementation-spec-addendum.md) — security subjects, blocklists, CAPTCHA, Pulse target binding, internal DNS, and CI/CD requirements.
3. [Admin session token management addendum](hashi-v2-admin-session-token-management-addendum.md) — server-side admin sessions, IP binding, scopes, expiry, revocation, and audit requirements. This controls where it is more specific than the base authentication section.
4. [UI design foundation addendum](hashi-v2-ui-design-foundation-addendum.md) — shared visual system, themes, responsive behavior, and representative validation scenes. Page composition and information architecture are explicitly deferred by that document.

The base specification identifies itself as the implementation source of truth. Addenda extend it; they do not silently replace unrelated requirements. The implementation plan and audit files provide execution history and evidence, not alternate product specifications.
