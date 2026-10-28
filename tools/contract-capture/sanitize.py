#!/usr/bin/env python3
"""Sanitize raw contract captures for the public repo (contract-capture skill, step 5).

Usage: sanitize.py <raw dir> <out dir> <denylist>
- Optional `rename.txt` next to the denylist (gitignored): `real=fake` label pairs (e.g. host names), applied first.
- Every zone name seen in the capture → example.com, example.org, example.net (in order of first appearance); any name
  ending in one is rewritten with the same suffix mapping.
- Zone ids → 100001, 100002, …; IPv4 → 192.0.2.x / 198.51.100.x / 203.0.113.x; IPv6 → 2001:db8::x (stable per value).
- TXT contents keep their quoting and chunk lengths but the characters become 'x'.
- RRSet names: structural labels (`@`, `on`, `via`, `_…`, renamed hosts) stay; every other label becomes svcN; a DKIM
  selector becomes selector1. Hosts outside the zone (except Hetzner's name servers) become hostN.extM.example.net.
- Fails if any denylist line or real zone name is still present.
Structure, field names, types, pagination and error shapes are kept exactly.
"""
import ipaddress
import json
import pathlib
import re
import sys

raw, out, deny = (pathlib.Path(p) for p in sys.argv[1:4])
out.mkdir(parents=True, exist_ok=True)
docs = {f.name: json.loads(f.read_text()) for f in sorted(raw.glob("*.json"))}

zones: dict[str, str] = {}
ids: dict[int, int] = {}
v4: dict[str, str] = {}
v6: dict[str, str] = {}
pools = ["192.0.2.", "198.51.100.", "203.0.113."]


def collect(o):
    if isinstance(o, dict):
        if "name" in o and "record_count" in o and isinstance(o.get("id"), int):
            zones.setdefault(o["name"], ["example.com", "example.org", "example.net"][len(zones)])
            ids.setdefault(o["id"], 100001 + len(ids))
        for v in o.values():
            collect(v)
    elif isinstance(o, list):
        for v in o:
            collect(v)


for d in docs.values():
    collect(d)


def map_ip(s: str) -> str:
    try:
        ip = ipaddress.ip_address(s)
    except ValueError:
        return s
    if ip.version == 4:
        if s not in v4:
            n = len(v4)
            v4[s] = pools[n // 250 % 3] + str(n % 250 + 1)
        return v4[s]
    v6.setdefault(s, f"2001:db8::{len(v6) + 1:x}")
    return v6[s]


rename_file = deny.parent / "rename.txt"
renames = [l.split("=", 1) for l in rename_file.read_text().splitlines() if "=" in l] if rename_file.exists() else []


def map_names(s: str) -> str:
    for real, fake in renames:
        s = re.sub(rf"(?<![A-Za-z0-9-]){re.escape(real.strip())}(?![A-Za-z0-9-])", fake.strip(), s)
    for real, fake in zones.items():
        s = re.sub(rf"(?<![A-Za-z0-9-]){re.escape(real)}(\.?)(?![A-Za-z0-9-])", rf"{fake}\1", s)
    return s


KEEP = {"@", "on", "via", "ns", "www", "mail", "autodiscover", "hosts", "api", "status"}
labels: dict[str, str] = {}
ext_domains: dict[str, int] = {}
ext_hosts: dict[str, str] = {}


def map_rel(name: str) -> str:
    out = []
    parts = name.split(".")
    for i, l in enumerate(parts):
        fakes = {f.strip() for _, f in renames}
        if i + 1 < len(parts) and parts[i + 1] == "_domainkey":
            out.append("selector1")
        elif l in KEEP or l.startswith("_") or l in fakes:
            out.append(l)
        else:
            out.append(labels.setdefault(l, f"svc{len(labels) + 1}"))
    return ".".join(out)


def map_host(tok: str) -> str:
    if not tok.endswith(".") or "." not in tok[:-1] or not re.fullmatch(r"[A-Za-z0-9._-]+", tok):
        return tok
    host = tok[:-1]
    for fake in ("example.com", "example.org", "example.net"):
        if host == fake or host.endswith("." + fake):
            rel = host[: -len(fake)].rstrip(".")
            return (map_rel(rel) + "." if rel else "") + fake + "."
    if re.search(r"\.ns\.hetzner\.(com|de)$", host) or host == "dns.hetzner.com":
        return tok
    if host.isdigit():
        return tok
    dom = ".".join(host.split(".")[-2:])
    m = ext_domains.setdefault(dom, len(ext_domains) + 1)
    return ext_hosts.setdefault(host, f"host{len(ext_hosts) + 1}.ext{m}.example.net.")


def txt(s: str) -> str:
    return re.sub(r'"((?:[^"\\]|\\.)*)"', lambda m: '"' + "x" * len(m.group(1)) + '"', s)


def walk(o, rtype=None, key=None):
    if isinstance(o, dict):
        t = o.get("type", rtype)
        d = {k: walk(v, t, k) for k, v in o.items()}
        if "records" in o and isinstance(o.get("name"), str):
            d["name"] = map_rel(map_names(o["name"]))
            if isinstance(o.get("id"), str) and "/" in o["id"]:
                n, typ = o["id"].rsplit("/", 1)
                d["id"] = map_rel(map_names(n)) + "/" + typ
        return d
    if isinstance(o, list):
        return [walk(v, rtype, key) for v in o]
    if isinstance(o, int) and not isinstance(o, bool) and o in ids and key in ("id", "zone"):
        return ids[o]
    if isinstance(o, str):
        if key == "request":
            for real_id, fake_id in ids.items():
                o = o.replace(f"/{real_id}", f"/{fake_id}")
            return map_names(o)
        if rtype == "TXT" and key == "value":
            return txt(o)
        parts = [map_host(map_names(map_ip(p))) for p in o.split(" ")]
        return " ".join(parts)
    return o


denylist = [l.strip() for l in deny.read_text().splitlines() if l.strip() and not l.startswith("#")]
for name, d in docs.items():
    text = json.dumps(walk(d), indent=2, ensure_ascii=False) + "\n"
    leaks = [w for w in denylist + list(zones) if w in text]
    if leaks:
        sys.exit(f"{name}: still contains {len(leaks)} denylisted value(s); not written")
    (out / name).write_text(text)
    print(f"sanitized {name}")
