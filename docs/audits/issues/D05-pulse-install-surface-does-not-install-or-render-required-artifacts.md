# D05 - Pulse install surface does not install or render required artifacts

Priority: Medium

Spec conflicts: section 4 requires the Pulse Linux install to be a one-line command that installs the binary, root-owned config, and a systemd timer or cron fallback. The same section requires Docker install as a generated Compose snippet.

## Problem

The generated Linux command downloads and runs an install script, but the script does not download a released Pulse binary. It reuses an already-present binary or builds from a local source checkout, and otherwise exits. It also installs a long-running systemd service only; it does not install a systemd timer, cron fallback, or OpenRC-compatible fallback.

The generated Docker install output is a `docker run` command, not a Compose snippet as required by the spec.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:99-103` requires a static native agent, Docker image, one-line Linux install with root-owned config and systemd timer or cron fallback, Docker Compose snippet, and per-agent token handling.
- `agents/pulse/install.sh:43-54` reuses an existing binary, builds from `HASHI_PULSE_SOURCE_DIR`, or builds from the current directory; otherwise it exits with "No hashi-pulse binary found."
- `agents/pulse/install.sh:58-66` writes a root-owned `0600` config, which is good, but depends on the missing binary install path.
- `agents/pulse/install.sh:68-87` writes and enables only `/etc/systemd/system/${SERVICE_NAME}.service`.
- `src/Hashi.Infrastructure/Platform/PulseInstallRenderer.cs:10-15` generates the one-line Linux command that pipes `/api/pulse/install/linux.sh` to `sudo bash`.
- `src/Hashi.Infrastructure/Platform/PulseInstallRenderer.cs:16-24` returns a `docker run` command rather than a Compose service.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:631-657` serves the install renderer and shell script, so this is the install path operators receive from the API.

## Expected outcome

The generated Pulse install surface should be directly usable on a fresh Linux host and should provide the requested Docker Compose deployment option.

## Fix guidance

Publish Pulse binaries for linux/amd64 and linux/arm64 and have the install script download and verify the correct artifact. Decide whether Pulse should run continuously or on an interval; if the spec's timer behavior remains desired, install a systemd timer and cron fallback. If the implementation intentionally uses a daemon, update the spec, but still add non-systemd fallback support. Change the Docker install response to include a Compose service snippet.

## Acceptance criteria

- The one-line Linux install works on a fresh supported host without Go or a source checkout.
- The installed binary is root-owned or otherwise protected against unprivileged modification.
- The config remains root-owned and `0600`.
- systemd timer or cron fallback behavior is implemented as specified, or the spec is intentionally revised and non-systemd fallback is still present.
- The Docker install output is a Compose snippet with the required environment variables.
- Tests or smoke scripts cover missing-binary fresh install, architecture selection, and generated Docker Compose content.
