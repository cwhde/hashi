# AUD-156: Uptime strips discard timestamps and mislabel multi-minute history

Priority: P2

Category: Wrong implementation / monitoring history

Status: open; confirmed by manual source trace.

## Finding

MonitorStatusStrip accepts only up booleans, truncates/pads to60 positions and labels the strip Last60minutes. Callers discard bucket timestamps/intervals, so gaps in the middle are compressed into adjacent cells and padding is moved to the beginning. Five/hourly rollups are still labeled minutes. The chart cannot distinguish a real continuous interval from missing samples and misrepresents longer selected ranges.

## Evidence

web/src/lib/components/monitoring/MonitorStatusStrip.svelte:2-30; public/PublicStatusView.svelte and routes/(admin)/status/+page.svelte convert rollups to up-only cells.

## Expected outcome and acceptance criteria

Carry bucket start/resolution and selected window into the strip, construct the real time grid and preserve unknown gaps. Label the actual interval/window and expose an accessible equivalent; missing telemetry must not become implied measured uptime/downtime.
