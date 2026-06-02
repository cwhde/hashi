# Hashi Pulse Agent

Go agent that reports host identity and private IP candidates to Hashi via heartbeat.

## Build

```bash
cd agents/pulse
make build        # local binary ./hashi-pulse
make docker-image # local image hashi-pulse:local
```

## Run (binary)

```bash
export HASHI_PULSE_API=https://hashi.example.com
export HASHI_PULSE_AGENT_ID=<agent-guid>
export HASHI_PULSE_TOKEN=<one-time-token-from-create-agent>
./hashi-pulse
```

## Install (Linux host)

Create an agent in the Hashi admin UI under **Pulse**, copy the one-time token, then run the generated Linux install command on the target host. The installer:

- downloads the matching `hashi-pulse-linux-amd64` or `hashi-pulse-linux-arm64` release artifact;
- verifies the binary with the matching `.sha256` asset, unless `HASHI_PULSE_SKIP_VERIFY=1` is set;
- writes root-owned config at `/etc/hashi/pulse/pulse.env` with mode `0600`;
- installs a systemd timer that runs one heartbeat per interval, or a root cron fallback when systemd is unavailable.

By default, `HASHI_PULSE_VERSION=latest` resolves the latest release from `https://git.juzo.io/api/v1/repos/juzo/hashi/releases/latest` and downloads assets from `https://git.juzo.io/juzo/hashi/releases/download/<tag>/...`.

Useful overrides:

```bash
export HASHI_PULSE_VERSION=v1.2.3
export HASHI_PULSE_INSTALL_MODE=cron
export HASHI_PULSE_CRON_SCHEDULE='*/5 * * * *'
export HASHI_PULSE_BINARY_URL=https://example.com/hashi-pulse-linux-amd64
export HASHI_PULSE_SHA256=<expected-sha256>
```

## Run (Docker Compose)

Published images: `git.juzo.io/juzo/hashi-pulse:latest` (on `main` merges and version tags).

```yaml
services:
  hashi-pulse:
    image: git.juzo.io/juzo/hashi-pulse:latest
    restart: unless-stopped
    environment:
      HASHI_PULSE_API: https://hashi.example.com
      HASHI_PULSE_AGENT_ID: <agent-guid>
      HASHI_PULSE_TOKEN: <token>
      HASHI_PULSE_DOCKER_IMAGE: git.juzo.io/juzo/hashi-pulse:latest
      HASHI_PULSE_DOCKER_NETWORK_MODE: bridge
```

## CI

- `ci.yml` job **pulse-agent** runs when `agents/**` changes: `make vet`, `make test`, `make build`, and `docker build`.
- `docker-build-pulse.yml` publishes multi-arch images to `git.juzo.io` on `main` and semver tags (`v*.*.*` or `pulse-v*.*.*`).
- `docker-build-pulse.yml` also builds `hashi-pulse-linux-amd64` and `hashi-pulse-linux-arm64`, writes `.sha256` files, uploads them as workflow artifacts, and attaches them to version-tag releases for the Linux installer.
