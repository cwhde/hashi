# H-081: Vault Setup Tradeoff Not Explicit in Service-Sync Flow

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** §8 (Tradeoff must be explicit in setup)

## Description

Vault setup response returns `ServiceSyncWrapStored` but no acknowledgment step or explanation of the tradeoff (unattended sync vs reduced security). Users are not informed that enabling service-sync vault means secrets are decryptable without a browser session.

## Evidence

- Vault setup endpoint returns success after storing the service-sync wrap without any acknowledgment prompt
- No UI or API step explains that service-sync vault reduces security by making secrets accessible without an active browser session

## Expected Outcome

Setup should explain the security tradeoff of service-sync vault before enabling it. Users must explicitly acknowledge that server compromise with the vault key exposes sync secrets.

## Fix Guidance

1. Add acknowledgment step to vault setup explaining the tradeoff.
2. Require explicit consent before enabling service-sync vault.
3. Include clear language about what "unattended sync" means for secret exposure.

## Acceptance Criteria

- [ ] Setup explains the security tradeoff of service-sync vault
- [ ] User must explicitly acknowledge before enabling
- [ ] Tradeoff explanation mentions that server compromise with vault key exposes sync secrets
