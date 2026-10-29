# nftables contract (S6 entry gate, captured 2026-10-29)

Source of truth for the firewall renderer (05-firewall.md §3). Nothing here is written from memory: every file was
produced or checked by the real `nft` binary.

| File | What it is |
| --- | --- |
| `shapes.nft` | Every rule and set shape of 05 §3.2 in nft text syntax (documentation and neutral private addresses) |
| `list_table.json` | `nft -j list table inet hashi` after loading `shapes.nft` (nftables 1.1.3, Debian 13), `handle` fields removed |
| `apply_batch.json` | The §3.3 transaction built from `list_table.json`: add table, add sets (definitions only), add + flush chains, add rules, flush + add elements of the plan-owned sets |

## Where it was checked

| Where | nft | Kernel | Result |
| --- | --- | --- | --- |
| Debian 13 container, own netns (`--cap-add NET_ADMIN`) | 1.1.3 | 6.8 | `nft -c -f shapes.nft` ok; loaded; JSON dump re-checked with `nft -c -j -f` ok, also from stdin (`-f -`) |
| Debian 13 dev target (unprivileged LXC) | 1.1.3 | 7.0.6-pve | `nft -c` (check mode, no commit) of text, JSON and the apply batch: ok |
| First real firewall host | 1.1.3 | 7.0.2-pve | pending the user's OK (check mode only) |

## Findings

1. `nft -j list` output is valid `nft -j -f` input once `handle` fields are dropped; `metainfo` may stay.
2. The apply batch is idempotent: applied twice in a row → same rules (no duplicates, because every chain is flushed
   in the same transaction), and elements added to `dyn4` (with timeout) and `staging4` before the re-apply are still
   there afterwards with their remaining time; `allow4` holds exactly the batch's elements.
3. `add set` with a definition identical to the existing set keeps its elements (that is what keeps `dyn`/`staging`).
4. Meter shapes accepted by 1.1.3 on both kernels:
   - `update @m { ip saddr limit rate over N/minute burst B packets }` on a set with `flags dynamic, timeout`;
   - `add @m { ip saddr ct count over N }` on a set with `flags dynamic` and a `size`, **no timeout** (connlimit
     entries go away with their connections);
   - `add @staging4 { ip saddr }` on `flags dynamic, timeout` with a set-level `timeout 5m`.
5. `dyn6` with `flags interval, timeout` takes /64 elements with a timeout.
6. Debian's default `/etc/nftables.conf` starts with `flush ruleset`. The Debian dev target has `nftables.service`
   enabled and a `table inet filter` loaded from it; the first real host has the unit disabled and no nft tables.

## NetBird backend choice (05 §10 open question)

NetBird v0.81.3 picks its firewall backend once, at engine start, in `client/firewall/create_linux.go` `check()`.
`../netbird/backend_check/` replays that function with the same libraries and versions (go-iptables v0.7.0,
google/nftables v0.3.0). In a netns with NetBird-like iptables-legacy chains and ipsets:

| State | NetBird picks |
| --- | --- |
| iptables-legacy chains only | iptables |
| + `table inet hashi` (full shapes) | iptables |
| + any nft table named `filter` with a chain | **nftables** |
| that table removed again | iptables |
| `iptables` alternative switched to `iptables-nft`, `table inet hashi` present | iptables |

Reason: when `iptables` works and is ≥ 1.8, NetBird switches to nftables only if some nft chain lives in a table named
`filter`; otherwise any listed iptables `filter` chain (the built-ins are always listed) keeps it on iptables. Hashi's
table is named `hashi`, so it never changes NetBird's choice. A `filter` table from another tool (Debian's default
`nftables.conf`, `iptables-nft`) would, at NetBird's next restart — validation reports it.
