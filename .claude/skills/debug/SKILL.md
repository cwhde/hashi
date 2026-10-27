---
name: debug
description: Fix a bug or failing test in Hashi using reproduce-first with a two-attempt cap. Use when something fails, a test is red, the user reports a bug, or a fix didn't work.
---

# Reproduce, then fix — at most twice

1. **Reproduce.** Write a failing test, or record the failing observation on the real system (exact command + output).
   No reproduction → no fix; tell the user what you need to reproduce it.
2. **Locate.** State one hypothesis in a sentence, with the evidence for it. If the failure involves an external system,
   compare against `testdata/contracts/` — is the fixture still true? Re-capture if in doubt (`contract-capture` skill).
3. **Fix** the cause, not the symptom. Never weaken assertions, add sleeps, skip tests, or edit `TestInvariant_*`.
4. **Verify.** The reproduction passes and `make check` passes.
5. If attempt 2 still fails: stop. Under "Blocked" in `docs/progress.md` write the reproduction, both hypotheses, what each
   attempt changed and what happened. Revert speculative changes. Ask the user.

Signs you are looping — stop immediately: editing the same test back and forth, adding/removing mocks, fixing a finding
that creates a new finding in the same area, or "fixing" something you can't reproduce.
