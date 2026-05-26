# Hashi Pulse Agent

Go agent that reports host identity and private IPv4 candidates to Hashi.

## Build

```bash
cd agents/pulse
go build -o hashi-pulse ./cmd/hashi-pulse
```

## Run

```bash
export HASHI_PULSE_API=https://hashi.example.com
export HASHI_PULSE_AGENT_ID=<agent-guid>
export HASHI_PULSE_TOKEN=<one-time-token-from-create-agent>
./hashi-pulse
```

## Docker

```bash
docker run --rm \
  -e HASHI_PULSE_API=https://hashi.example.com \
  -e HASHI_PULSE_AGENT_ID=<agent-guid> \
  -e HASHI_PULSE_TOKEN=<token> \
  ghcr.io/hashi/pulse:latest
```
