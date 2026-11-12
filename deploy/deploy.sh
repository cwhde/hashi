#!/usr/bin/env bash
# deploy.sh <deploy|rollback|smoke> <env> [tag]
# Runs everything on the target over one SSH hop (plus `pct exec` when DEPLOY_CT is set). Idempotent.
set -euo pipefail

cmd=${1:?deploy|rollback|smoke}; env=${2:?env}; tag=${3:-}
here=$(cd "$(dirname "$0")" && pwd)
target="$here/targets/$env.env"
[[ -f $target ]] || { echo "missing $target (copy deploy/targets.example/$env.env)"; exit 2; }
# shellcheck source=/dev/null
source "$target"

on_host() { # run a shell snippet on the Docker host
  if [[ -n ${DEPLOY_CT:-} ]]; then ssh -o BatchMode=yes "$DEPLOY_SSH" pct exec "$DEPLOY_CT" -- sh -c "$(printf '%q' "$1")"
  else ssh -o BatchMode=yes "$DEPLOY_SSH" "$1"; fi
}
on_jump() { ssh -o BatchMode=yes "$DEPLOY_SSH" "$1"; } # the SSH host itself (LAN vantage point for smoke checks)

smoke() {
  local want=${1:-} base="http://$DEPLOY_HOST_IP"
  echo "smoke: waiting for /readyz on $base:$DEPLOY_ADMIN_PORT"
  for _ in $(seq 1 30); do
    if on_jump "curl -fsS -m 3 $base:$DEPLOY_ADMIN_PORT/readyz" >/dev/null 2>&1; then break; fi
    sleep 2
  done
  on_jump "curl -fsS -m 3 $base:$DEPLOY_ADMIN_PORT/readyz" && echo
  local v; v=$(on_jump "curl -fsS -m 3 $base:$DEPLOY_ADMIN_PORT/api/version")
  echo "version: $v"
  if [[ -n $want && $want != latest ]] && ! grep -Eq "\"v?$want\"" <<<"$v"; then echo "smoke: expected $want"; exit 1; fi
  on_jump "curl -fsS -m 3 -o /dev/null -w 'spa %{http_code}\n' $base:$DEPLOY_ADMIN_PORT/"
  on_jump "curl -fsS -m 3 -o /dev/null -w 'public-apps %{http_code}\n' $base:$DEPLOY_PUBLIC_APPS_PORT/"
  on_jump "curl -fsS -m 3 -o /dev/null -w 'public-status %{http_code}\n' $base:$DEPLOY_PUBLIC_STATUS_PORT/"
}

up() { # write compose + .env, pull, start
  local t=$1
  on_host "mkdir -p '$DEPLOY_DIR' '$DEPLOY_DATA_DIR' && chown 65532:65532 '$DEPLOY_DATA_DIR' && chmod 700 '$DEPLOY_DATA_DIR'"
  if [[ -n ${DEPLOY_CT:-} ]]; then
    ssh -o BatchMode=yes "$DEPLOY_SSH" pct exec "$DEPLOY_CT" -- sh -c "'cat > $DEPLOY_DIR/compose.yaml.new'" < "$here/compose/compose.yaml"
  else
    ssh -o BatchMode=yes "$DEPLOY_SSH" "cat > '$DEPLOY_DIR/compose.yaml.new'" < "$here/compose/compose.yaml"
  fi
  # The master key is created once on the host by the operator (never by this script, never in the repo).
  # Strict vault mode (02 §5.4) keeps the key off the host: the file stays, empty, so compose can still start.
  if [[ ${DEPLOY_VAULT_MODE:-service} == strict ]]; then
    on_host "test -e '$DEPLOY_DIR/master.key'" || { echo "strict mode: create an empty $DEPLOY_DIR/master.key on the host"; exit 1; }
  else
    on_host "test -s '$DEPLOY_DIR/master.key'" || { echo "missing $DEPLOY_DIR/master.key on the host:" \
      "create it once with \`docker run --rm git.juzo.io/juzo/hashi:$t keygen\`, owner 65532, mode 0400"; exit 1; }
  fi
  # Cap (06 §11) is optional: with DEPLOY_CAP_PORT set, its dashboard password is created once on the host (random, never
  # printed) and the `cap` profile is switched on. Without it, cap.env stays an empty file so compose can parse the project.
  local cap=""
  if [[ -n ${DEPLOY_CAP_PORT:-} ]]; then
    cap="COMPOSE_PROFILES=cap\nHASHI_CAP_PORT=$DEPLOY_CAP_PORT\n"
    on_host "cd '$DEPLOY_DIR' && { grep -q '^ADMIN_KEY=.\{12,\}' cap.env 2>/dev/null || \
      { umask 077 && printf 'ADMIN_KEY=%s\n' \"\$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')\" > cap.env; }; } \
      && chown 65532:65532 cap.env && chmod 600 cap.env"
  else
    on_host "cd '$DEPLOY_DIR' && { [ -e cap.env ] || { umask 077 && : > cap.env; }; }"
  fi
  on_host "cd '$DEPLOY_DIR' && mv compose.yaml.new compose.yaml && chmod 600 compose.yaml \
    && cur=\$( (grep -E '^HASHI_TAG=' .env 2>/dev/null || true) | cut -d= -f2) \
    && { [ -z \"\$cur\" ] || [ \"\$cur\" = '$t' ] || echo \"\$cur\" > .previous-tag; } \
    && printf 'HASHI_TAG=%s\nHASHI_ADMIN_PORT=%s\nHASHI_PUBLIC_APPS_PORT=%s\nHASHI_PUBLIC_STATUS_PORT=%s\nHASHI_DATA_DIR=%s\n$cap' \
       '$t' '$DEPLOY_ADMIN_PORT' '$DEPLOY_PUBLIC_APPS_PORT' '$DEPLOY_PUBLIC_STATUS_PORT' '$DEPLOY_DATA_DIR' > .env && chmod 600 .env \
    && docker compose pull --quiet && docker compose up -d --remove-orphans && docker compose ps"
}

case $cmd in
  deploy)   [[ -n $tag ]] || { echo "TAG required"; exit 2; }; up "$tag"; smoke "$tag" ;;
  rollback) prev=$(on_host "cat '$DEPLOY_DIR/.previous-tag'"); [[ -n $prev ]] || { echo "no previous tag"; exit 1; }
            echo "rolling back to $prev"; up "$prev"; smoke "$prev" ;;
  smoke)    smoke "$tag" ;;
  *)        echo "unknown command $cmd"; exit 2 ;;
esac
