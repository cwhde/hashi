# H-059: Vault Lacks Secret Classes and Purpose-Specific Keys — Single Key Compromise Exposes All Secrets

**Priority:** High
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §8 (three classes of secrets: session-unlocked, service-sync, server-operational; purpose-specific vault keys)

## Description

The spec requires three distinct secret classes with different access semantics: (1) session-unlocked secrets for recovery-only vault material, secret reveal, and destructive approval, (2) service-sync secrets for SSH, DNS tokens, AdGuard, notifications, and ACME that background sync needs, and (3) server-operational secrets for cookie signing and data protection that must be available at boot. The implementation only has a boolean `IsServiceSyncEligible` flag — no "session-unlocked" or "server-operational" class exists. Additionally, the spec requires purpose-specific vault keys (separate wrapping keys for SSH, DNS, OIDC purposes), but the implementation wraps all DEKs with a single admin root key. Compromising this one key exposes all secrets.

## Evidence

SecretRecordEntity has only `IsServiceSyncEligible` boolean; no class enum or purpose-specific key references. SecretRecordService.StoreAsync() wraps all DEKs with `session.GetRootKeyOrThrow()` — a single key for all purposes.

## Expected Outcome

Three explicit secret classes with different access semantics. Purpose-specific vault keys so compromising one key doesn't expose all secrets.

## Fix Guidance

(1) Add a SecretClass enum (SessionUnlocked, ServiceSync, ServerOperational) to SecretRecordEntity. (2) Add purpose-specific vault keys (e.g., SshVaultKey, DnsVaultKey, OidcVaultKey) to VaultWrappedKeyEntity. (3) Route secret encryption to the correct class/purpose key at storage time.

## Acceptance Criteria

- [ ] Secrets are categorized into three classes with different access requirements
- [ ] DEKs are wrapped by purpose-specific vault keys
- [ ] Compromising one vault key does not expose secrets of other purposes
