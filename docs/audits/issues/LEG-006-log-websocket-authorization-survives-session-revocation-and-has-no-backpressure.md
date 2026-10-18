# LEG-006: Log WebSocket authorization survives session revocation and has no backpressure

Priority: P2

Category: Legacy security / log streaming

Status: open; manual source review. Applies to the separately deployed legacy artifact, not the active V2 process.

## Finding

The /logs WebSocket validates the session only at connection time. Logout/expiry removes the session but the connection stays registered with the logger and continues receiving logs. There is no explicit Origin policy or bounded outbound buffering; a slow client can accumulate memory. SameSite cookie behavior limits some cross-site scenarios, so no unconditional cross-site cookie theft claim is made.

## Evidence

hashi.old/src/routes/websocket.js; src/services/logger.js addClient/broadcast; src/services/auth.js logout/expiry.

## Expected outcome and acceptance criteria

Close/revalidate streams on expiry/logout/revocation, define allowed origins and cap or disconnect slow consumers. Revoked clients must stop receiving privileged logs.
