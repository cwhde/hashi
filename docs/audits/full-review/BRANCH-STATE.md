# Branch and pull request reconciliation — 2026-10-18

## Fresh remote state

- Default branch: `main`.
- Remote implementation baseline: `49f34dc1a8b7b3ec7332588b39311d66fe13cabc`.
- `git ls-remote --heads origin` returns only `main`; no remote implementation fix/redesign branch remains unmerged.
- Fresh Forgejo REST inventory:15 total PRs,15 merged, zero unmerged PRs, zero open issue objects. The project has historically tracked findings as Markdown rather than Forgejo issues.
- The earlier authorized cleanup recorded62 ancestor branches removed and two documentation-only redesign commits fast-forwarded into main, then that branch removed. No additional branch deletion was necessary in this final check. The removed branch names are not reconstructed or invented from PR titles.
- Current review adds documentation locally. That audit documentation is not an unmerged implementation fix; this review introduces no application fixes. Remote source was not changed during the restarted manual pass.

Merged state proves integration, not correctness. See the historical issue register and current residual IDs for features whose earlier “fix” was merged but remains incomplete or incorrect.

## PR inventory

| PR | Title | State | Base | Former head | Merge commit | Merged at |
| --- | --- | --- | --- | --- | --- | --- |
| [1](https://git.juzo.io/juzo/hashi/pulls/1) | feat: add Gatus monitoring support for raw TCP/UDP resources | merged | main | refs/pull/1/head | `dcda156294bad830b933e76234c25188f25d3563` | 2025-12-09T18:43:41+01:00 |
| [2](https://git.juzo.io/juzo/hashi/pulls/2) | V2 foundation: phases −1–14 scaffold | merged | main | refs/pull/2/head | `382025b65b64c8a7f3ef7200a7b76768409a748a` | 2026-05-26T16:35:39+02:00 |
| [3](https://git.juzo.io/juzo/hashi/pulls/3) | Add Traefik access-log ingestion worker for passive abuse signal collection. | merged | main | refs/pull/3/head | `621141ccb6ca36e96edbbc331ec7090a17058899` | 2026-05-26T19:28:58+02:00 |
| [4](https://git.juzo.io/juzo/hashi/pulls/4) | Feat/gap 2 request buckets | merged | main | refs/pull/4/head | `5c707d9bfa3f2844db41ec4243cfe07755d13245` | 2026-05-26T19:36:09+02:00 |
| [5](https://git.juzo.io/juzo/hashi/pulls/5) | Add monitor check dispatch for non-HTTP endpoints. | merged | main | refs/pull/5/head | `c8ef65e25651f8bdc5cea541cc5a4237b71845fa` | 2026-05-26T19:38:20+02:00 |
| [6](https://git.juzo.io/juzo/hashi/pulls/6) | Add native PostgreSQL partitioning for monitor sample retention. | merged | main | refs/pull/6/head | `9418fa2be1033668b7dbfc176c1f046e4825d35e` | 2026-05-26T19:42:15+02:00 |
| [7](https://git.juzo.io/juzo/hashi/pulls/7) | Wire FluentValidation for resource and SSH request DTOs. | merged | main | refs/pull/7/head | `6b007807a4ab6a05e96003bbd66656397e398596` | 2026-05-26T19:43:14+02:00 |
| [8](https://git.juzo.io/juzo/hashi/pulls/8) | feat(web): add advanced resource routes editor UI | merged | main | refs/pull/8/head | `08637adc5494ca7e7fbf58641560e1deebba148d` | 2026-05-26T19:44:01+02:00 |
| [9](https://git.juzo.io/juzo/hashi/pulls/9) | Wire optional setup step to provider APIs. | merged | main | refs/pull/9/head | `1c64a55a7013c3b198fd1e2610ce063609f81f6c` | 2026-05-26T19:44:55+02:00 |
| [10](https://git.juzo.io/juzo/hashi/pulls/10) | Add CodeMirror shell editor for scripts page | merged | main | refs/pull/10/head | `55e24f06b4f36e6ccdda3c1c8e1999cc84b5683f` | 2026-05-26T19:45:35+02:00 |
| [11](https://git.juzo.io/juzo/hashi/pulls/11) | Add guided notification provider setup flows (Gap 9). | merged | main | refs/pull/11/head | `0f23121637d836f137d71edf1f5d3039373660df` | 2026-05-26T19:46:34+02:00 |
| [12](https://git.juzo.io/juzo/hashi/pulls/12) | Complete security dashboard with DB aggregations and filters. | merged | main | refs/pull/12/head | `af8c10833095f7a922731f83fa8a228ac95ab64d` | 2026-05-26T19:49:03+02:00 |
| [13](https://git.juzo.io/juzo/hashi/pulls/13) | feat: rewrite hashi pulse agent to .NET 10 Native AOT | merged | main | refs/pull/13/head | `ccdf08c46d1f6051f8908a9e3cb843418c013cb6` | 2026-06-05T22:11:02+02:00 |
| [15](https://git.juzo.io/juzo/hashi/pulls/15) | Audit series h | merged | main | refs/pull/15/head | `6de30f5bbf7cbe579136d4e277190a06da9dc153` | 2026-06-12T14:18:38+02:00 |
| [16](https://git.juzo.io/juzo/hashi/pulls/16) | Feat/admin session token management | merged | main | refs/pull/16/head | `677de892b925db58a96770c388ea45c79145608b` | 2026-06-13T09:11:42+02:00 |
