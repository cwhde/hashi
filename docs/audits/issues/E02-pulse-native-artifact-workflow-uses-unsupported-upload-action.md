# E02 - Pulse native artifact workflow uses unsupported upload action

Priority: Medium

Spec conflicts: section 4 requires Pulse to be available as a small static native binary and as a Docker image. Section 30 requires Gitea workflows to build and publish release artifacts reliably.

## Problem

The Pulse workflow now builds linux/amd64 and linux/arm64 binaries, but the native artifact job fails before publishing them because it uses `actions/upload-artifact@v4`. The current Gitea/GHES-compatible runner reports that artifact v2 and `upload-artifact@v4` are not supported there.

The Docker image job can still succeed, but the native binary path that the install workflow depends on is red on the current main branch.

## Evidence

- `.gitea/workflows/docker-build-pulse.yml:30-40` builds linux/amd64 and linux/arm64 Pulse binaries and checksum files.
- `.gitea/workflows/docker-build-pulse.yml:42-49` uploads those files with `actions/upload-artifact@v4`.
- The latest public `docker-build-pulse.yml #323` `build-native-artifacts` job for commit `52cdfc3481` fails at that upload step: https://git.juzo.io/juzo/hashi/actions/runs/354/jobs/841/logs
- The log shows the binaries and checksum files were built, then reports: `upload-artifact@v4+ and download-artifact@v4+ are not currently supported on GHES.`
- The previous D05 issue covered the install surface not producing usable artifacts. That surface has improved, but this workflow still prevents native artifacts from being published successfully.

## Expected outcome

Pulse native binaries should be produced and attached or otherwise made available by the Gitea workflow without depending on an action version unsupported by the runner.

## Fix guidance

Replace the unsupported artifact step with a Gitea-compatible artifact mechanism. Options include using an older supported upload action, attaching the files directly to releases through the Gitea API in the same job, or using a first-party Gitea artifact upload path if available in this runner version. Keep the Docker image build separate if that remains reliable.

## Acceptance criteria

- The native artifact job passes on the current Gitea runner.
- linux/amd64 and linux/arm64 Pulse binaries plus checksums are downloadable from a successful run or release.
- Tag builds attach the native binaries to the corresponding release.
- The one-line Linux installer has a stable download source for the published binaries.
