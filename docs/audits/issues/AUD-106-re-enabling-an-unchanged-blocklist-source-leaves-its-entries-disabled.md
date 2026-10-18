# AUD-106: Re-enabling an unchanged blocklist source leaves its entries disabled

Priority: P2

Category: Security / feed lifecycle

Status: open; confirmed by manual source trace.

## Finding

DisableAsync marks all source entries disabled. EnableAsync sets only the source flag and immediately performs a conditional fetch. When the upstream returns304, RefreshCoreAsync returns before ApplyParsedEntry can copy source.Enabled back to the entries. The UI reports an enabled source while none of its old entries enforce, indefinitely while the same ETag/content remains unchanged. Enforcement-mode edits similarly are not copied to existing entries on304; resync flags alone cannot change the stale per-entry mode.

## Evidence

src/Hashi.Infrastructure/Platform/BlocklistSourceManagementService.cs:117-155,259-274,399-415. This differs from AUD-071, which concerns observe-mode evaluation after entries have the correct mode.

## Expected outcome and acceptance criteria

Apply source lifecycle/enforcement changes to stored entries independently of a successful content change. Test disable, enable with304 and mode changes with304 using a synthetic conditional upstream; effective edge/firewall behavior must match the enabled source settings.
