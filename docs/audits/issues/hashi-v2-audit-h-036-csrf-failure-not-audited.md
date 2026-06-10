# H-036: CSRF Validation Failure Not Audited in AdminCsrfMiddleware

**Priority:** Medium
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §9, §27

**Status:** Not Started
**Branch:** 

## Description

`AdminCsrfMiddleware.cs` catches `AntiforgeryValidationException` and returns a 403 response, but does not log or audit the failure:

```csharp
// AdminCsrfMiddleware.cs
catch (AntiforgeryValidationException)
{
    context.Response.StatusCode = 403;
    return; // No audit log, no logging
}
```

CSRF validation failures are a critical security signal — they indicate either:
1. A legitimate user with an expired/missing CSRF token (usability issue)
2. An active CSRF attack attempt (security incident)

Silently returning 403 without any audit event or log entry means security incidents go undetected, and operators have no visibility into potential attacks. The spec §9 requires CSRF protection and §27 lists unsafe admin endpoints requiring CSRF. The spec also requires audit logging for security-significant events.

## Evidence

```csharp
// AdminCsrfMiddleware.cs — the catch block
catch (AntiforgeryValidationException)
{
    context.Response.StatusCode = 403;
    return;
}
```

No call to `ILogger.LogWarning` or `AuditService.WriteAsync` exists in the catch block.

## Expected Outcome

Every CSRF validation failure should:
1. Be logged at `Warning` level with the request path, method, and client IP (non-sensitive metadata only)
2. Write an audit event with category `auth`, action `csrf_validation_failed`
3. Include enough context to identify whether this is a pattern of attacks or isolated failures

## Fix Guidance

1. Inject `ILogger<AdminCsrfMiddleware>` into the middleware constructor.
2. Add `logger.LogWarning("CSRF validation failed for {Method} {Path} from {ClientIp}", ...)` before returning 403.
3. Optionally inject `AuditService` and write an audit event (if available in the middleware pipeline).
4. Ensure client IP is resolved from forwarded headers (use existing `ForwardedClientContextResolver`).

## Acceptance Criteria

- [ ] CSRF failure generates a log entry at Warning level
- [ ] Log entry includes request path, HTTP method, and client IP
- [ ] Audit event is recorded if `AuditService` is available
- [ ] No sensitive data (cookies, tokens, headers) is logged
