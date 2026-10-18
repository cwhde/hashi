# AUD-143: Concurrent deletion can remove every remaining passkey

Priority: P2

Category: Reliability / passkey lifecycle

Status: open; confirmed by manual source trace.

## Finding

DeleteAsync checks the total credential count before deletion without locking/serializing the check and mutation. Two concurrent removals of the last two credentials can each observe count2, then both revoke sessions and delete their credential. Setup is already complete, so bootstrap does not provide the intended normal recovery login and the account loses its passkey login capability. This requires authenticated authorized concurrent mutations; it is not an anonymous credential deletion.

## Evidence

src/Hashi.Infrastructure/Auth/PasskeyAuthService.cs:143-160.

## Expected outcome and acceptance criteria

Serialize the last-credential invariant transactionally and make concurrent requests leave at least one usable credential. Revoke only sessions appropriate to a committed deletion, with an explicit supported recovery path.
