# AUD-170: Vault input decoding and purpose validation produce invalid state or server errors

Priority: P2

Category: Input validation / API correctness

Status: open; confirmed by manual endpoint/service trace.

## Finding and evidence

Vault setup and secret storage call Convert.FromBase64String directly without validating malformed input or mapping FormatException to a client error. Enum.TryParse accepts numeric values not defined by SecretPurpose, and storage falls through to the generic class/no purpose tag rather than rejecting the unknown value. Thus malformed requests become500 responses or persist semantically unsupported purposes. This is independent of the actual PRF base64url incompatibility AUD-081 and the empty-envelope utility defect AUD-144.

Evidence: `src/Hashi.Api/Features/Vault/VaultEndpoints.cs:39-49,92-104`; `src/Hashi.Infrastructure/Auth/SecretRecordService.cs:169-194` default mappings; `src/Hashi.Core/Auth/VaultModels.cs` supported enum vocabulary.

## Expected outcome and acceptance criteria

Validate encoding/decoded length and Enum.IsDefined (or a string vocabulary) before invoking state-changing services. Invalid input must return a clear4xx response with no partial persisted secret/setup state. Accepted purpose/encoding values must retain the intended classification and capability restrictions.
