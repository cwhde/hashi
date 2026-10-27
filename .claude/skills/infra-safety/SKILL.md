---
name: infra-safety
description: Rules for running anything on the real homelab — Proxmox hosts, LXCs, Docker hosts, Traefik/Pangolin, DNS servers, AdGuard, NetBird, firewall. Use before any ssh, pct, qm, docker or provider API call against real infrastructure.
---

# Working on real infrastructure

The host inventory is in `CLAUDE.local.md` (gitignored). Everything there is production.

**Free to run (read-only):** `hostname`, `uname -a`, `cat /etc/os-release`, `pct list`, `pct config <id>`, `qm list`,
`ip -br a`, `ip route`, `ss -lntup`, `iptables-save`, `ip6tables-save`, `ipset list -t`, `nft list ruleset`,
`netbird status`, `systemctl status`, `docker ps`, `docker compose config` (redact output), `cat` of config files,
`journalctl -n`, `curl -s` GET to local APIs, `pct exec <id> -- <one of the above>`.

**Needs the user's explicit OK for this exact command in this session:** anything that writes files, restarts/stops
services or containers, changes firewall/NAT/ipset/nft state, changes NetBird, installs packages, pulls images, or calls a
provider API with POST/PUT/PATCH/DELETE. The `infra_guard` hook enforces a prompt for most of these; don't try to route around it.

**Never:** flush or delete foreign chains/tables/sets, touch NetBird's rules, reboot a Proxmox host, run `pct destroy`/`qm destroy`,
change SSH config or keys on a host, disable the rescue path.

**Secrets you see** (compose files, `.env`, configs): never echo them into the conversation, files in the repo, commit
messages or logs. Refer to them by location ("Hetzner token in <path> on <host>"). Redact output with
`sed -E 's/([Tt]oken|[Kk]ey|[Ss]ecret|[Pp]ass(word)?)([^=:]*[=:]\s*).*/\1\3<redacted>/'` before it reaches the conversation.

**Firewall changes** go only through Hashi's plan/apply with the rescue timer (`docs/spec/05-firewall.md` §5) — never by hand.
