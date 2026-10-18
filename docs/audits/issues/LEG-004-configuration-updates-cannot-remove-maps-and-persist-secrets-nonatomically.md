# LEG-004: Configuration updates cannot remove maps and persist secrets nonatomically

Priority: P2

Category: Legacy persistence / config editing

Status: open; manual source review. Applies to the separately deployed legacy artifact, not the active V2 process.

## Finding

deepMerge merges empty objects into existing maps without removing old keys, so the frontend cannot delete some overrides by submitting an empty map. Shape changes and insufficient schema validation can corrupt expected configuration structure. Config and backup writes store raw credentials, overwrite files nonatomically and lack an explicit restrictive permissions policy; an interrupted write can lose the configuration/account. A prototype pollution claim is not established here because Fastify JSON parser protections must be accounted for.

## Evidence

hashi.old/src/core/config.js load/save/backup/deepMerge; src/routes/api.js config update; public/js/app.js override editing.

## Expected outcome and acceptance criteria

Define typed replace/patch/delete semantics, validate the full configuration before committing, use atomic restricted writes and protect credential-bearing backups. Preserve a recoverable previous configuration on interruption.
