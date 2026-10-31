# Firewall host contract (S6 PR B, captured 2026-10-29)

Read-only command output from two hosts over SSH, sanitized with `tools/contract-capture/sanitize_text.py`
(documentation and neutral private addresses, renamed hosts, WireGuard keys and peer names replaced; networks stay
valid networks and an address keeps its network across the file). The rule shapes Hashi writes are in `../nft/`.

| File | Host | Software |
| --- | --- | --- |
| `probe_debian13_pve_netbird.txt` | First real firewall host: Proxmox VE on Debian 13, public address on `vmbr0`, LXC bridge `vmbr1`, NetBird routing peer (`wt0`) | kernel 7.0.2-pve, nftables 1.1.3, iptables 1.8.11 (legacy), NetBird 0.81.3, systemd 257 |
| `probe_debian13_lxc.txt` | Debian 13 dev target (unprivileged LXC, no NetBird) | kernel 7.0.6-pve, nftables 1.1.3, systemd 257 |
| `takeover_iptables_legacy.txt` | First real host: `iptables-legacy-save`, `ip6tables-legacy-save`, `ipset save`, root's crontab | — |

`probe.sh` is the exact probe (`firewall.ProbeCommand`; a test keeps them equal). Commands in the probe files, each framed by `### <command>` and `### rc=<exit code>`: `uname -r`, `/etc/os-release`,
`nft --version`, `nft -j list ruleset`, `iptables -V`, `ip -j addr show`, `ip -j route show default` (and `-6`),
`ip -j link show`, `ss -Hlntup`, `systemctl is-enabled|is-active nftables`, `systemctl is-active firewalld`,
`netbird status --json`, `netbird status`, `sysctl -n net.ipv4.ip_forward net.ipv6.conf.all.forwarding`,
`systemd-run --version`, then `command -v` for every tool Hashi needs (`missing:<tool>` when absent).

## Findings

1. `iptables -V` prints the active backend in parentheses: `iptables v1.8.11 (legacy)` / `(nf_tables)`.
2. `nft -j list ruleset` on a host without tables is only the `metainfo` object. The Debian dev target has a
   `table inet filter` (from Debian's enabled `nftables.service`) — the table that would switch NetBird (../nft/).
3. `netbird status --json` exists in 0.81.3: `fqdn`, `netbirdIp` (with prefix: the overlay network), `netbirdIpv6`,
   `usesKernelInterface`, `wireguardPort`, `daemonStatus`, `management.connected`, `signal.connected`, `networks`
   (routes this peer uses, **not** the networks it routes for others: those appear only as `Networks:` in the text
   output, together with the default routes)). It prints no final newline, so the probe's `### rc=` marker lands at the end of the JSON line. It names no interface; the kernel interface is found in `ip -j link`
   (`wt0` here) by the overlay address. Without NetBird: `sh: 1: netbird: not found`, rc 127.
4. `systemctl is-active` exit codes: 0 active, 3 inactive, 4 unknown unit (firewalld absent). `is-enabled`: 1 when
   disabled.
5. `ss -Hlntup` lists the NetBird WireGuard port (51820/udp) without a process (kernel WireGuard).
6. The old firewall script's objects (05 §3.4) live in iptables-legacy: chains `CUSTOM_DNAT` (nat, jumped from
   PREROUTING and OUTPUT), `CUSTOM_FWD`, `CUSTOM_INPUT` (filter, `CUSTOM_INPUT` ends in DROP), rules in built-in chains
   (mangle FORWARD TCPMSS, three nat POSTROUTING MASQUERADEs, filter FORWARD `trusted_hosts` ACCEPT and the duplicated
   `wt0`↔bridge ACCEPT pairs), the ipset `trusted_hosts`, and two `@reboot` + two `*/15` crontab lines. NetBird's own
   objects are the `NETBIRD-*` chains, their jumps and CONNMARK rules, and the `nb…` ipsets.

7. `$SSH_CONNECTION` in a login (checked on the dev target) is `<client ip> <client port> <server ip> <server port>`:
   validation stores the client address as the rescue source.

8. Takeover formats: the remove and restore inputs are `iptables-legacy-restore --noflush` scripts (`-D` for each copy
   of a rule, then `-F` and `-X` of the script's chains, one atomic commit per table), `ipset restore -exist` for saved
   sets, `crontab <file>` for the crontab. Run against the real binaries in `TestIntegrationTakeover`.

## Rescue timer (`systemd-run`, Debian dev target, systemd 257)

- `systemd-run --on-active=N --unit=hashi-fw-rescue <cmd>` creates `hashi-fw-rescue.timer` + `.service`; exit 0.
- Without `--timer-property=AccuracySec=1s` the timer fired 6 s after a 3 s arm (systemd's default accuracy is one
  minute); with it, 3.9 s. Hashi always sets it.
- `systemctl stop hashi-fw-rescue.timer` disarms (exit 0); the command never runs; the units are gone afterwards.
- Arming while a timer of that name is loaded fails: `Failed to start transient timer unit: Unit hashi-fw-rescue.timer
  was already loaded or has a fragment file.` (exit 1). After a successful run both units unload by themselves; after
  a failed run the service stays `failed` and arming fails the same way (`Unit hashi-fw-rescue.service was already
  loaded …`) until `systemctl reset-failed hashi-fw-rescue.service`. Hashi therefore runs `systemctl stop
  hashi-fw-rescue.timer` and `systemctl reset-failed hashi-fw-rescue.service` (errors ignored) before arming.
