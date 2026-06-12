# Hashi V2 Addendum: Admin Session Token Management

## 1. Purpose

This document extends the Hashi V2 implementation specification with server-side admin session token management.

The goals are:

1. Bind each issued admin session token to the client IP address observed when that token is created.
2. Limit inactive sessions to a maximum four-hour rolling window.
3. Limit every session to an eight-hour absolute lifetime regardless of activity.
4. Attach explicit authorization scopes to each session.
5. Support immediate server-side revocation, inspection, cleanup, and audit.

This addendum does not bind the Hashi admin account to one IP address. The same administrator may authenticate from multiple IP addresses. Each successful login creates a distinct token, and each token is valid only from the IP address to which it was issued.

## 2. Compatibility With Existing Specifications

The implementation must preserve these existing requirements:

1. Hashi remains a single-user administrative application.
2. Passkey authentication is required after setup.
3. Admin authentication continues to use secure, HTTP-only cookies.
4. Unsafe admin endpoints require CSRF protection.
5. High-risk operations require recent passkey reauthentication.
6. Session keys and raw authentication tokens are never logged.
7. Vault unlock material remains isolated by admin session.
8. Public, edge-auth, and Pulse heartbeat endpoints keep their separate authentication models.

Where this addendum is more specific than the original configurable session-timeout requirement, this addendum controls admin session behavior.

## 3. Terminology

### Admin Session Token

A cryptographically random bearer credential issued after successful bootstrap or passkey authentication and transported in the `hashi.session` cookie.

The client-visible token must not contain authorization policy, client IP, vault material, or other sensitive server-side state in plaintext.

### Session Record

The authoritative server-side record associated with one admin session token. It stores token identity, IP binding, scopes, timestamps, authentication method, and revocation state.

### Bound IP

The canonical client IP address observed by Hashi when the token is issued. A token presented from any other canonical client IP is invalid.

### Idle Lifetime

The maximum duration since the session's last accepted activity. The default and maximum idle lifetime is four hours.

### Absolute Lifetime

The maximum duration since the original authentication that issued the token. The default and maximum absolute lifetime is eight hours. Activity and recent reauthentication do not extend this deadline.

### Recent Reauthentication

A successful passkey assertion performed inside an existing valid admin session. It authorizes high-risk operations for five minutes but does not create a new session or reset the absolute lifetime.

## 4. Non-Goals

The following are out of scope:

1. Binding the administrator account itself to one IP address.
2. Reusing one token after the client's IP address changes.
3. Automatically moving a token to a new IP after reauthentication.
4. Replacing passkeys with API keys or long-lived personal access tokens.
5. Applying admin session scopes to Edge SSO or Pulse agent tokens.
6. Treating IP binding as a substitute for TLS, CSRF protection, passkeys, or recent reauthentication.

## 5. Required Session Model

Hashi stores one `admin_sessions` row per issued token with at least:

* `id`: random 256-bit session identifier or an equivalent identifier with at least 128 bits of entropy.
* `auth_method`: `bootstrap` or `passkey`.
* `bound_ip`: canonical IPv4 or IPv6 address.
* `scopes`: explicit list of allowed admin scopes.
* `created_at_utc`.
* `last_seen_at_utc`.
* `idle_expires_at_utc`.
* `absolute_expires_at_utc`.
* `reauthenticated_at_utc`, nullable.
* `revoked_at_utc`, nullable.
* `revocation_reason`, nullable.
* `user_agent_hash`, nullable, for correlation and anomaly visibility without storing the complete header.

Raw cookie values and vault keys must never be stored in the session table or audit log.

## 6. Token Issuance

On successful authentication:

1. Resolve the canonical client IP through the trusted-proxy policy.
2. Generate a new random session identifier.
3. Create the server-side session record.
4. Set `created_at_utc` and `last_seen_at_utc` to the current UTC time.
5. Set `idle_expires_at_utc` to four hours after creation.
6. Set `absolute_expires_at_utc` to eight hours after creation.
7. Attach the scopes appropriate for the authentication flow.
8. Issue the secure `hashi.session` cookie.
9. Audit session creation using a non-secret session correlation value.

Every login creates a new token. Logging in from another IP is allowed and creates another independently bound session.

Privilege changes must issue a new session identifier or update server-side scopes only after successful passkey reauthentication.

## 7. Strict Per-Token IP Binding

For every authenticated request:

1. Resolve the canonical client IP using only direct connection data or forwarding headers received from a configured trusted proxy.
2. Normalize IPv4-mapped IPv6 addresses to canonical IPv4 form.
3. Compare the canonical request IP with the session's `bound_ip` using exact address equality.
4. Reject the session if the addresses differ.

No subnet tolerance, roaming allowance, or automatic rebinding is permitted in strict mode.

An IP mismatch must:

* Reject the request as unauthenticated.
* Revoke the affected session token.
* Clear the authentication cookie where possible.
* Remove vault material held for that session.
* Write an audit event that includes the stored and observed canonical IP addresses but no token value.

The administrator may log in again from the new IP, producing a new token bound to that IP.

## 8. Trusted Proxy Requirements

Strict IP binding is only correct when Hashi has a trustworthy canonical client IP.

Hashi must:

1. Ignore forwarding headers from untrusted direct peers.
2. Maintain an explicit trusted-proxy CIDR configuration.
3. Parse forwarding chains according to trusted-proxy boundaries rather than trusting arbitrary client-supplied leftmost values.
4. Use the same client-IP resolver for token issuance and token validation.
5. Fail closed if the stored or observed address cannot be normalized.

Changes to trusted-proxy configuration are high-risk and require recent reauthentication.

## 9. Timeout Semantics

### 9.1 Idle Timeout

An admin session is invalid when:

```text
now >= idle_expires_at_utc
```

Accepted authenticated activity advances `last_seen_at_utc` and `idle_expires_at_utc`, capped by `absolute_expires_at_utc`.

To avoid a database write on every request, Hashi may persist activity at a bounded interval of no more than five minutes. The validation calculation must not grant more than the configured four-hour inactivity period.

### 9.2 Absolute Timeout

An admin session is invalid when:

```text
now >= absolute_expires_at_utc
```

The absolute deadline is fixed when the token is issued. Sliding-cookie renewal, normal activity, vault unlock, and five-minute reauthentication must not change it.

### 9.3 Expiry Behavior

When either timeout is reached, Hashi must:

* Reject the session.
* Clear the cookie where possible.
* Remove session vault material.
* Mark or delete the expired server-side record according to retention policy.
* Record an expiry audit event without logging the token.

## 10. Session Scopes

Each session has an explicit allowlist of scopes. Unknown scopes do not grant access.

Initial scope vocabulary:

* `admin.read`: read protected admin state.
* `admin.write`: perform ordinary administrative mutations.
* `settings.manage`: modify application and security settings.
* `secrets.manage`: reveal, add, replace, or remove secrets and change vault state.
* `sync.apply`: apply provider and generated-state sync plans.
* `firewall.apply`: apply or roll back firewall state.
* `scripts.manage`: create, modify, or execute privileged scripts.
* `security.manage`: modify blocks, blocklists, CAPTCHA, and security policy.

The normal passkey admin session receives the complete supported scope set. Bootstrap sessions receive only the scopes required to complete setup and must remain invalid after setup completes.

Scope checks and recent reauthentication are cumulative. A high-risk request succeeds only if the session has the required scope and has completed recent reauthentication where required.

## 11. Reauthentication

Recent reauthentication remains valid for five minutes.

The timestamp is stored on the authoritative session record. It must survive normal application process restarts and must be cleared when the session is revoked or expires.

Reauthentication must not:

* Change `bound_ip`.
* Extend `absolute_expires_at_utc`.
* Create a replacement token unless an explicit token-rotation flow is requested.
* Restore a revoked or expired session.

## 12. Revocation and Logout

Logout revokes the current server-side session before clearing the cookie.

Hashi must support:

* Revoking the current session.
* Revoking one selected session.
* Revoking all sessions other than the current session.
* Revoking all sessions after a security-sensitive recovery or credential event.

Deleting a passkey should revoke sessions authenticated with that credential when credential identity is available on the session record.

Revocation takes effect on the next request and must not depend on cookie expiry.

## 13. API Surface

Required admin APIs:

* `GET /api/auth/session`: current session status, scopes, and expiry metadata.
* `GET /api/auth/sessions`: list active admin sessions with masked correlation ID, bound IP, creation time, last activity, absolute expiry, and current-session marker.
* `DELETE /api/auth/sessions/{sessionId}`: revoke one session.
* `POST /api/auth/sessions/revoke-others`: revoke every session except the caller's current session.
* Existing reauthentication and logout endpoints continue to operate on the current server-side session.

Session-management mutations require CSRF protection. Revoking sessions other than the current session requires recent reauthentication and `security.manage`.

## 14. Audit and Privacy

Audit these events:

* Session issued.
* Session renewed for activity.
* Session expired by idle timeout.
* Session expired by absolute timeout.
* Session rejected because of IP mismatch.
* Session revoked by logout.
* Session revoked manually.
* Other sessions revoked in bulk.
* Scope validation failure.
* Recent reauthentication success and failure.

Never log:

* Raw cookie values.
* Raw session identifiers.
* Passkey assertions.
* Vault keys or decrypted secrets.

Use a salted hash or short non-secret correlation identifier when audit correlation is required.

## 15. Cleanup and Retention

A background cleanup operation must remove or archive:

* Expired sessions.
* Revoked sessions older than the configured audit-retention interval.
* Orphaned in-memory vault session material.

Cleanup failure must not make expired or revoked sessions valid again.

## 16. Migration

Implementation order:

1. Add the `admin_sessions` table and required indexes.
2. Add server-side issuance and validation while retaining the existing cookie name.
3. Invalidate pre-migration cookies that do not reference a valid server-side session.
4. Move recent-reauthentication state from process memory to the session record.
5. Add session inspection and revocation APIs.
6. Add periodic cleanup.

No compatibility period is required for old admin cookies. Deployment may require administrators to log in again once.

## 17. Testing Requirements

### 17.1 Unit Tests

Add tests for:

* Canonical IPv4 and IPv6 comparison.
* IPv4-mapped IPv6 normalization.
* Idle expiry calculation.
* Absolute expiry enforcement.
* Activity extension capped by absolute expiry.
* Scope-to-endpoint mapping.
* Unknown and missing scope rejection.
* Five-minute reauthentication calculation.

### 17.2 Integration Tests

Add tests proving:

* A token works from its issuance IP.
* The same token is rejected and revoked from a different IP.
* Two logins from different IPs create two independently valid tokens.
* Activity extends idle expiry but never absolute expiry.
* A token is rejected after four hours of inactivity.
* A continuously active token is rejected after eight hours.
* Recent reauthentication does not reset absolute expiry.
* Revocation takes effect before cookie expiry.
* Missing scopes reject otherwise authenticated requests.
* Application restart does not lose session or recent-reauthentication state.

### 17.3 Security Tests

Add tests proving:

* Untrusted forwarding headers cannot select the bound IP.
* Trusted proxy forwarding produces the same canonical IP during issuance and validation.
* Raw tokens are absent from audit events and application logs.
* Expired, revoked, malformed, and unknown sessions fail closed.

## 18. Acceptance Criteria

The feature is complete when:

1. Every newly issued admin token has an authoritative server-side session record.
2. Each token is strictly usable only from the exact canonical IP to which it was issued.
3. Logging in from another IP creates a separate token rather than moving an existing token.
4. Idle lifetime is no more than four hours.
5. Absolute lifetime is no more than eight hours from original token issuance.
6. Activity and recent reauthentication never extend the absolute deadline.
7. Every protected admin endpoint requires an appropriate explicit session scope.
8. High-risk endpoints still require five-minute recent passkey reauthentication.
9. Logout and manual revocation invalidate server-side state immediately.
10. Vault material is removed when its session is revoked, expires, or fails IP validation.
11. Trusted-proxy handling cannot be bypassed with arbitrary forwarding headers.
12. Session lifecycle events are auditable without exposing bearer credentials.
