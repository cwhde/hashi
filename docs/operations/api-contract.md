# API Contract Workflow

Hashi V2 uses an OpenAPI-first contract between backend and frontend. The frontend must not invent API shapes; it consumes the published OpenAPI document and generated TypeScript client.

## Source of truth

- OpenAPI document: `openapi/hashi.json`
- Generated types: `web/src/lib/api/schema.d.ts`
- Hand-written fetch helpers: `web/src/lib/api/client.ts` (must align with generated paths)

## Backend change rules

Every backend change that adds, removes, or modifies an HTTP endpoint MUST, in the same commit:

1. Update typed DTOs in `src/Hashi.Contracts/Api/`
2. Export OpenAPI: `./scripts/export-openapi.sh`
3. Regenerate frontend types: `./scripts/generate-api-client.sh`
4. Commit `openapi/hashi.json` and `web/src/lib/api/schema.d.ts` together with the backend code

Do not merge backend API work without refreshing the contract artifacts.

## Commands

```bash
# Export OpenAPI from the running API (no database required)
./scripts/export-openapi.sh

# Regenerate frontend types from openapi/hashi.json
./scripts/generate-api-client.sh

# Or from web/
pnpm run generate:api
```

## OpenAPI export environment

OpenAPI export runs the API with:

- `ASPNETCORE_ENVIRONMENT=OpenApiExport`
- `HASHI_SKIP_STARTUP_HOOKS=1`

This loads `src/Hashi.Api/appsettings.OpenApiExport.json` and skips PostgreSQL migrations so export works without a database.

## CI expectation

CI should verify that committed `openapi/hashi.json` and `web/src/lib/api/schema.d.ts` match a fresh export from the built API.

## Currently published admin APIs

### Health & setup

- `GET /api/health`
- `GET /api/setup/status`
- `GET /api/setup/bootstrap-allowed`
- `POST /api/setup/steps/{stepSlug}/complete`
- `POST /api/setup/complete`

### Settings & activity

- `GET /api/settings/general`
- `PUT /api/settings/general`
- `GET /api/activity/audit`

### Auth (Phase 2)

- `POST /api/auth/bootstrap/login`
- `POST /api/auth/passkeys/register/begin`
- `POST /api/auth/passkeys/register/complete`
- `POST /api/auth/passkeys/login/begin`
- `POST /api/auth/passkeys/login/complete`
- `GET /api/auth/session`
- `POST /api/auth/logout`

### Vault (Phase 2)

- `GET /api/vault/status`
- `POST /api/vault/recovery-key/generate`
- `POST /api/vault/setup`
- `POST /api/vault/unlock`
- `POST /api/vault/lock`
- `GET /api/vault/secrets`
- `POST /api/vault/secrets`
- `POST /api/vault/verify-unlock`
