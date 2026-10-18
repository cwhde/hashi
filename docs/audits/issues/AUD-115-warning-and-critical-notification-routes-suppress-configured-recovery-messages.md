# AUD-115: Warning and critical notification routes suppress configured recovery messages

Priority: P2

Category: Wrong implementation / recovery notifications

Status: open; confirmed by manual source trace.

## Finding

RouteMonitorTransitionAsync classifies recovery as info. RouteEventAsync applies the ordinary severity threshold before checking SendRecovery; therefore warning/critical routes never send recovery even with SendRecovery=true. Cooldown can additionally suppress the paired recovery. This is separate from AUD-026, which covers missing degraded transition alerts.

## Evidence

src/Hashi.Infrastructure/Platform/NotificationRoutingService.cs:19-53,84-108,125-138. Follow both the initial alert and its recovery when applying route severity and cooldown.

## Expected outcome and acceptance criteria

Define recovery routing in terms of the matching prior incident/route and SendRecovery, with explicit cooldown semantics. A warning/critical down event followed by recovery must send both when enabled, and suppress recovery when disabled.
