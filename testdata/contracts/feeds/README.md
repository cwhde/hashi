# Feed contract (captured 2026-11-02)

The recommended blocklists of 06 §8, fetched once each with `curl` (GET, read-only, user agent `hashi-contract-capture`).
Header comments are kept as served; entries are cut to a few lines and their addresses replaced by benchmark and
documentation ranges (`198.18.0.0/15`, `2001:db8::/32`). The bogon lines of FireHOL Level 1 are kept as served,
because they matter.

| File | Source | Content type | Validators | Shape |
| --- | --- | --- | --- | --- |
| `feodo_recommended.txt` | https://feodotracker.abuse.ch/downloads/ipblocklist_recommended.txt | `text/plain` | ETag, Last-Modified | `#` comments, one IPv4 per line, `# END n entries`; served list had 1 entry |
| `spamhaus_drop.txt` | https://www.spamhaus.org/drop/drop.txt | `text/plain; charset=UTF-8` | Last-Modified only | `;` comments, `cidr ; SBLid` (inline comment after `;`), `; EOF`; 1,754 entries |
| `spamhaus_dropv6.txt` | https://www.spamhaus.org/drop/dropv6.txt | same | Last-Modified only | same with IPv6 /48 etc.; 89 entries |
| `spamhaus_drop_v4.jsonl` | https://www.spamhaus.org/drop/drop_v4.json | `text/json` | Last-Modified only | JSON **lines** (not an array): `{"cidr","sblid","rir"}`, last line `{"type":"metadata",…}` |
| `dshield_block.txt` | https://feeds.dshield.org/block.txt | `text/plain` | ETag, Last-Modified | `#` comments, tab separated: start, end, prefix length, attacks, name, country, email; 20 /24s |
| `firehol_level1.netset` | https://iplists.firehol.org/files/firehol_level1.netset | `application/octet-stream` | ETag, Last-Modified | `#` comments, one IPv4/CIDR per line; 3,940 entries |

Findings:

1. **FireHOL Level 1 contains the bogons**: `0.0.0.0/8`, `10.0.0.0/8`, `100.64.0.0/10` (the overlay network's range),
   `127.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `224.0.0.0/3`. Bound to the global block, it would block every
   internal client at forward auth. Hashi therefore never takes private, loopback, link-local, shared (CGNAT),
   multicast or reserved ranges from a feed: they are rejected with a reason and shown in the preview.
2. Spamhaus's JSON files are JSON lines, which 06 §8's formats did not cover; the plain `.txt` lists carry the same
   entries and need only "lines with comments" (`;` as comment marker, inline comments). Hashi recommends the `.txt` URLs.
3. DShield gives a range as start address plus prefix length in another column, not as a CIDR.
4. Spamhaus sends no ETag, only Last-Modified; the other three send both.
