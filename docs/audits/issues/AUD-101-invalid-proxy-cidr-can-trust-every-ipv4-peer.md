# AUD-101: Invalid proxy CIDR can trust every IPv4 peer

Priority: P2

Category: Security / malformed trust configuration

Status: open; reproduced against the actual resolver source.

## Finding

ForwardedClientContextResolver switches IPv4 comparisons to four-byte addresses only when prefixLength<=32. An IPv4 configuration such as `192.0.2.0/33` instead compares IPv4-mapped IPv6 bytes and accepts prefix lengths up to128. For /33 it compares only the common zero prefix of mapped IPv4 addresses, trusting unrelated IPv4 peers. An accidental malformed trusted-proxy entry therefore allows direct clients to spoof forwarded IPs used by session binding and security decisions. Valid configured IPv4 prefixes are not affected by this particular defect.

## Evidence

- `src/Hashi.Api/Hosting/ForwardedClientContextResolver.cs:201-251`: family mapping, <=32 branch and validation against mapped length.
- `work/repros/routing/Program.cs` compiles the actual resolver: configured `192.0.2.0/33`, peer `203.0.113.17`, forged XFF `198.51.100.42` produces TrustedProxy=true and the forged client address.
- AUD-018 concerns a separate DNS CIDR matcher. Both need family-specific prefix validation.

## Expected outcome

Reject invalid CIDRs at startup/configuration boundaries; compare address families explicitly and bound IPv4 prefixes to0..32 and IPv6 to0..128 before mapping.

## Acceptance criteria

Invalid negative/oversized prefixes fail configuration clearly and never broaden trust. Boundary prefixes and IPv4-mapped IPv6 inputs have intentional, documented behavior; unrelated peers cannot forge the canonical client IP.
