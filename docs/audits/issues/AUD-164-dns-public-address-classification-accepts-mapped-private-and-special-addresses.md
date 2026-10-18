# AUD-164: DNS public address classification accepts mapped private and special addresses

Priority: P2

Category: DNS / address validation

Status: open; confirmed by manual source trace.

## Finding and evidence

DnsRecordGenerator.IsPublicIp treats any parsed address as public unless it is loopback/unspecified, IPv6 link-local/ULA or a four-byte private/special IPv4 range. It does not normalize IPv4-mapped IPv6 before the four-byte test, so `::ffff:10.0.0.1` is classified public. Multicast/reserved ranges are also not excluded. FirstPublicIp can select such values for public resource DNS, violating the intended public/private topology distinction and yielding unusable or unintended published records.

Evidence: `src/Hashi.Core/Dns/DnsRecordGenerator.cs:96,150-151,233-261`. This is separate from the missing CGNAT discovery in Pulse (AUD-086), CIDR bounds (AUD-018) and host A/AAAA generation (AUD-132).

## Expected outcome and acceptance criteria

Canonicalize mapped addresses, define the routable public-address policy explicitly and apply family-aware checks before DNS generation. Mapped RFC1918, loopback, multicast and other excluded special ranges must not be selected as public topology addresses; valid global IPv4/IPv6 must still work.
