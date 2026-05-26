# Hashi Pulse Agent

Go agent that reports host identity and private IPv4 candidates to Hashi via heartbeat.

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

## Run (Docker)

Published images: `git.juzo.io/juzo/hashi-pulse:latest` (on `main` merges and version tags).

```bash
docker run --rm \
  -e HASHI_PULSE_API=https://hashi.example.com \
  -e HASHI_PULSE_AGENT_ID=<agent-guid> \
  -e HASHI_PULSE_TOKEN=<token> \
  git.juzo.io/juzo/hashi-pulse:latest
```

Create an agent in the Hashi admin UI under **Pulse**, copy the one-time token, then deploy the container on the target host.

## CI

- `ci.yml` job **pulse-agent** runs when `agents/**` changes: `make vet`, `make test`, `make build`, and `docker build`.
- `docker-build-pulse.yml` publishes multi-arch images to `git.juzo.io` on `main` and semver tags (`v*.*.*` or `pulse-v*.*.*`).
