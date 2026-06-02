#!/usr/bin/env bash
set -euo pipefail

API_URL="${HASHI_PULSE_API:-}"
AGENT_ID="${HASHI_PULSE_AGENT_ID:-}"
TOKEN="${HASHI_PULSE_TOKEN:-}"
INSTALL_DIR="${HASHI_PULSE_INSTALL_DIR:-/opt/hashi/pulse}"
CONFIG_DIR="${HASHI_PULSE_CONFIG_DIR:-/etc/hashi/pulse}"
CONFIG_FILE="${HASHI_PULSE_CONFIG_FILE:-${CONFIG_DIR}/pulse.env}"
SERVICE_NAME="${HASHI_PULSE_SERVICE_NAME:-hashi-pulse}"
INTERVAL="${HASHI_PULSE_INTERVAL:-60s}"
INSTALL_MODE="${HASHI_PULSE_INSTALL_MODE:-auto}"
CRON_SCHEDULE="${HASHI_PULSE_CRON_SCHEDULE:-* * * * *}"
BINARY_URL="${HASHI_PULSE_BINARY_URL:-}"
CHECKSUM_URL="${HASHI_PULSE_CHECKSUM_URL:-}"
EXPECTED_SHA256="${HASHI_PULSE_SHA256:-}"
DOWNLOAD_BASE_URL="${HASHI_PULSE_DOWNLOAD_BASE_URL:-}"
VERSION="${HASHI_PULSE_VERSION:-latest}"
RELEASES_URL="${HASHI_PULSE_RELEASES_URL:-https://git.juzo.io/juzo/hashi/releases}"
RELEASE_API_URL="${HASHI_PULSE_RELEASE_API_URL:-https://git.juzo.io/api/v1/repos/juzo/hashi/releases/latest}"
SKIP_VERIFY="${HASHI_PULSE_SKIP_VERIFY:-0}"
RUNNER="/usr/local/sbin/${SERVICE_NAME}-run"

usage() {
  cat <<EOF
Hashi Pulse Linux installer

Required environment variables:
  HASHI_PULSE_API       Hashi API base URL (e.g. https://hashi.example.com)
  HASHI_PULSE_AGENT_ID  Pulse agent UUID
  HASHI_PULSE_TOKEN     One-time agent token

Optional:
  HASHI_PULSE_VERSION            Release tag to install (default: latest)
  HASHI_PULSE_BINARY_URL         Direct hashi-pulse binary URL
  HASHI_PULSE_CHECKSUM_URL       Direct SHA-256 checksum URL
  HASHI_PULSE_SHA256             Expected SHA-256 hex digest
  HASHI_PULSE_DOWNLOAD_BASE_URL  Release asset base URL
  HASHI_PULSE_RELEASES_URL       Release page base URL
  HASHI_PULSE_RELEASE_API_URL    Latest release API URL
  HASHI_PULSE_ARCH               Override detected artifact architecture
  HASHI_PULSE_INSTALL_DIR        Install directory (default: /opt/hashi/pulse)
  HASHI_PULSE_CONFIG_DIR         Config directory (default: /etc/hashi/pulse)
  HASHI_PULSE_INSTALL_MODE       auto, systemd, or cron (default: auto)
  HASHI_PULSE_INTERVAL           Heartbeat interval for systemd timer (default: 60s)
  HASHI_PULSE_CRON_SCHEDULE      Cron schedule fallback (default: * * * * *)
  HASHI_PULSE_SKIP_VERIFY        Set to 1 only for private/manual artifacts
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

if [[ "$(id -u)" != "0" ]]; then
  echo "Run this installer as root, for example: curl -fsSL ... | sudo bash" >&2
  exit 1
fi

case "$INSTALL_MODE" in
  auto|systemd|cron) ;;
  *)
    echo "HASHI_PULSE_INSTALL_MODE must be auto, systemd, or cron." >&2
    exit 1
    ;;
esac

detect_arch() {
  if [[ -n "${HASHI_PULSE_ARCH:-}" ]]; then
    echo "$HASHI_PULSE_ARCH"
    return
  fi

  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      echo "Unsupported CPU architecture: $(uname -m). Set HASHI_PULSE_ARCH to override." >&2
      return 1
      ;;
  esac
}

download() {
  local url="$1"
  local destination="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$destination"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$destination" "$url"
  else
    echo "curl or wget is required to download Hashi Pulse." >&2
    return 1
  fi
}

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  else
    echo "sha256sum or shasum is required to verify Hashi Pulse." >&2
    return 1
  fi
}

shell_quote() {
  local value="$1"
  printf "'"
  printf "%s" "$value" | sed "s/'/'\\\\''/g"
  printf "'"
}

asset_base_url() {
  if [[ -n "$DOWNLOAD_BASE_URL" ]]; then
    echo "${DOWNLOAD_BASE_URL%/}"
  else
    local tag="$VERSION"
    if [[ "$tag" == "latest" ]]; then
      local tmp_release
      tmp_release="$(mktemp)"
      echo "Resolving latest Hashi Pulse release..." >&2
      download "$RELEASE_API_URL" "$tmp_release"
      tag="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp_release" | head -n1)"
      rm -f "$tmp_release"
      if [[ -z "$tag" ]]; then
        echo "Could not resolve latest release tag from ${RELEASE_API_URL}." >&2
        exit 1
      fi
    fi
    echo "${RELEASES_URL%/}/download/${tag}"
  fi
}

install_binary() {
  local arch
  arch="$(detect_arch)"
  local asset="hashi-pulse-linux-${arch}"
  local base
  base="$(asset_base_url)"
  local url="${BINARY_URL:-${base}/${asset}}"
  local checksum_url="${CHECKSUM_URL:-${url}.sha256}"
  local tmp_binary
  tmp_binary="$(mktemp)"
  local tmp_checksum
  tmp_checksum="$(mktemp)"
  trap 'rm -f "$tmp_binary" "$tmp_checksum"' EXIT

  echo "Downloading Hashi Pulse ${VERSION} (${arch})..."
  download "$url" "$tmp_binary"

  local expected="$EXPECTED_SHA256"
  if [[ -z "$expected" && "$SKIP_VERIFY" != "1" ]]; then
    echo "Downloading SHA-256 checksum..."
    download "$checksum_url" "$tmp_checksum"
    expected="$(awk '{print $1}' "$tmp_checksum")"
  fi

  if [[ "$SKIP_VERIFY" != "1" ]]; then
    if [[ -z "$expected" ]]; then
      echo "Missing SHA-256 checksum. Set HASHI_PULSE_SHA256 or HASHI_PULSE_CHECKSUM_URL." >&2
      exit 1
    fi
    local actual
    actual="$(sha256_file "$tmp_binary")"
    if [[ "${actual,,}" != "${expected,,}" ]]; then
      echo "Checksum mismatch for ${asset}." >&2
      echo "Expected: ${expected}" >&2
      echo "Actual:   ${actual}" >&2
      exit 1
    fi
  else
    echo "Skipping checksum verification because HASHI_PULSE_SKIP_VERIFY=1"
  fi

  install -d -m 0755 -o root -g root "$INSTALL_DIR"
  install -m 0755 -o root -g root "$tmp_binary" "${INSTALL_DIR}/hashi-pulse"
}

write_config() {
  install -d -m 0750 -o root -g root "$CONFIG_DIR"
  local tmp_config
  tmp_config="$(mktemp)"
  {
    printf "HASHI_PULSE_API=%s\n" "$(shell_quote "$API_URL")"
    printf "HASHI_PULSE_AGENT_ID=%s\n" "$(shell_quote "$AGENT_ID")"
    printf "HASHI_PULSE_TOKEN=%s\n" "$(shell_quote "$TOKEN")"
    printf "HASHI_PULSE_INTERVAL=%s\n" "$(shell_quote "$INTERVAL")"
  } > "$tmp_config"
  install -m 0600 -o root -g root "$tmp_config" "$CONFIG_FILE"
  rm -f "$tmp_config"
}

write_runner() {
  local target="${INSTALL_DIR}/hashi-pulse"
  local tmp_runner
  tmp_runner="$(mktemp)"
  cat > "$tmp_runner" <<RUNNER_SCRIPT
#!/usr/bin/env bash
set -euo pipefail
set -a
. "${CONFIG_FILE}"
set +a
exec "${target}" --once
RUNNER_SCRIPT
  install -m 0755 -o root -g root "$tmp_runner" "$RUNNER"
  rm -f "$tmp_runner"
}

install_systemd_timer() {
  cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<UNIT
[Unit]
Description=Hashi Pulse heartbeat
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=${RUNNER}
UNIT

  cat > "/etc/systemd/system/${SERVICE_NAME}.timer" <<TIMER
[Unit]
Description=Run Hashi Pulse heartbeat

[Timer]
OnBootSec=30s
OnUnitActiveSec=${INTERVAL}
AccuracySec=5s
Persistent=true
Unit=${SERVICE_NAME}.service

[Install]
WantedBy=timers.target
TIMER

  systemctl daemon-reload
  systemctl enable --now "${SERVICE_NAME}.timer"
  echo "Hashi Pulse installed with ${SERVICE_NAME}.timer"
}

install_cron() {
  if [[ ! -d /etc/cron.d ]]; then
    echo "Cannot install cron fallback because /etc/cron.d does not exist." >&2
    exit 1
  fi

  cat > "/etc/cron.d/${SERVICE_NAME}" <<CRON
SHELL=/bin/sh
PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin
${CRON_SCHEDULE} root ${RUNNER} >/dev/null 2>&1
CRON
  chmod 0644 "/etc/cron.d/${SERVICE_NAME}"
  echo "Hashi Pulse installed with cron schedule: ${CRON_SCHEDULE}"
}

install_binary
write_config
write_runner

if [[ "$INSTALL_MODE" == "systemd" ]] || { [[ "$INSTALL_MODE" == "auto" ]] && command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]; }; then
  install_systemd_timer
else
  install_cron
fi
