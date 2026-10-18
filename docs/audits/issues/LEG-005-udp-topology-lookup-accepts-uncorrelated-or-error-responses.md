# LEG-005: UDP topology lookup accepts uncorrelated or error responses

Priority: P2

Category: Legacy DNS transport correctness

Status: open; manual source review. Applies to the separately deployed legacy artifact, not the active V2 process.

## Finding

The custom UDP DNS resolver parses the first response without verifying sender address/port, transaction ID, echoed question, RCODE or truncation. Spoofed/unrelated/error responses can be treated as topology data. TXT parsing concatenates separate records, and truncated replies have no TCP fallback. The subnet matcher also uses a JavaScript32bit shift that mishandles /0 and accepts invalid prefixes.

## Evidence

hashi.old/src/utils/dns.js; src/core/topology.js subnetToRange/ipInSubnet/TXT parsing; duplicated subnet helpers in src/core/gatus.js.

## Expected outcome and acceptance criteria

Use a maintained resolver or implement full response correlation/error/truncation handling, preserve TXT RR boundaries and validate subnet prefixes with correct /0 semantics. Failed lookups must not replace good topology or authorize deletion.
