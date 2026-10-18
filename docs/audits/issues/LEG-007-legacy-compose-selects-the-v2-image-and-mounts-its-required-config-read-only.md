# LEG-007: Legacy Compose selects the V2 image and mounts its required config read only

Priority: P2

Category: Legacy deployment correctness

Status: open; manual source review. Applies to the separately deployed legacy artifact, not the active V2 process.

## Finding

hashi.old/docker-compose.yml uses the main latest tag while exposing legacy port3000 and expecting /app/config.yml. The main image is now V2 with different ports/storage. Even when selecting the legacy artifact, the config mount is read-only although registration and settings save overwrite it; initial registration can change memory and then fail persistence. The legacy image workflow still publishes an artifact, so the obsolete deployment example remains actionable.

## Evidence

hashi.old/docker-compose.yml:4-9; hashi.old/docker/Dockerfile; .gitea/workflows/docker-build-old.yml; legacy auth/config save methods.

## Expected outcome and acceptance criteria

Document/select the actual legacy artifact and provide writable, restricted durable state with a recoverable bootstrap flow. Keep legacy deployment guidance separate from the V2 installation path.
