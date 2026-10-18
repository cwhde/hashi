# AUD-144: AES GCM accepts empty plaintext but rejects its valid encrypted envelope

Priority: P3

Category: Correctness / encryption edge case

Status: open; confirmed by manual source trace.

## Finding

Encrypt permits an empty span and produces a12byte nonce plus16byte tag with zero ciphertext. FromBlob rejects lengths less than or equal to28, so Decrypt cannot read that valid envelope. The utility contract is internally inconsistent even if particular credential forms currently disallow blank values.

## Evidence

src/Hashi.Infrastructure/Crypto/AesGcmCipher.cs:16-23,37-49.

## Expected outcome and acceptance criteria

Either support authenticated empty messages by rejecting only lengths below28, or consistently reject empty input at encryption/write boundaries. Independently verify empty and nonempty round trips and malformed/truncated blobs.
