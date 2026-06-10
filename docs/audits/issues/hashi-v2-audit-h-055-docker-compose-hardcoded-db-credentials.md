# H-055: Docker Compose Dev Environment Uses Hardcoded Credentials and Exposes PostgreSQL to Host

**Priority:** Medium
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §30, §33; Non-Negotiable Rule Set §3 (#29 never commit real secrets)

**Status:** Fixed
**Branch:** h/backend-quality

## Description

`docker-compose.dev.yml` exposes PostgreSQL on the host network with hardcoded credentials:

```yaml
# docker-compose.dev.yml
services:
  postgres:
    image: postgres:18
    ports:
      - "5432:5432"
    environment:
      POSTGRES_DB: hashi
      POSTGRES_USER: hashi
      POSTGRES_PASSWORD: hashi
```

The production `docker-compose.yml` also uses `hashi` as both username and password:

```yaml
POSTGRES_DB: hashi
POSTGRES_USER: hashi
POSTGRES_PASSWORD: hashi
```

The dev compose file additionally exposes PostgreSQL port 5432 to the host machine, which means any process on the development machine can connect to the database with well-known credentials. In the production compose file, PostgreSQL is not port-mapped (only accessible within the Docker network), but the credentials are still well-known defaults.

## Evidence

- `deploy/compose/docker-compose.dev.yml` — Port 5432 exposed, credentials `hashi:hashi`
- `deploy/compose/docker-compose.yml` — Port not exposed, but credentials still `hashi:hashi`

## Expected Outcome

1. Dev environment: PostgreSQL port should only be exposed if explicitly required, and credentials should be overridable via environment variables.
2. Production environment: Credentials should be required as environment variables or Docker secrets, not hardcoded defaults.
3. A `.env.example` file should document required credentials without containing defaults.

## Fix Guidance

1. In `docker-compose.yml`, use environment variable substitution for PostgreSQL credentials:
   ```yaml
   POSTGRES_USER: ${POSTGRES_USER:-hashi}
   POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?required}
   ```
2. Create a `.env.example` file with placeholder values.
3. In `docker-compose.dev.yml`, remove the port mapping or guard it behind an environment variable (`${POSTGRES_EXPOSE_PORT:-}`).
4. Document that production deployments MUST change the default credentials.

## Acceptance Criteria

- [ ] Production compose file does not contain hardcoded default credentials (or requires them via env vars)
- [ ] Dev compose file does not expose PostgreSQL port by default
- [ ] `.env.example` exists with documented required variables
- [ ] Docker Compose fails with clear error if required credentials are not provided
