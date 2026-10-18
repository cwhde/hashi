# AUD-153: Vault lock and unlock race with borrowed root key spans

Priority: P3

Category: Crypto lifecycle / concurrency

Status: open; confirmed by manual source trace.

## Finding

VaultSessionState returns a span over the mutable key array stored in a concurrent dictionary. Lock or another unlock can zero that same array while an in-flight operation encrypts/decrypts, producing nondeterministic crypto failures. Two concurrent UnlockForSession calls can both miss the other's replacement and leave discarded key arrays unzeroed. The dictionary protects map operations but does not make key borrowing, replacement and zeroization atomic.

## Evidence

src/Hashi.Infrastructure/Auth/VaultSessionState.cs:19-58; SecretRecordService reads GetRootKeyOrThrow for envelope operations.

## Expected outcome and acceptance criteria

Define an operation-scoped key lease/copy with synchronized lifecycle and reliable zeroization of replaced copies. Concurrent lock/unlock/decrypt must fail predictably or complete with a valid leased key, without retaining discarded root material.
