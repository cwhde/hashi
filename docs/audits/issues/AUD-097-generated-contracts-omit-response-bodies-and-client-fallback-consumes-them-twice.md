# AUD-097: Generated contracts omit response bodies and client fallback consumes them twice

Priority: P2

Category: API contract / frontend reliability

Status: open; confirmed by manual source trace and independent synthetic reproduction.

## Finding

Many handlers return IResult JSON without concrete response metadata, leaving committed OpenAPI success responses with no content schema (passkey ceremonies, bootstrap, DNS mutations/validation, resource mutations, sync apply). The frontend routes around generated typing with handwritten shapes and as-never casts. Resource create/update and AdGuard sync additionally call readUndocumentedJson(r.response) after openapi-fetch already parsed the body. Response.json then throws because the stream is consumed; the helper suppresses it and returns an empty object. AdGuard sync is also cast to a legacy synced shape although the backend returns succeeded/status/runId/message, losing failure details. postUndocumented was independently rewritten to raw fetch and is not itself subject to this double-consumption defect.

## Evidence

openapi/hashi.json:1091-1177,1191-1211,2113-2200; web/src/lib/api/schema.d.ts corresponding content?:never declarations; web/src/lib/api/client.ts:83-89,393-401,896-901; src/Hashi.Api/Features/Auth/AuthEndpoints.cs and Resources/ResourceEndpoints.cs IResult handlers.

Main specification sections4/29 require generated OpenAPI/client types; AUD-095 separately captures truly absent routes. AUD-092 covers resulting misleading success UI.

## Verification

Independent work/repros/response-body.mjs invokes the installed openapi-fetch with a synthetic in-memory fetch response: parsedData is populated, bodyAlreadyConsumed=true, and the current fallback pattern returns {}. No remote service or existing tests used.

## Expected outcome

Describe real success/error payloads with concrete DTOs and metadata, regenerate both contracts, consume parsed data exactly once, and remove casts/fallbacks concealing missing operations or mismatched response shapes.

## Acceptance criteria

Generated clients infer actual payloads for each operation; synthetic responses preserve IDs and succeeded=false/message fields. Resource save/sync can distinguish success, failure and cancellation without swallowing body-read errors.
