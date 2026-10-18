# AUD-168: Pulse release publication is not atomic and agent version is static

Priority: P2

Category: Release reliability / agent metadata

Status: open; confirmed by manual workflow and agent source trace.

## Finding and evidence

The native release workflow creates a published release before attaching its binaries/checksums, and replaces existing assets by deleting them before uploading replacements. The installer resolves the latest published release immediately, so it can select a release with missing or mismatched assets during publication/retry. The agent also reports a hard-coded `const version = "0.1.0"`; native and Docker builds do not derive this value from the release, so later artifacts still report that version. Earlier reading notes calling the default `dev` were incorrect; the actual constant is0.1.0.

Evidence: `.gitea/workflows/docker-build-pulse.yml` release creation/deletion/upload sequence; `agents/pulse/install.sh:127-175`; `agents/pulse/cmd/hashi-pulse/main.go:20,119,139`.

## Expected outcome and acceptance criteria

Publish a draft/complete verified asset set atomically, then expose it to latest-release discovery; avoid destructive replacement gaps for active published versions. Embed an accurate immutable release/source identity in native and image builds. The installer must never select a partially published release, and heartbeat version metadata must identify the artifact actually installed.
