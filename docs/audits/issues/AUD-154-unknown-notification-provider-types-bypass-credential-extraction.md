# AUD-154: Unknown notification provider types bypass credential extraction

Priority: P2

Category: Security / notification settings validation

Status: open; confirmed by manual source trace.

## Finding

Provider create/update accepts arbitrary Type. BuildStoredSettingsJsonAsync extracts and encrypts known SMTP/Telegram/Discord credentials only in a type switch with no rejecting default, then stores the remaining JSON. An invalid/case-mismatched provider type can therefore store password/token fields as raw SettingsJson and later fail delivery as unsupported. Changing Type without SettingsJson also preserves the previous provider's incompatible config. This is a credential-at-rest invariant failure for malformed authenticated configuration, not an unauthenticated read path.

## Evidence

src/Hashi.Infrastructure/Notifications/NotificationDispatcher.cs:27-70,477-527; notification endpoint contracts do not restrict Type.

## Expected outcome and acceptance criteria

Validate/canonicalize provider types and type-specific settings before any save; reject unknown credential-bearing shapes. Provider-type changes must require a valid migrated/replaced configuration without persisting raw secrets or orphaning old credential records.
