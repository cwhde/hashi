#!/usr/bin/env bash
set -euo pipefail

API_URL="${HASHI_PULSE_API:-}"
AGENT_ID="${HASHI_PULSE_AGENT_ID:-}"
TOKEN="${HASHI_PULSE_TOKEN:-}"
INSTALL_DIR="${HASHI_PULSE_INSTALL_DIR:-/opt/hashi/pulse}"
SERVICE_NAME="${HASHI_PULSE_SERVICE_NAME:-hashi-pulse}"
SOURCE_DIR="${HASHI_PULSE_SOURCE_DIR:-}"

usage() {
  cat <<EOF
Hashi Pulse Linux installer

Required environment variables:
  HASHI_PULSE_API       Hashi API base URL (e.g. https://hashi.example.com)
  HASHI_PULSE_AGENT_ID  Pulse agent UUID
  HASHI_PULSE_TOKEN     One-time agent token

Optional:
  HASHI_PULSE_INSTALL_DIR   Install directory (default: /opt/hashi/pulse)
  HASHI_PULSE_SOURCE_DIR    Path to agents/pulse checkout to build from source
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ -z "$API_URL" || -z "$AGENT_ID" || -z "$TOKEN" ]]; then
  echo "Missing required HASHI_PULSE_* environment variables." >&2
  usage >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
TARGET="${INSTALL_DIR}/hashi-pulse"

if [[ -x "$TARGET" ]]; then
  echo "Reusing existing binary at $TARGET"
elif [[ -n "$SOURCE_DIR" && -f "${SOURCE_DIR}/go.mod" ]]; then
  echo "Building hashi-pulse from ${SOURCE_DIR}..."
  (cd "$SOURCE_DIR" && go build -o "$TARGET" ./cmd/hashi-pulse)
elif command -v go >/dev/null && [[ -f go.mod ]]; then
  echo "Building hashi-pulse from current directory..."
  go build -o "$TARGET" ./cmd/hashi-pulse
else
  echo "No hashi-pulse binary found. Set HASHI_PULSE_SOURCE_DIR or place a binary at $TARGET" >&2
  exit 1
fi

chmod 0755 "$TARGET"

cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<UNIT
[Unit]
Description=Hashi Pulse agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=HASHI_PULSE_API=${API_URL}
Environment=HASHI_PULSE_AGENT_ID=${AGENT_ID}
Environment=HASHI_PULSE_TOKEN=${TOKEN}
ExecStart=${TARGET}
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now "${SERVICE_NAME}.service"
echo "Hashi Pulse installed and started as ${SERVICE_NAME}.service"
