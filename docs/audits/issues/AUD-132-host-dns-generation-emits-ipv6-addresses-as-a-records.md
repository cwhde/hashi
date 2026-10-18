# AUD-132: Host DNS generation emits IPv6 addresses as A records

Priority: P2

Category: DNS / IPv6 support

Status: open; confirmed by manual source trace.

## Finding

GenerateHostRecords always constructs an A record from host.PublicIp, unlike GenerateResourceRecords which selects AAAA for IPv6. The firewall-host model accepts an address string and supports IPv6 topology. A valid IPv6 public host address therefore produces an invalid A record and can abort provider apply for the whole zone.

## Evidence

src/Hashi.Core/Dns/DnsRecordGenerator.cs:34-56; resource override family selection at69-74 provides the contrasting supported representation.

## Expected outcome and acceptance criteria

Validate and normalize host addresses and emit A/AAAA according to address family. Cover IPv4, IPv6, mapped addresses and invalid values without allowing one malformed host record to fail unrelated safe plans.
