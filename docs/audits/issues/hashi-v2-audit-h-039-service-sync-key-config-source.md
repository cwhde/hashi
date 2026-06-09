# H-039: ServiceSyncVaultBootstrapper Reads Key From IConfiguration — Could Be Source-Controlled

**Priority:** High
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §8, Non-Negotiable Rule Set §3 (#29 never commit real secrets)

## Description

`ServiceSyncVaultBootstrapper` in `src/Hashi.Infrastructure/Auth/ServiceSyncVaultBootstrapper.cs` reads the service-sync vault key from `IConfiguration`:

```csharp
// ServiceSyncVaultBootstrapper.cs lines ~15-17
var serviceSyncKey = configuration["HASHI_SERVICE_SYNC_VAULT_KEY"];
```

`IConfiguration` aggregates multiple sources including `appsettings.json`, `appsettings.Development.json`, environment variables, and command-line arguments. If the key is placed in an `appsettings.json` file (even unintentionally), it will be committed to source control and distributed with the application.

The spec §8 explicitly states: "Service-sync vault keys are wrapped by a Docker secret or equivalent deployment secret so routine sync can run after restart without a logged-in browser session." Additionally, the Docker Compose file already passes `HASHI_SERVICE_SYNC_VAULT_KEY` as an environment variable, so environment variable reading is the intended path. However, the blanket use of `IConfiguration` allows the key to leak through other configuration sources.

## Evidence

```csharp
// ServiceSyncVaultBootstrapper.cs
var serviceSyncKey = configuration["HASHI_SERVICE_SYNC_VAULT_KEY"];
```

There is no check to ensure the value came from an environment variable or Docker secret source rather than a file-based configuration source.

## Expected Outcome

1. The service-sync vault key should be read exclusively from environment variables (or Docker secrets mounted as files), not from `IConfiguration`.
2. Or, validate that the value did not come from `appsettings.json` by checking the configuration source.
3. If key is missing, log a clear error and mark service-sync vault as unavailable (already done).
4. Optionally: warn if the key appears to have been read from a file-based configuration source.

## Fix Guidance

1. Use `Environment.GetEnvironmentVariable("HASHI_SERVICE_SYNC_VAULT_KEY")` directly instead of `IConfiguration`.
2. Alternatively, keep `IConfiguration` but add a startup validation step that checks the key is not present in any committed configuration files.
3. Add a `.gitleaks` pattern to detect base64-encoded 32-byte strings in configuration files.

## Acceptance Criteria

- [ ] Service-sync vault key cannot be read from `appsettings.json` or any other file-based config source
- [ ] Key is read exclusively from environment variables or Docker secrets
- [ ] Missing key produces clear error log and disables service-sync vault (existing behavior preserved)
- [ ] Secret scanning detects if the key appears in committed config files
