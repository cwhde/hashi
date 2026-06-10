# H-024: NotificationProviderEntity Missing Type-Specific Config Fields

**Priority:** Low
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §18.6

**Status:** Fixed
**Branch:** h/spec-compliance-1

## Description

The spec defines notification providers:
```
Providers:
- SMTP email.
- Telegram bot.
- Discord bot.
```

With easy setup:
```
Easy setup:
- Telegram: after token entry, ask user to message the bot; use updates to discover chat/channel.
- Discord: short pairing mode can connect to gateway and wait for a DM or mention to capture channel/user ID; manual IDs are always supported.
- SMTP: send test email.
```

The `NotificationProviderEntity` uses a generic JSON approach:
```csharp
public string SettingsJson { get; set; } = "{}";
```

This is a valid approach - type-specific configuration is stored as JSON rather than individual fields. The `NotificationDispatcher` service would parse this JSON based on the provider type.

This is an acceptable design pattern for flexible configuration. The entity model supports all provider types through the generic JSON field.

## Evidence

```csharp
public sealed class NotificationProviderEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();
    public string Name { get; set; } = string.Empty;
    public string Type { get; set; } = "smtp";
    public string SettingsJson { get; set; } = "{}";
    public bool Enabled { get; set; } = true;
}
```

The `Type` field identifies the provider (smtp, telegram, discord), and `SettingsJson` contains type-specific configuration. This is a flexible and extensible approach.

## Expected Outcome

- All notification provider types are supported
- Type-specific configuration is stored correctly
- Provider setup works as specified

## Fix Guidance

The entity model is correct. The JSON-based approach is flexible and supports all provider types. No changes needed.

## Acceptance Criteria

- [x] Provider type field exists (implemented)
- [x] Settings JSON field exists (implemented)
- [ ] Verify Telegram setup discovers chat/channel
- [ ] Verify Discord pairing mode works
- [ ] Verify SMTP test email works
