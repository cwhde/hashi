#!/usr/bin/env bash
# Regression test for deploy.sh rollback bookkeeping. ssh/pct/docker/curl are stubs; the "host" is a temp dir.
set -euo pipefail
repo=$(cd "$(dirname "$0")/../.." && pwd)
work=$(mktemp -d); trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin" "$work/deploy/targets" "$work/deploy/compose" "$work/host"
cp "$repo/deploy/deploy.sh" "$work/deploy/"; cp "$repo/deploy/compose/compose.yaml" "$work/deploy/compose/"
cat > "$work/deploy/targets/test.env" <<T
DEPLOY_SSH=stub
DEPLOY_CT=1
DEPLOY_DIR=$work/host/app
DEPLOY_DATA_DIR=$work/host/data
DEPLOY_ADMIN_PORT=1
DEPLOY_PUBLIC_APPS_PORT=2
DEPLOY_PUBLIC_STATUS_PORT=3
DEPLOY_HOST_IP=127.0.0.1
T
printf '#!/bin/bash\nwhile [[ $1 == -o ]]; do shift 2; done; shift; exec bash -c "$*"\n' > "$work/bin/ssh"
printf '#!/bin/bash\nshift 3; exec "$@"\n' > "$work/bin/pct"
printf '#!/bin/bash\nexit 0\n' > "$work/bin/docker"; cp "$work/bin/docker" "$work/bin/chown"
printf '#!/bin/bash\necho "{\\"version\\":\\"$(grep HASHI_TAG= %s/host/app/.env | cut -d= -f2)\\"}"\n' "$work" > "$work/bin/curl"
chmod +x "$work/bin/"*
export PATH="$work/bin:$PATH"

run() { "$work/deploy/deploy.sh" "$@" >"$work/out" 2>&1 || { echo "FAIL: deploy.sh $*"; cat "$work/out"; exit 1; }; }
tag() { grep HASHI_TAG= "$work/host/app/.env" | cut -d= -f2; }

# Without the master key file the deploy stops before touching the running stack.
if "$work/deploy/deploy.sh" deploy test A >"$work/out" 2>&1 || ! grep -q "master.key" "$work/out"; then
  echo "FAIL: deploy without master.key must stop"; cat "$work/out"; exit 1
fi
mkdir -p "$work/host/app" && echo key > "$work/host/app/master.key"

run deploy test A
run deploy test B
run deploy test B   # redeploy of the same tag must keep A as rollback target
run rollback test
[[ $(tag) == A ]] || { echo "FAIL: rollback after redeploy went to $(tag), want A"; exit 1; }
[[ -f $work/host/app/cap.env && ! -s $work/host/app/cap.env ]] || { echo "FAIL: without Cap, cap.env must exist and be empty"; exit 1; }
grep -q COMPOSE_PROFILES "$work/host/app/.env" && { echo "FAIL: Cap profile on without DEPLOY_CAP_PORT"; exit 1; }

# With Cap: its dashboard password is created once (64 hex characters, owner-only) and kept on the next deploy.
echo "DEPLOY_CAP_PORT=4" >> "$work/deploy/targets/test.env"
run deploy test C
key=$(cat "$work/host/app/cap.env")
[[ $key =~ ^ADMIN_KEY=[0-9a-f]{64}$ ]] || { echo "FAIL: cap.env is not a fresh key"; exit 1; }
[[ $(stat -c %a "$work/host/app/cap.env" 2>/dev/null || stat -f %Lp "$work/host/app/cap.env") == 600 ]] || { echo "FAIL: cap.env mode"; exit 1; }
grep -qx COMPOSE_PROFILES=cap "$work/host/app/.env" && grep -qx HASHI_CAP_PORT=4 "$work/host/app/.env" || { echo "FAIL: .env without the Cap profile"; cat "$work/host/app/.env"; exit 1; }
run deploy test C
[[ $(cat "$work/host/app/cap.env") == "$key" ]] || { echo "FAIL: redeploy replaced the Cap key"; exit 1; }
echo "deploy rollback test: ok"
