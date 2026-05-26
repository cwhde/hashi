#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT/web"

if [[ ! -f ../openapi/hashi.json ]]; then
  "$ROOT/scripts/export-openapi.sh"
fi

pnpm exec openapi-typescript ../openapi/hashi.json -o src/lib/api/schema.d.ts

echo "Generated web/src/lib/api/schema.d.ts"
