---
name: slice
description: Start or continue the current Hashi delivery slice (S0–S12). Use when the user says "continue", "next step", "work on the slice", "start S3", or asks what to build next.
argument-hint: "[slice id, optional]"
---

# Work on a delivery slice

1. Read `docs/progress.md`. Identify the current slice (or `$ARGUMENTS` if given). If the current slice has not passed its
   prod-proof gate, you are still on it — do not start another one. If "Blocked" has entries, raise them with the user first.
2. Read `docs/spec/README.md` (invariants) and only the spec files referenced by the slice row in `docs/spec/11-delivery.md` §3.
3. List the external systems this slice touches. For each, check `testdata/contracts/`. Missing contract → run the
   `contract-capture` skill first, or ask the user for access. Never write an adapter from memory.
4. Plan the thinnest end-to-end path that would satisfy the gate: DB → API (`api/openapi.yaml` first, `make generate`)
   → logic → UI → real system. Write the plan as a checklist into `docs/progress.md` under the slice. Keep each PR ≤ ~500 lines.
5. Implement one checklist item at a time. After each: `make check`. Commit with a conventional message.
6. When the checklist is done: deploy (ask the user to run `/deploy`), then execute the prod-proof gate with the user.
   Record date, version and evidence (command output, screenshots) in the slice's row.
7. Run the `review` skill on the slice's diff. Fix only reproduced/invariant findings; park the rest under "Unverified".
8. Update `docs/progress.md`: status, what's next, anything learned that changes the spec (spec edits in a separate `docs(spec):` commit).

Stop and ask the user when: a contract can't be captured, the spec is ambiguous, an action would change production state,
or the same failure survived two fix attempts.
