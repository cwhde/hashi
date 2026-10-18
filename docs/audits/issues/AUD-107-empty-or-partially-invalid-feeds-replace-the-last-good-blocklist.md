# AUD-107: Empty or partially invalid feeds replace the last good blocklist

Priority: P2

Category: Reliability / feed last good state

Status: open; confirmed by manual source trace.

## Finding

RefreshCoreAsync rejects only zero parsed entries combined with at least one parse error. Empty content or comment-only/error-page text classified as ignored can succeed with zero errors and disable every old entry. A response with one valid entry and many invalid rows also replaces the entire old set; parse errors are merely recorded. Transport success is treated as semantic success without a configurable empty-list or rejection-ratio safeguard, so an accidental malformed upstream update can silently remove protection.

## Evidence

src/Hashi.Infrastructure/Platform/BlocklistSourceManagementService.cs:277-305,351-396. This is separate from the network body timeout AUD-067 and the source304 lifecycle AUD-106.

## Expected outcome and acceptance criteria

Define explicit empty-list and partial-parse policies, preserve the previous effective set on suspicious parse failure, and provide an intentional reviewed path for a legitimate empty feed. Report parse quality and removal size before destructive replacement.
