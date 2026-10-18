# AUD-090: Credential lookup migration locks out existing passkeys

Priority: P1

Category: Upgrade / authentication

Status: open; confirmed by manual source tracing.

## Finding

FixPendingModelChanges adds non-null CredentialIdBase64 with an empty-string default and never backfills existing CredentialId byte arrays. PasskeyAuthService now locates assertions solely by equality on CredentialIdBase64. Every pre-migration credential therefore becomes unfindable despite its valid public key and credential bytes remaining in the database. Once setup is complete bootstrap login is disabled; users can lose admin access during upgrade.

## Evidence

src/Hashi.Infrastructure/Persistence/Migrations/20260610065242_FixPendingModelChanges.cs:91-96; src/Hashi.Infrastructure/Auth/PasskeyAuthService.cs:115-125

Confirmed by tracing the migration and lookup; fresh registrations populate the field and do not expose this upgrade failure.

## Expected outcome

Backfill normalized base64 credential IDs for existing rows, retain safe compatibility during rolling recovery, and index/validate the lookup field.

## Acceptance criteria

A passkey registered before this migration still logs in afterward without re-registration or database edits. Base64 representation matches Convert.ToBase64String including long IDs.
