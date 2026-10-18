# LEG-008: Legacy generated checks and RRset updates misrepresent actual services

Priority: P3

Category: Legacy monitoring / provider correctness

Status: open; manual source review. Applies to the separately deployed legacy artifact, not the active V2 process.

## Finding

Gatus generation probes UDP/DNS service availability using TCP, forces insecure HTTPS checking, builds unbracketed IPv6 URLs and can generate duplicate names. The Hetzner adapter overwrites a whole RRset with one record, ignores requested TTL on update and has no overall request deadlines; failed mutations can be masked by the caller. The frontend provider test uses stored values instead of the currently edited draft, and its simple YAML parser cannot round-trip nested/quoted configuration reliably.

## Evidence

hashi.old/src/core/gatus.js; src/core/hetzner.js; src/utils/validation.js obsolete DNS token-test URL; public/js/app.js tests/YAML parser.

## Expected outcome and acceptance criteria

Preserve multi-value provider semantics, deadlines and failure outcomes; generate protocol-correct monitoring with valid URI/identity handling. Test drafts explicitly and use a real supported parser for editable structured configuration.
