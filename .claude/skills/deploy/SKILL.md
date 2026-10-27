---
name: deploy
description: Deploy the current Hashi image to the real Docker host and record it in progress.md.
disable-model-invocation: true
argument-hint: "[image tag, default: short SHA of HEAD]"
---

1. Confirm `make check` passes and the branch is pushed; the release workflow for the tag (`${ARGUMENTS:-$(git rev-parse --short HEAD)}`) is green.
2. `make deploy ENV=prod TAG=<tag>` (reads the target from `CLAUDE.local.md` / local env; the master key and `.env` stay on the host).
3. Wait for `/readyz`; run `make smoke ENV=prod`.
4. On failure: `make rollback ENV=prod`, then use the `debug` skill.
5. Append to the Log in `docs/progress.md`: date, tag, result.
