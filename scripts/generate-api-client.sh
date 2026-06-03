#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT/web"

if [[ ! -f ../openapi/hashi.json ]]; then
  "$ROOT/scripts/export-openapi.sh"
fi

if command -v pnpm >/dev/null 2>&1; then
  PNPM=(pnpm)
elif command -v corepack >/dev/null 2>&1; then
  PNPM=(corepack pnpm)
else
  echo "pnpm is required to generate the API client. Install pnpm or enable Corepack." >&2
  exit 1
fi

"${PNPM[@]}" exec openapi-typescript ../openapi/hashi.json -o src/lib/api/schema.d.ts

echo "Generated web/src/lib/api/schema.d.ts"
