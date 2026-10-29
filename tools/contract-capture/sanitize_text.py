#!/usr/bin/env python3
"""Sanitize raw command-output captures (firewall probe, S6) for the public repo (contract-capture skill, step 5).

Usage: sanitize_text.py <raw file> <out file> <denylist> [zone ...]
- `rename.txt` next to the denylist (gitignored): `real=fake` labels (host names), applied first, case-insensitive.
- Every zone given and names under it → example.com, example.org, … (in argument order).
- Public IPv4 → 192.0.2.x / 198.51.100.x / 203.0.113.x; private IPv4 → 10.20.x.y; 127.0.0.0/8, 0.0.0.0 and
  255.255.255.255 stay. IPv6: global → 2001:db8::x, ULA → fd00::x, link-local → fe80::x, :: and ::1 stay. Every
  mapping is stable per value, so the same address keeps its identity across the file.
- MAC addresses → 02:00:00:00:xx:yy; WireGuard public keys → wgkeyN…; NetBird peer names other than renamed
  hosts → peerN; names under the example zones → hostN.example.*.
- Fails if any denylist line is still present.
"""
import ipaddress
import pathlib
import re
import sys

raw, out, deny = (pathlib.Path(p) for p in sys.argv[1:4])
zones = sys.argv[4:]
text = raw.read_text()

rename = deny.parent / "rename.txt"
if rename.exists():
    for line in rename.read_text().splitlines():
        if "=" in line:
            real, fake = line.split("=", 1)
            text = re.sub(re.escape(real.strip()), fake.strip(), text, flags=re.IGNORECASE)

fake_zones = ["example.com", "example.org", "example.net"]
for i, z in enumerate(zones):
    text = re.sub(re.escape(z), fake_zones[i], text, flags=re.IGNORECASE)

maps: dict[str, str] = {}
pools = ["192.0.2.", "198.51.100.", "203.0.113."]
counts = {"pub4": 0, "priv4": 0, "g6": 0, "ula": 0, "ll": 0}


def map_v4(s: str) -> str:
    ip = ipaddress.IPv4Address(s)
    if ip.is_loopback or s in ("0.0.0.0", "255.255.255.255"):
        return s
    if s not in maps:
        if ip.is_private or ip in ipaddress.IPv4Network("100.64.0.0/10"):
            n = counts["priv4"]
            counts["priv4"] += 1
            maps[s] = f"10.20.{n // 250}.{n % 250 + 1}"
        else:
            n = counts["pub4"]
            counts["pub4"] += 1
            maps[s] = pools[n // 250 % 3] + str(n % 250 + 1)
    return maps[s]


def map_v6(s: str) -> str:
    try:
        ip = ipaddress.IPv6Address(s)
    except ValueError:
        return s
    if ip.is_loopback or ip.is_unspecified:
        return s
    if s not in maps:
        kind, base = ("ll", "fe80::") if ip.is_link_local else (("ula", "fd00::") if ip in ipaddress.IPv6Network("fc00::/7") else ("g6", "2001:db8::"))
        counts[kind] += 1
        maps[s] = f"{base}{counts[kind]:x}"
    return maps[s]


# Networks seen in the capture ("a.b.c.d/n" and ip -j's "local"+"prefixlen"): an address inside one maps into that
# network's fake network, so interface addresses, routes and rules stay consistent. Short private prefixes
# (10/8, 100.64/10, 172.16/12) and /0 are generic and stay; host parts become 1, 2, 3, … per network.
seen_nets: set[ipaddress.IPv4Network] = set()
for a, b in re.findall(r"(?<![\w.])(\d{1,3}(?:\.\d{1,3}){3})/(\d{1,2})(?![\w.])", text) + \
        re.findall(r'"local":"(\d{1,3}(?:\.\d{1,3}){3})","prefixlen":(\d{1,2})', text):
    try:
        n = ipaddress.IPv4Network(f"{a}/{b}", strict=False)
    except ValueError:
        continue
    if 12 < n.prefixlen < 32 and not n.is_loopback:
        seen_nets.add(n)
fake_net: dict[ipaddress.IPv4Network, ipaddress.IPv4Network] = {}
hosts: dict[str, str] = {}


def map_any_v4(addr: str) -> str:
    ip = ipaddress.IPv4Address(addr)
    inside = sorted((n for n in seen_nets if ip in n), key=lambda n: -n.prefixlen)
    if not inside:
        return map_v4(addr)
    n = inside[0]
    if n not in fake_net:
        k = len(fake_net)
        private = n.is_private or n.subnet_of(ipaddress.IPv4Network("100.64.0.0/10"))
        # Private: 10.(30+k).0.0/len (no two overlap); public: 198.18.k.0/len (len >= 24 in practice).
        base = f"10.{30 + k}.0.0" if private else f"198.18.{k}.0"
        fake_net[n] = ipaddress.IPv4Network((base, n.prefixlen), strict=False)
    if ip == n.network_address:
        return str(fake_net[n].network_address)
    key = addr
    if key not in hosts:
        used = sum(1 for k in hosts if ipaddress.IPv4Address(k) in n)
        hosts[key] = str(fake_net[n].network_address + used + 1)
    return hosts[key]


def map_v4_prefix(addr: str, bits: str) -> str:
    if bits == "0" or (int(bits) <= 12 and ipaddress.IPv4Address(addr).is_private):
        return f"{addr}/{bits}"
    return f"{map_any_v4(addr)}/{bits}"


text = re.sub(r"(?<![\w.])(\d{1,3}(?:\.\d{1,3}){3})/(\d{1,2})(?![\w.])", lambda m: map_v4_prefix(m.group(1), m.group(2)), text)
text = re.sub(r"(?<![\w./])(\d{1,3}(?:\.\d{1,3}){3})(?![\w./])", lambda m: map_any_v4(m.group(1)), text)
# IPv6: hex groups with at least two colons; strings that do not parse are left alone.
text = re.sub(r"(?<![\w:])([0-9a-fA-F]{0,4}(?::[0-9a-fA-F]{0,4}){2,7})(?![\w:])", lambda m: map_v6(m.group(1)), text)
macs: dict[str, str] = {}
text = re.sub(r"\b([0-9a-f]{2}(?::[0-9a-f]{2}){5})\b",
              lambda m: macs.setdefault(m.group(1), f"02:00:00:00:{len(macs) // 256:02x}:{len(macs) % 256:02x}"), text)

# WireGuard public keys and the NetBird names of other peers identify devices: replace them.
keys: dict[str, str] = {}
text = re.sub(r"(?<![A-Za-z0-9+/])([A-Za-z0-9+/]{43}=)",
              lambda m: keys.setdefault(m.group(1), ("wgkey" + str(len(keys) + 1)).ljust(43, "A") + "="), text)
peers: dict[str, str] = {}
text = re.sub(r"\b([a-z0-9-]+)(\.netbird\.selfhosted)\b",
              lambda m: (m.group(1) if m.group(1).startswith("edge-") else peers.setdefault(m.group(1), f"peer{len(peers) + 1}")) + m.group(2), text)

# Labels under the example zones are real service names: hostN, stable per name.
labels: dict[str, str] = {}
text = re.sub(r"\b((?:[a-z0-9-]+\.)+)(example\.(?:com|org|net))\b",
              lambda m: labels.setdefault(m.group(1), f"host{len(labels) + 1}.") + m.group(2), text)

bad = [line.strip() for line in deny.read_text().splitlines() if line.strip() and not line.startswith("#")]
left = [b for b in bad if b.lower() in text.lower()]
for z in zones:
    if z.lower() in text.lower():
        left.append(z)
if left:
    sys.exit(f"denylist entries still present in {out.name}: {len(left)} (not printed)")
out.parent.mkdir(parents=True, exist_ok=True)
out.write_text(text)
print(f"{out}: {len(maps)} addresses, {len(macs)} MACs mapped")
