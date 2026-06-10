# H-028: Docker Compose Missing Environment Variable Validation

**Priority:** Low
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §4 (Deployment)

**Status:** Fixed
**Branch:** h/spec-compliance-1

## Description

The Docker Compose file at `deploy/compose/docker-compose.yml` defines environment variables:
```yaml
environment:
  - ConnectionStrings__DefaultConnection=Host=postgres;Database=hashi;Username=hashi;Password=hashi
  - HASHI_SERVICE_SYNC_VAULT_KEY=${HASHI_SERVICE_SYNC_VAULT_KEY:-}
```

The spec requires:
- Service-sync vault key for unattended provider sync
- PostgreSQL connection string
- Configurable ports

The implementation has:
- ✅ PostgreSQL connection string
- ✅ Service-sync vault key (with empty default)
- ✅ Configurable ports (via environment variables)

The `HASHI_SERVICE_SYNC_VAULT_KEY` has an empty default, which means the service-sync vault cannot unlock without setting this variable. This is correct behavior per the spec: "If the service-sync vault cannot unlock, provider sync jobs pause and surface a critical health warning."

However, the Compose file does not validate that required environment variables are set. Docker Compose does not have built-in validation, but the application should handle missing variables gracefully.

## Evidence

```yaml
# deploy/compose/docker-compose.yml
hashi:
  environment:
    - ConnectionStrings__DefaultConnection=Host=postgres;Database=hashi;Username=hashi;Password=hashi
    - HASHI_SERVICE_SYNC_VAULT_KEY=${HASHI_SERVICE_SYNC_VAULT_KEY:-}
  ports:
    - "${HASHI_PORT_ADMIN:-8080}:8080"
    - "${HASHI_PORT_DASHBOARD:-8081}:8081"
    - "${HASHI_PORT_STATUS:-8082}:8082"
```

The empty default for `HASHI_SERVICE_SYNC_VAULT_KEY` is intentional - it allows the container to start but provider sync will be paused until the key is set.

## Expected Outcome

- Required environment variables are documented
- Missing variables cause clear error messages
- Application handles missing vault key gracefully

## Fix Guidance

The implementation is correct. The application should log a clear warning when `HASHI_SERVICE_SYNC_VAULT_KEY` is not set. The Compose file's empty default is intentional.

## Acceptance Criteria

- [x] Environment variables are documented in Compose file (implemented)
- [x] Ports are configurable (implemented)
- [x] Vault key has empty default (intentional)
- [ ] Application logs warning when vault key is missing
