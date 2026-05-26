# A12 - Notification provider secrets are stored as plaintext JSON

Priority: Critical

Spec conflicts: non-negotiable rule 5; sections 9.2 and 23. Notification tokens must be encrypted and never returned/logged as plaintext.

## Problem

Notification provider settings are stored as raw JSON in `notification_providers.SettingsJson`. SMTP passwords, Telegram bot tokens, and Discord webhook URLs are read directly from that JSON. The API request model accepts an opaque `SettingsJson` blob, so there is no structured secret handling or vault wrapping.

Telegram chat discovery accepts a raw bot token and uses it directly in the request URL. Even though this may be convenient, it increases the chance of tokens appearing in logs, traces, or browser history.

## Evidence

- `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:380-388` stores notification settings in plaintext JSON.
- `src/Hashi.Infrastructure/Notifications/NotificationDispatcher.cs:21-34` saves request `SettingsJson` directly.
- `src/Hashi.Infrastructure/Notifications/NotificationDispatcher.cs:58-61` updates `SettingsJson` directly.
- `src/Hashi.Infrastructure/Notifications/NotificationDispatcher.cs:202-242` reads SMTP password, Telegram bot token, and Discord webhook from `SettingsJson`.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:524-532` models Telegram token and provider settings as request payload fields instead of secret ids.

## Expected outcome

Notification secrets should be stored through the encrypted secret record/vault path. Provider records should store non-secret metadata plus secret ids. API responses should never return secret values. Discovery/test flows should avoid putting tokens in URLs where practical and should redact errors/logs.

## Fix guidance

Replace `SettingsJson` for secrets with structured fields and `SecretRecordId`s. Keep non-secret provider settings relational or in JSON. Add migration to move existing secrets into vault records. Ensure test/discovery methods use secret ids or one-time request bodies with redacted logging.

## Acceptance criteria

- SMTP password, Telegram token, and Discord webhook are encrypted at rest.
- Listing providers never returns secret material.
- Updating a provider can leave existing secrets unchanged without resubmitting them.
- Tests verify plaintext tokens do not appear in stored provider JSON or API responses.
