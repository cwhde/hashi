# H-032: Hardcoded Default PostgreSQL Password in Dependency Injection Registration

**Priority:** High
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §30, Non-Negotiable Rule Set §3 (#29 never commit real secrets)

**Status:** Not Started
**Branch:** 

## Description

`DependencyInjection.cs` in `src/Hashi.Infrastructure/` contains a hardcoded default PostgreSQL connection string with an embedded password:

```csharp
// Line ~28-29
var fallbackConnectionString = "Host=localhost;Port=5432;Database=hashi;Username=hashi;Password=hashi";
var connectionString = configuration.GetConnectionString("Hashi") ?? fallbackConnectionString;
```

This hardcoded fallback includes the password `hashi` as plaintext directly in the source code. While this is a "default" that would typically be overridden by configuration, the mere presence of credentials in source code violates the principle that no secrets should ever be committed.

Furthermore, if an environment or deployment inadvertently fails to provide the connection string configuration, the application silently connects to `localhost` with default credentials, which could connect to an unintended PostgreSQL instance.

## Evidence

```csharp
// DependencyInjection.cs
var fallbackConnectionString = "Host=localhost;Port=5432;Database=hashi;Username=hashi;Password=hashi";
var connectionString = configuration.GetConnectionString("Hashi") ?? fallbackConnectionString;
```

## Expected Outcome

1. The fallback connection string should not contain a password (use trusted auth or require explicit configuration).
2. If no connection string is configured, the application should fail fast with a clear error message at startup rather than silently using a hardcoded default.
3. No credentials should exist as string literals in source code.

## Fix Guidance

1. Remove the hardcoded fallback string entirely.
2. Require the connection string to be explicitly provided via environment variable or configuration.
3. At startup, validate that the connection string is present and log a clear error if missing.
4. Alternatively, provide a development-only fallback using `appsettings.Development.json` (excluded from Docker/production builds).

## Acceptance Criteria

- [ ] No hardcoded database password exists in any source file
- [ ] Application fails fast with clear error if no connection string is provided (non-dev)
- [ ] Secret scanning tools (gitleaks) do not flag any source files
- [ ] Docker Compose `docker-compose.yml` explicitly provides the connection string via env var
