# AUD-114: Five minute idle settings expire sessions despite continuous activity

Priority: P2

Category: Reliability / active admin sessions

Status: open; confirmed by manual source trace.

## Finding

AdminSessionService permits a5minute idle timeout but persists activity only after5minutes. At or after that same boundary it first rejects IdleExpiresAtUtc before reaching the renewal branch. Requests every few seconds do not renew the session, so an actively used5minute session always expires at its initial deadline. Larger timeouts also lose up to the write interval of recent activity. This is a rolling-idle implementation defect, not an absolute-lifetime bypass.

## Evidence

src/Hashi.Infrastructure/Auth/AdminSessionService.cs:19-20,34-35,120-135. Admin session addendum9.1 requires rolling idle expiry based on activity.

## Expected outcome and acceptance criteria

Track activity at a cadence safely below the minimum idle limit or use a bounded cache/write policy preserving rolling semantics. A synthetic clock with continuous traffic at the minimum supported timeout must remain valid until genuinely idle or absolutely expired.
