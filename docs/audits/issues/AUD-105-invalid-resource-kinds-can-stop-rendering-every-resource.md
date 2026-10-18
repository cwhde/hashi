# AUD-105: Invalid resource kinds can stop rendering every resource

Priority: P1

Category: Reliability / resource validation

Status: open; confirmed by manual source trace.

## Finding

CreateResourceRequestValidator requires Kind merely to be nonempty. ResourceService persists it unchanged and BuildDefinitionsAsync Enum.Parse processes all stored rows before any enabled-resource filtering. An authenticated request with Kind=invalid creates a durable row that makes every subsequent definition build throw, including after disabling that row. Validation also does not establish the complete kind/scheme/port combinations before persistence. The global impact is a configuration/rendering outage rather than code execution.

## Evidence

src/Hashi.Core/Validation/RequestValidators.cs:7-20; src/Hashi.Infrastructure/Platform/PlatformServices.cs:31-82,803-849; src/Hashi.Api/Features/Resources/ResourceEndpoints.cs:29-52. AUD-035 separately covers unsafe interpolation of otherwise accepted values.

## Expected outcome and acceptance criteria

Validate the supported kind enum and routing combinations at every write boundary before saving; handle legacy malformed rows with explicit per-resource errors without aborting the entire render. Validate complete routes/rules before the initial save and commit related rows transactionally.
