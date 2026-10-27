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

run deploy test A
run deploy test B
run deploy test B   # redeploy of the same tag must keep A as rollback target
run rollback test
[[ $(tag) == A ]] || { echo "FAIL: rollback after redeploy went to $(tag), want A"; exit 1; }
echo "deploy rollback test: ok"
