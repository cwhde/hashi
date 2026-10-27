---
name: review
description: Review a Hashi diff or branch against the spec's acceptance criteria and invariants, in a fresh read-only reviewer. Use after finishing a slice checklist item or before a PR, or when the user asks for a review.
argument-hint: "[git range or branch, default: main...HEAD]"
context: fork
agent: spec-reviewer
---

Review the changes in `${ARGUMENTS:-main...HEAD}` (run `git diff` yourself).

1. Find the slice in `docs/progress.md` and the spec sections it references (`docs/spec/11-delivery.md` §3).
2. Check every acceptance point of those sections and every invariant in `docs/spec/README.md` the diff could affect.
3. Check that external interactions are backed by fixtures in `testdata/contracts/` (no invented API shapes).
4. Skip style, naming and refactoring suggestions entirely.

Report at most 10 findings, most severe first. For each: file:line, the spec clause or invariant, a concrete failing
scenario, and a classification:

- **reproduced** — you demonstrated it (test, command output) → must fix
- **invariant** — violates a README invariant by reading the code → must fix
- **unverified** — plausible but not demonstrated → goes to "Unverified" in progress.md, not fixed now

Also list acceptance points that are not implemented or not tested. If nothing qualifies, say "no findings" — that is a
valid and good result. Do not pad the list.
