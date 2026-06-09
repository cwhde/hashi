# H-034: Admin Session Cookie Expiry Hardcoded to 8 Hours

**Priority:** Medium
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §9, §24

## Description

In `src/Hashi.Api/Program.cs`, the admin auth session cookie sliding expiry is hardcoded:

```csharp
// Program.cs line ~65
options.Cookie.SlidingExpiration = true;
options.Cookie.Expiration = TimeSpan.FromHours(8);
```

The spec §9 states: "Session timeout configurable" and §24 lists "Session duration" under Security settings. Currently there is no mechanism to change the 8-hour default from configuration or through the settings UI.

## Evidence

```csharp
// Program.cs
builder.Services.AddAuthentication(CookieAuthenticationDefaults.AuthenticationScheme)
    .AddCookie(options =>
    {
        // ...
        options.Cookie.Expiration = TimeSpan.FromHours(8);
        // ...
    });
```

The `AppSettingsEntity` in `CoreEntities.cs` does not include a `SessionDurationMinutes` or `SessionDurationHours` field — only edge SSO session settings exist (`EdgeSsoSessionHours`, `EdgeSsoIdleTimeoutMinutes`).

## Expected Outcome

- Admin session duration should be configurable via a settings field (e.g., `AdminSessionMinutes`)
- The default should still be 8 hours
- The setting should be loadable from the database settings table and applied at authentication configuration time

## Fix Guidance

1. Add `AdminSessionMinutes` (or hours) field to `AppSettingsEntity` with default 480 (8 hours).
2. Read this setting dynamically and apply to cookie configuration.
3. Add the setting to the Security section of the settings UI.
4. Ensure edge SSO session settings remain separate from admin session settings.

## Acceptance Criteria

- [ ] Admin session expiry can be changed in settings
- [ ] Default remains 8 hours (480 minutes)
- [ ] Changing the setting takes effect on the next session creation
- [ ] Setting is persisted in PostgreSQL and survives restart
