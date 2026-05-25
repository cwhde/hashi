#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

PORT="${HASHI_OPENAPI_PORT:-5099}"
OPENAPI_URL="http://127.0.0.1:${PORT}/openapi/v1.json"
OUTPUT="${ROOT}/openapi/hashi.json"

mkdir -p "${ROOT}/openapi"

dotnet build src/Hashi.Api/Hashi.Api.csproj -c Release >/dev/null

export ASPNETCORE_ENVIRONMENT=Development
export DOTNET_ENVIRONMENT=Development

dotnet run --no-build -c Release --project src/Hashi.Api/Hashi.Api.csproj \
  --urls "http://127.0.0.1:${PORT}" \
  --environment OpenApiExport &
SERVER_PID=$!

cleanup() {
  kill "${SERVER_PID}" 2>/dev/null || true
  wait "${SERVER_PID}" 2>/dev/null || true
}
trap cleanup EXIT

for _ in $(seq 1 30); do
  if curl -sf "${OPENAPI_URL}" -o "${OUTPUT}.tmp"; then
    mv "${OUTPUT}.tmp" "${OUTPUT}"
    echo "Wrote ${OUTPUT}"
    exit 0
  fi
  sleep 1
done

echo "Timed out waiting for OpenAPI document at ${OPENAPI_URL}" >&2
exit 1
