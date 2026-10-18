# AUD-074: AdGuard rewrite client expects the wrong response formats

Priority: P1

Category: AdGuard / API compatibility

Status: open; confirmed by manual source tracing.

## Finding

The official AdGuard list endpoint returns a JSON array, but ListRemoteRewritesAsync calls TryGetProperty on the root expecting an object with rewrites. That throws on an array, breaking every plan/reconcile path. PushToAdGuardAsync also parses every successful add response as JSON with an optional id, while the official add contract promises200 with no JSON body; an empty success becomes failure after the remote write.

## Evidence

Paths below are relative to src/Hashi.Infrastructure/:

Platform/AdGuardSyncService.cs:678-699,952-970

## Expected outcome

Implement the actual AdGuard Home array response and empty add success contract, with explicit compatible-version handling.

## Acceptance criteria

Realistic array list and empty200 add responses support preview/apply/verify successfully; malformed responses still fail safely.

No live service configuration or firewall was changed to establish this finding.

Primary reference: [Upstream contract/source](https://github.com/AdguardTeam/AdGuardHome/blob/master/AGHTechDoc.md#api-list-rewrite-entries).
