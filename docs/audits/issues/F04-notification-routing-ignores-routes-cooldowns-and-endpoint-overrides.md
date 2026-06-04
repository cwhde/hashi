# F04 - Notification routing ignores routes, cooldowns, and endpoint overrides

Priority: Medium

Spec conflicts: section 18.6 requires notification routing with global defaults, per-endpoint overrides, severity thresholds, cooldowns, and recovery notifications.

## Problem

Notification provider setup exists, but routing is not implemented as specified. Monitor transitions and security events are sent to every enabled provider type, independent of route records, endpoint matches, severity thresholds, cooldown windows, or recovery-notification preferences.

There is a `NotificationRouteEntity`, but it is not exposed through the API/UI and it is not used by `NotificationRoutingService`. As a result, operators cannot configure global defaults versus endpoint-specific routes, suppress noisy endpoints with cooldowns, choose severity thresholds, or decide which routes receive recovery notifications.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:1067-1073` requires global defaults, per-endpoint override, severity thresholds, cooldowns, and recovery notifications.
- `src/Hashi.Infrastructure/Platform/NotificationRoutingService.cs:24-44` routes monitor down/recovered transitions to all enabled provider types.
- `src/Hashi.Infrastructure/Platform/NotificationRoutingService.cs:47-58` routes security events to all enabled provider types.
- `src/Hashi.Infrastructure/Platform/NotificationRoutingService.cs:61-66` queries distinct enabled provider types from `NotificationProviders`, not enabled notification routes.
- `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:558-577` defines `NotificationRouteEntity` with provider, event kind, severity, and match JSON fields, but no cooldown or recovery fields.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:736-782` exposes notification provider CRUD, provider tests, Telegram chat discovery, and manual send, but no route management endpoints.
- `web/src/lib/components/settings/NotificationsSettings.svelte:18-38` tracks provider form state only.
- `web/src/lib/components/settings/NotificationsSettings.svelte:249-302` renders provider setup for SMTP, Telegram, and Discord webhook configuration, but no routing, cooldown, severity, or endpoint override controls.

## Expected outcome

Notifications should be routed through configurable rules. Global defaults should exist, endpoints should be able to override routing, severity thresholds should be honored, cooldowns should suppress repeated notifications, and recovery notifications should be controlled by route settings.

## Fix guidance

Implement route CRUD in the backend contracts, API, and settings UI. Extend the route model with cooldown and recovery-notification settings. Update `NotificationRoutingService` to resolve matching enabled routes for each event, apply severity and endpoint matching, enforce cooldowns using delivery history or route state, and create/send deliveries for the selected providers only.

Keep provider setup separate from routing setup, but ensure at least one usable default route can be created during setup so basic alerts still work.

## Acceptance criteria

- Operators can configure notification routes with provider, event kind, severity threshold, endpoint/resource match, cooldown, and recovery behavior.
- Monitor and security notifications are sent only to providers selected by matching enabled routes.
- Per-endpoint overrides can replace or refine global defaults.
- Cooldowns prevent repeated notifications for the same route/subject within the configured window.
- Recovery notifications are sent or suppressed according to route configuration.
- Tests cover global defaults, endpoint overrides, severity thresholds, cooldown suppression, and recovery behavior.
